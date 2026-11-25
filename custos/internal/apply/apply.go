// Package apply is the reconciler behind custos-apply: it reads
// custos.json, renders every generated file, writes only what changed
// (temporary file + rename), and applies each change the cheapest safe way:
// nft -f (one transaction), networkctl reload, hostapd restart, AdGuard
// Home API calls. Running it twice in a row changes nothing the second time.
package apply

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/martincvitic19/custos-marginis/custos/internal/agh"
	"github.com/martincvitic19/custos-marginis/custos/internal/alerts"
	"github.com/martincvitic19/custos-marginis/custos/internal/config"
	"github.com/martincvitic19/custos-marginis/custos/internal/render"
	"github.com/martincvitic19/custos-marginis/custos/internal/state"
	"github.com/martincvitic19/custos-marginis/custos/internal/wgkeys"
)

// Paths.
const (
	RunDir      = "/run/custos"
	LockPath    = RunDir + "/lock"
	StatusPath  = RunDir + "/apply.json"
	CurfewPath  = RunDir + "/curfew.json"
	NFTPath     = RunDir + "/firewall/50-custos-policy.nft"
	WGDir       = "/data/custos/wg"
	MullvadConf = WGDir + "/mullvad.conf"
)

// Modes.
const (
	ModeBoot  = "boot"  // before networking: nft + networkd files, no AdGuard Home
	ModeFull  = "full"  // everything
	ModeRetry = "retry" // timer: same as full (also moves curfew boundaries)
)

// Options configure a run (tests point the paths into a temp dir and swap
// the command runner).
type Options struct {
	Mode       string
	ConfigPath string
	StatePath  string
	Secrets    string
	Root       string // prefix for every generated path ("" on the device)
	AGH        *agh.Client
	Run        func(name string, args ...string) error
	Now        func() time.Time
	WiFiMAC    func() string // radio MAC, "" when there is no radio
	EventsPath string
	// AGHWait is how long to wait for AdGuard Home's API (boot: it may
	// still be loading block lists).
	AGHWait time.Duration
}

// DeviceOptions returns the options used on the router.
func DeviceOptions(mode string) Options {
	return Options{
		Mode:       mode,
		ConfigPath: config.DefaultPath,
		StatePath:  state.StatePath,
		Secrets:    state.SecretsPath,
		AGH:        agh.New(agh.DefaultBase),
		Run:        run,
		Now:        time.Now,
		WiFiMAC:    radioMAC,
		EventsPath: alerts.Path,
		AGHWait:    10 * time.Second,
	}
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...) // fixed binaries, validated arguments, no shell
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, strings.TrimSpace(out.String()))
	}
	return nil
}

// radioMAC returns the address of the first Wi-Fi radio's wlan0.
func radioMAC() string {
	b, err := os.ReadFile("/sys/class/net/wlan0/address")
	if err == nil {
		return strings.TrimSpace(string(b))
	}
	// AP mode not started yet: the phy's permanent address.
	m, _ := filepath.Glob("/sys/class/ieee80211/*/macaddress")
	if len(m) > 0 {
		b, err := os.ReadFile(m[0])
		if err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	return ""
}

// Status is /run/custos/apply.json.
type Status struct {
	Time     time.Time `json:"time"`
	Mode     string    `json:"mode"`
	OK       bool      `json:"ok"`
	Error    string    `json:"error,omitempty"`
	Changes  []string  `json:"changes"`
	Devices  int       `json:"devices"`
	Peers    int       `json:"peers"`
	Curfew   any       `json:"curfew"`
	Duration string    `json:"duration"`
}

func (o *Options) path(p string) string { return o.Root + p }

// Run does one reconcile. It always writes the status file.
func Run(ctx context.Context, o Options) (*Status, error) {
	start := o.Now()
	st := &Status{Time: start, Mode: o.Mode, Changes: []string{}}
	err := runLocked(ctx, &o, st)
	st.OK = err == nil
	if err != nil {
		st.Error = err.Error()
	}
	st.Duration = o.Now().Sub(start).Round(time.Millisecond).String()
	if b, e := json.MarshalIndent(st, "", "  "); e == nil {
		os.MkdirAll(filepath.Dir(o.path(StatusPath)), 0755)
		config.WriteFileAtomic(o.path(StatusPath), append(b, '\n'), 0644)
	}
	if len(st.Changes) > 0 && o.EventsPath != "" && o.Mode != ModeBoot {
		msg := strings.Join(st.Changes, "; ")
		if len(msg) > 500 {
			msg = msg[:500] + "..."
		}
		alerts.Append(o.path(o.EventsPath), alerts.Event{Kind: alerts.KindApply, Severity: "info", Message: msg})
	}
	return st, err
}

func runLocked(ctx context.Context, o *Options, st *Status) error {
	if err := os.MkdirAll(o.path(RunDir), 0755); err != nil {
		return err
	}
	lock, err := os.OpenFile(o.path(LockPath), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	// Wait for a concurrent run (timer vs. socket) instead of failing.
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock: %w", err)
	}

	cfg, created, err := loadOrCreate(o)
	if err != nil {
		return err
	}
	if created {
		st.Changes = append(st.Changes, "custos.json created with defaults")
	}
	st.Devices, st.Peers = len(cfg.Devices), len(cfg.VPN.Peers)

	s, err := state.Load(o.path(o.StatePath))
	if err != nil {
		return err
	}
	if ch, err := s.Assign(cfg); err != nil {
		return err
	} else if ch {
		if err := s.Save(o.path(o.StatePath)); err != nil {
			return err
		}
		chgrp(o.path(o.StatePath), "custos") // custosd shows reserved addresses
		st.Changes = append(st.Changes, "DHCP reservations updated")
	}
	sec, err := state.LoadSecrets(o.path(o.Secrets))
	if err != nil {
		return err
	}
	if ch, err := sec.EnsureWiFi(cfg); err != nil {
		return err
	} else if ch {
		if err := sec.Save(o.path(o.Secrets)); err != nil {
			return err
		}
		chgrp(o.path(o.Secrets), "custos")
		st.Changes = append(st.Changes, "Wi-Fi passphrases generated")
	}

	var mullvad *render.Mullvad
	if cfg.VPN.Enabled {
		if ch, err := ensureServerKey(o); err != nil {
			return err
		} else if ch {
			st.Changes = append(st.Changes, "WireGuard server key generated")
		}
		if b, err := os.ReadFile(o.path(MullvadConf)); err == nil {
			m, err := render.ParseMullvad(string(b))
			if err != nil {
				return err
			}
			if ch, err := writeIfChanged(o.path(WGDir+"/mullvad.key"), m.PrivateKey+"\n", 0640, "systemd-network"); err != nil {
				return err
			} else if ch {
				st.Changes = append(st.Changes, "Mullvad key installed")
			}
			mullvad = m
		}
	}

	now := o.Now()
	in := render.Input{Cfg: cfg, State: s, Now: now, MullvadUp: mullvad != nil, WGIf: "wg0"}
	snap := render.Curfew(in)
	st.Curfew = snap

	// 1. Firewall policy: one nft transaction.
	nftChanged, err := writeIfChanged(o.path(NFTPath), render.NFT(in), 0600, "")
	if err != nil {
		return err
	}
	if nftChanged {
		if err := o.Run("nft", "-f", o.path(NFTPath)); err != nil {
			return err
		}
		st.Changes = append(st.Changes, "firewall policy loaded")
	}

	// 2. networkd: DHCP reservations, WireGuard, Wi-Fi bridging.
	files := render.Leases(cfg, s)
	vpnFiles := render.VPN(cfg, mullvad, WGDir)
	files = append(files, vpnFiles...)
	wifi := render.WiFi(cfg, sec, o.WiFiMAC())
	var hostapd *render.File
	for i := range wifi {
		if wifi[i].Path == render.HostapdPath {
			hostapd = &wifi[i]
		} else {
			files = append(files, wifi[i])
		}
	}
	netChanged, wgChanged, err := syncNetworkd(o, files)
	if err != nil {
		return err
	}
	if netChanged && o.Mode == ModeBoot {
		// networkd starts after us and reads the files itself.
		st.Changes = append(st.Changes, "networkd files written")
	} else if netChanged {
		if wgChanged {
			// networkd does not update an existing WireGuard netdev in place.
			o.Run("networkctl", "delete", "wg0")
			o.Run("networkctl", "delete", "wg-mullvad")
		}
		if err := o.Run("networkctl", "reload"); err != nil {
			return err
		}
		st.Changes = append(st.Changes, "networkd reloaded")
	}

	// 3. hostapd.
	if hostapd != nil {
		if ch, err := writeIfChanged(o.path(hostapd.Path), hostapd.Content, 0600, ""); err != nil {
			return err
		} else if ch && o.Mode == ModeBoot {
			st.Changes = append(st.Changes, "hostapd configuration written")
		} else if ch {
			if err := o.Run("systemctl", "restart", "custos-hostapd.service"); err != nil {
				return err
			}
			st.Changes = append(st.Changes, "hostapd restarted")
		}
	} else if _, err := os.Stat(o.path(render.HostapdPath)); err == nil {
		os.Remove(o.path(render.HostapdPath))
		o.Run("systemctl", "stop", "custos-hostapd.service")
		st.Changes = append(st.Changes, "hostapd stopped (no radio or Wi-Fi disabled)")
	}

	// 4. Curfew: cut existing connections of devices that just got blocked.
	flushed := flushCurfew(o, cfg, s, snap)
	st.Changes = append(st.Changes, flushed...)

	// 5. AdGuard Home (not at boot: it starts later).
	if o.Mode != ModeBoot && o.AGH != nil {
		// Right after boot AdGuard Home may still be loading its lists.
		var probe map[string]any
		deadline := o.Now().Add(o.AGHWait)
		// /status answers before the filtering handlers are registered
		// (404 on /filtering/status right after start), so wait for that.
		for o.AGH.Get(ctx, "/filtering/status", &probe) != nil && o.Now().Before(deadline) {
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
			}
		}
		res, err := o.AGH.Reconcile(ctx, cfg, s)
		if err != nil {
			return fmt.Errorf("adguardhome: %w", err)
		}
		st.Changes = append(st.Changes, res.Changes...)
	}
	return nil
}

func loadOrCreate(o *Options) (*config.Config, bool, error) {
	p := o.path(o.ConfigPath)
	cfg, err := config.Load(p)
	if err == nil {
		return cfg, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, false, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0770); err != nil {
		return nil, false, err
	}
	cfg = config.Default()
	if err := cfg.Save(p, 0660); err != nil {
		return nil, false, err
	}
	chgrp(p, "custos")
	chgrp(filepath.Dir(p), "custos")
	return cfg, true, nil
}

func ensureServerKey(o *Options) (bool, error) {
	dir := o.path(WGDir)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return false, err
	}
	chgrp(dir, "systemd-network")
	key := filepath.Join(dir, "wg0.key")
	if b, err := os.ReadFile(key); err == nil {
		pub, err := wgkeys.Public(strings.TrimSpace(string(b)))
		if err != nil {
			return false, err
		}
		_, err = writeIfChanged(filepath.Join(dir, "wg0.pub"), pub+"\n", 0644, "")
		return false, err
	}
	priv, pub, err := wgkeys.Generate()
	if err != nil {
		return false, err
	}
	if _, err := writeIfChanged(key, priv+"\n", 0640, "systemd-network"); err != nil {
		return false, err
	}
	_, err = writeIfChanged(filepath.Join(dir, "wg0.pub"), pub+"\n", 0644, "")
	return true, err
}

// WireGuardServerPublicKey reads wg0.pub (for peer configs).
func WireGuardServerPublicKey(root string) (string, error) {
	b, err := os.ReadFile(root + WGDir + "/wg0.pub")
	return strings.TrimSpace(string(b)), err
}

func chgrp(path, group string) {
	g, err := user.LookupGroup(group)
	if err != nil {
		return
	}
	gid, _ := strconv.Atoi(g.Gid)
	os.Chown(path, 0, gid)
}

// writeIfChanged writes content when it differs (atomically) and reports
// whether it did.
func writeIfChanged(path, content string, mode os.FileMode, group string) (bool, error) {
	if b, err := os.ReadFile(path); err == nil && string(b) == content {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return false, err
	}
	if err := config.WriteFileAtomic(path, []byte(content), mode); err != nil {
		return false, err
	}
	os.Chmod(path, mode)
	if group != "" {
		chgrp(path, group)
	}
	return true, nil
}

// Generated networkd files are recognised by name, so stale ones (a VLAN
// without devices any more, VPN disabled, radio removed) are deleted.
func managed(rel string) bool {
	base := filepath.Base(rel)
	return base == "50-custos-leases.conf" ||
		strings.HasPrefix(base, "30-custos-wg0.") ||
		strings.HasPrefix(base, "31-custos-wg-mullvad.") ||
		strings.HasPrefix(base, "06-custos-wlan")
}

func syncNetworkd(o *Options, files []render.File) (changed, wg bool, err error) {
	want := map[string]bool{}
	for _, f := range files {
		p := o.path(f.Path)
		want[p] = true
		ch, err := writeIfChanged(p, f.Content, os.FileMode(f.Mode), f.Group)
		if err != nil {
			return false, false, err
		}
		if ch {
			changed = true
			if strings.Contains(f.Path, "-wg") {
				wg = true
			}
		}
	}
	root := o.path(render.NetworkdDir)
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !managed(p) || want[p] {
			return nil
		}
		os.Remove(p)
		changed = true
		if strings.Contains(p, "-wg") {
			wg = true
		}
		return nil
	})
	return changed, wg, nil
}

// flushCurfew deletes conntrack entries of devices that moved into curfew
// since the last run. The drop rules already sit before "established
// accept"; the flush also ends UDP flows and keeps conntrack clean. Deleting
// an entry of an allowed device is harmless: the next packet re-creates it.
func flushCurfew(o *Options, c *config.Config, s *state.State, now render.CurfewSnapshot) []string {
	var prev render.CurfewSnapshot
	if b, err := os.ReadFile(o.path(CurfewPath)); err == nil {
		json.Unmarshal(b, &prev)
	}
	if b, err := json.Marshal(now); err == nil {
		config.WriteFileAtomic(o.path(CurfewPath), b, 0644)
	}
	was := map[string]bool{}
	for _, n := range append(prev.Devices, prev.Peers...) {
		was[n] = true
	}
	var out []string
	for _, d := range c.Devices {
		if contains(now.Devices, d.Name) && !was[d.Name] {
			if ip := s.Leases[state.LeaseKey(d.MAC, d.VLAN)]; ip != "" {
				o.Run("conntrack", "-D", "-f", "ipv4", "-s", ip)
			}
			out = append(out, "curfew started: "+d.Name)
		} else if !contains(now.Devices, d.Name) && was[d.Name] {
			out = append(out, "curfew ended: "+d.Name)
		}
	}
	for _, p := range c.VPN.Peers {
		if contains(now.Peers, p.Name) && !was[p.Name] {
			o.Run("conntrack", "-D", "-f", "ipv4", "-s", p.Address)
			out = append(out, "curfew started: "+p.Name)
		}
	}
	if now.Unknown && !prev.Unknown {
		for _, v := range c.Network.VLANs {
			gw := config.VLANGateway(v.ID).As4()
			net := netip.AddrFrom4([4]byte{gw[0], gw[1], gw[2], 0})
			o.Run("conntrack", "-D", "-f", "ipv4", "-s", net.String(), "--mask-src", "255.255.255.0")
		}
		out = append(out, "curfew started: unknown devices (child role)")
	} else if !now.Unknown && prev.Unknown {
		out = append(out, "curfew ended: unknown devices")
	}
	sort.Strings(out)
	return out
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

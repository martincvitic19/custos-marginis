package watch

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/martincvitic19/custos-marginis/custos/internal/alerts"
	"github.com/martincvitic19/custos-marginis/custos/internal/config"
	"github.com/martincvitic19/custos-marginis/custos/internal/state"
)

// Sample is one reading of the per-device counters.
type Sample struct {
	Time  time.Time
	Bytes int64
	DNS   int64
}

// Watcher keeps the counter history and de-duplicates alerts.
type Watcher struct {
	ConfigPath string
	StatePath  string
	EventsPath string
	Emit       func(alerts.Event) // defaults to appending to EventsPath

	mu      sync.Mutex
	history map[string][]Sample // key "mac . ifname"
	last    map[string]time.Time
}

// New returns a watcher with device paths.
func New() *Watcher {
	w := &Watcher{ConfigPath: config.DefaultPath, StatePath: state.StatePath, EventsPath: alerts.Path,
		history: map[string][]Sample{}, last: map[string]time.Time{}}
	w.Emit = func(e alerts.Event) {
		if err := alerts.Append(w.EventsPath, e); err != nil {
			log.Printf("event log: %v", err)
		}
		log.Printf("ALERT %s %s %s: %s", e.Kind, e.Device, e.MAC, e.Message)
	}
	return w
}

// once emits an alert at most once per key and period.
func (w *Watcher) once(key string, period time.Duration, now time.Time, e alerts.Event) {
	w.mu.Lock()
	t, ok := w.last[key]
	if ok && now.Sub(t) < period {
		w.mu.Unlock()
		return
	}
	w.last[key] = now
	w.mu.Unlock()
	e.Time = now
	w.Emit(e)
}

// nftCounters reads a dynamic counter set (nft -j).
func NFTCounters(set string) (map[string]int64, map[string]int64, error) {
	out, err := exec.Command("nft", "-j", "list", "set", "inet", "custos", set).Output()
	if err != nil {
		return nil, nil, fmt.Errorf("nft list set %s: %w", set, err)
	}
	return parseNFTSet(out)
}

func parseNFTSet(out []byte) (bytesBy, pktsBy map[string]int64, err error) {
	var doc struct {
		Nftables []struct {
			Set *struct {
				Elem []struct {
					Elem struct {
						Val struct {
							Concat []string `json:"concat"`
						} `json:"val"`
						Counter struct {
							Packets int64 `json:"packets"`
							Bytes   int64 `json:"bytes"`
						} `json:"counter"`
					} `json:"elem"`
				} `json:"elem"`
			} `json:"set"`
		} `json:"nftables"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, nil, err
	}
	bytesBy, pktsBy = map[string]int64{}, map[string]int64{}
	for _, n := range doc.Nftables {
		if n.Set == nil {
			continue
		}
		for _, e := range n.Set.Elem {
			if len(e.Elem.Val.Concat) != 2 {
				continue
			}
			k := e.Elem.Val.Concat[0] + " . " + e.Elem.Val.Concat[1]
			bytesBy[k] = e.Elem.Counter.Bytes
			pktsBy[k] = e.Elem.Counter.Packets
		}
	}
	return bytesBy, pktsBy, nil
}

func deviceName(c *config.Config, key string) (name, mac string) {
	mac, ifn, _ := strings.Cut(key, " . ")
	for _, d := range c.Devices {
		if d.MAC == mac && c.VLANIf(d.VLAN) == ifn {
			return d.Name, mac
		}
	}
	return "nepoznat uređaj (" + ifn + ")", mac
}

// Traffic takes a counter sample and checks the bypass and upload rules.
func (w *Watcher) Traffic(c *config.Config, now time.Time, tx, dns map[string]int64) {
	bw := time.Duration(c.Alerts.Bypass.WindowMinutes) * time.Minute
	uw := time.Duration(c.Alerts.Upload.WindowMinutes) * time.Minute
	keep := bw
	if uw > keep {
		keep = uw
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for k, b := range tx {
		h := append(w.history[k], Sample{now, b, dns[k]})
		// Counters restart when the firewall is reloaded: start over.
		if len(h) > 1 && (b < h[len(h)-2].Bytes || dns[k] < h[len(h)-2].DNS) {
			h = h[len(h)-1:]
		}
		for len(h) > 1 && now.Sub(h[1].Time) >= keep {
			h = h[1:]
		}
		w.history[k] = h
		name, mac := deviceName(c, k)
		if s, ok := oldest(h, now, bw); ok {
			db, dd := b-s.Bytes, dns[k]-s.DNS
			if db >= c.Alerts.Bypass.MinBytes && dd <= c.Alerts.Bypass.MaxDNS {
				w.mu.Unlock()
				w.once("bypass "+k, time.Hour, now, alerts.Event{Kind: alerts.KindBypass, Severity: "alert", Device: name, MAC: mac,
					Message: fmt.Sprintf("%s vjerojatno zaobilazi zaštitu: %d MB prometa i %d DNS upita prema Custosu u %d min",
						name, db>>20, dd, c.Alerts.Bypass.WindowMinutes)})
				w.mu.Lock()
			}
		}
		if s, ok := oldest(h, now, uw); ok && b-s.Bytes >= c.Alerts.Upload.MaxBytes {
			w.mu.Unlock()
			w.once("upload "+k, time.Hour, now, alerts.Event{Kind: alerts.KindUpload, Severity: "warning", Device: name, MAC: mac,
				Message: fmt.Sprintf("neobično velik odlazni promet: %s je poslao %d MB u %d min", name, (b-s.Bytes)>>20, c.Alerts.Upload.WindowMinutes)})
			w.mu.Lock()
		}
	}
}

// oldest returns the first sample at least window old (the window is
// covered completely), if any.
func oldest(h []Sample, now time.Time, window time.Duration) (Sample, bool) {
	var best Sample
	ok := false
	for _, s := range h {
		if now.Sub(s.Time) >= window {
			best, ok = s, true
		}
	}
	return best, ok
}

// ISPSide scans the wan subnet. Expected: the ISP router (default gateway)
// and Custos itself. Anything else is connected around Custos.
func (w *Watcher) ISPSide(c *config.Config, now time.Time) (gatewayMAC string, err error) {
	res, _, err := ARPScan(c.Network.WAN, 3*time.Second)
	if err != nil {
		return "", err
	}
	gw := defaultGateway()
	ignore := map[string]bool{}
	for _, m := range c.Alerts.ISPWatch.IgnoreMACs {
		ignore[m] = true
	}
	for _, r := range res {
		if r.IP == gw {
			gatewayMAC = r.MAC
			continue
		}
		if ignore[r.MAC] {
			continue
		}
		w.once("isp "+r.MAC, 6*time.Hour, now, alerts.Event{Kind: alerts.KindISPSide, Severity: "alert", MAC: r.MAC, IP: r.IP.String(),
			Message: fmt.Sprintf("na operaterov router je spojen uređaj mimo Custosa: %s (%s)", r.IP, r.MAC)})
	}
	return gatewayMAC, nil
}

// defaultGateway reads the main table default route from /proc.
func defaultGateway() netip.Addr {
	b, _ := readFile("/proc/net/route")
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) > 2 && f[1] == "00000000" {
			var g uint32
			fmt.Sscanf(f[2], "%X", &g)
			return netip.AddrFrom4([4]byte{byte(g), byte(g >> 8), byte(g >> 16), byte(g >> 24)})
		}
	}
	return netip.Addr{}
}

var (
	bssRe  = regexp.MustCompile(`(?m)^BSS ([0-9a-f:]{17})`)
	ssidRe = regexp.MustCompile(`(?m)^\s*SSID: (.*)$`)
)

// ISPWiFi scans the air (AP mode needs "ap-force") for the ISP router's
// network: a configured ISP SSID, or a BSSID next to the ISP router's MAC.
func (w *Watcher) ISPWiFi(c *config.Config, gatewayMAC string, now time.Time) error {
	if _, err := net.InterfaceByName("wlan0"); err != nil {
		return nil // no radio (W-WIFI): nothing to scan
	}
	out, err := exec.Command("iw", "dev", "wlan0", "scan", "ap-force").Output()
	if err != nil {
		return fmt.Errorf("iw scan: %w", err)
	}
	isp := map[string]bool{}
	for _, s := range c.WiFi.ISPSSIDs {
		isp[s] = true
	}
	for _, block := range strings.Split(string(out), "\nBSS ")[0:] {
		b := "BSS " + strings.TrimPrefix(block, "BSS ")
		m := bssRe.FindStringSubmatch(b)
		if m == nil {
			continue
		}
		ssid := ""
		if s := ssidRe.FindStringSubmatch(b); s != nil {
			ssid = strings.TrimSpace(s[1])
		}
		near := gatewayMAC != "" && len(gatewayMAC) == 17 && m[1][3:14] == gatewayMAC[3:14]
		if isp[ssid] || near {
			w.once("wifi "+m[1], 6*time.Hour, now, alerts.Event{Kind: alerts.KindISPWiFi, Severity: "alert", MAC: m[1],
				Message: fmt.Sprintf("operaterov router emitira Wi-Fi mrežu %q (%s): ugasite je, inače se može zaobići Custos", ssid, m[1])})
		}
	}
	return nil
}

// ARPGuard listens on every LAN VLAN and alerts when someone else claims
// the router's address there (ARP spoofing / man in the middle).
func (w *Watcher) ARPGuard(c *config.Config, stop <-chan struct{}) {
	for _, v := range c.Network.VLANs {
		ifn := c.VLANIf(v.ID)
		gw := config.VLANGateway(v.ID)
		ifi, err := net.InterfaceByName(ifn)
		if err != nil {
			continue
		}
		own := ifi.HardwareAddr.String()
		go func() {
			err := ListenARP(ifn, stop, func(p arpPacket) {
				if p.SenderIP == gw && p.SenderMAC.String() != own {
					w.once("spoof "+ifn+p.SenderMAC.String(), 10*time.Minute, time.Now(), alerts.Event{Kind: alerts.KindARPSpoof, Severity: "alert",
						MAC: p.SenderMAC.String(), Message: fmt.Sprintf("uređaj %s se na %s predstavlja kao Custos (%s): mogući ARP spoofing / prisluškivanje", p.SenderMAC, ifn, gw)})
				}
			})
			if err != nil {
				log.Printf("ARP guard %s: %v", ifn, err)
			}
		}()
	}
}

func readFile(p string) ([]byte, error) { return os.ReadFile(p) }

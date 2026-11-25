package api

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/martincvitic19/custos-marginis/custos/internal/alerts"
	"github.com/martincvitic19/custos-marginis/custos/internal/apply"
	"github.com/martincvitic19/custos-marginis/custos/internal/cgnat"
	"github.com/martincvitic19/custos-marginis/custos/internal/config"
	"github.com/martincvitic19/custos-marginis/custos/internal/curfew"
	"github.com/martincvitic19/custos-marginis/custos/internal/qrpng"
	"github.com/martincvitic19/custos-marginis/custos/internal/state"
)

// ---- status ----------------------------------------------------------------

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	var applySt map[string]any
	readJSONFile(s.P.ApplyStatus, &applySt)
	var agh map[string]any
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	aghErr := s.AGH.Get(ctx, "/status", &agh)
	evs, _ := alerts.Read(s.P.Events, time.Now().Add(-24*time.Hour), 0)
	warn := []alerts.Event{}
	for _, e := range evs {
		if e.Severity != "info" {
			warn = append(warn, e)
		}
	}
	if len(warn) > 20 {
		warn = warn[:20]
	}
	up, _ := strconv.ParseFloat(strings.Fields(readAll("/proc/uptime") + " 0")[0], 64)
	_, radio := os.Stat("/sys/class/ieee80211")
	// XDP edge filter counters (A12). custosd may not touch BPF itself;
	// custos-xdp.timer writes this summary every minute. Missing file =
	// filter not loaded (or not in this image).
	var xdp map[string]any
	if readJSONFile(s.P.XDPStats, &xdp) != nil {
		xdp = nil
	}
	writeJSON(w, 200, map[string]any{
		"uptime_seconds": int(up),
		"apply":          applySt,
		"adguardhome": map[string]any{
			"running":            aghErr == nil && agh["running"] == true,
			"protection_enabled": agh["protection_enabled"],
			"version":            agh["version"],
		},
		"alerts_24h":  warn,
		"wifi_radio":  radio == nil && hasEntries("/sys/class/ieee80211"),
		"wan_address": ifaceAddr("wan"),
		"xdp":         xdp,
	})
}

func hasEntries(dir string) bool {
	e, err := os.ReadDir(dir)
	return err == nil && len(e) > 0
}

func ifaceAddr(name string) string {
	i, err := net.InterfaceByName(name)
	if err != nil {
		return ""
	}
	as, _ := i.Addrs()
	for _, a := range as {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
			return n.String()
		}
	}
	return ""
}

// ---- devices ---------------------------------------------------------------

type deviceView struct {
	config.Device
	IP            string `json:"ip"`
	Known         bool   `json:"known"`
	CurfewNow     bool   `json:"curfew_now"`
	EffectiveRole string `json:"effective_role"`
}

// arpNeighbours reads /proc/net/arp: devices seen on the LAN VLANs.
func arpNeighbours(c *config.Config) map[string][2]string { // key mac/vlan -> ip, ifname
	out := map[string][2]string{}
	lan := map[string]int{}
	for _, v := range c.Network.VLANs {
		lan[c.VLANIf(v.ID)] = v.ID
	}
	sc := bufio.NewScanner(strings.NewReader(readAll("/proc/net/arp")))
	sc.Scan() // header
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 6 || f[2] == "0x0" {
			continue
		}
		vlan, ok := lan[f[5]]
		if !ok {
			continue
		}
		mac, err := config.NormalizeMAC(f[3])
		if err != nil || mac == "00:00:00:00:00:00" {
			continue
		}
		out[state.LeaseKey(mac, vlan)] = [2]string{f[0], f[5]}
	}
	return out
}

func (s *Server) devices(w http.ResponseWriter, r *http.Request) {
	c := s.loadConfig(w)
	if c == nil {
		return
	}
	st, _ := state.Load(s.P.State)
	now := time.Now()
	seen := arpNeighbours(c)
	out := []deviceView{}
	for _, d := range c.Devices {
		k := state.LeaseKey(d.MAC, d.VLAN)
		ip := st.Leases[k]
		if n, ok := seen[k]; ok {
			ip = n[0]
		}
		delete(seen, k)
		out = append(out, deviceView{Device: d, IP: ip, Known: true, CurfewNow: curfew.DeviceBlocked(c, d, now), EffectiveRole: d.Role})
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		mac, vlanS, _ := strings.Cut(k, "/")
		vlan, _ := strconv.Atoi(vlanS)
		out = append(out, deviceView{
			Device: config.Device{Name: "", MAC: mac, VLAN: vlan, Role: config.RoleChild},
			IP:     seen[k][0], Known: false, CurfewNow: curfew.UnknownBlocked(c, now), EffectiveRole: config.RoleChild,
		})
	}
	writeJSON(w, 200, map[string]any{"devices": out, "admin_vlan": c.Network.AdminVLAN, "vlans": c.Network.VLANs})
}

func (s *Server) setDevice(w http.ResponseWriter, r *http.Request) {
	var in config.Device
	if !decode(w, r, &in) {
		return
	}
	mac, err := config.NormalizeMAC(in.MAC)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	in.MAC = mac
	in.CurfewOverrideUntil = nil
	if s.editConfig(w, func(c *config.Config) error {
		for i, d := range c.Devices {
			if strings.EqualFold(d.Name, in.Name) || (d.MAC == in.MAC && d.VLAN == in.VLAN) {
				in.CurfewOverrideUntil = d.CurfewOverrideUntil
				c.Devices[i] = in
				return nil
			}
		}
		c.Devices = append(c.Devices, in)
		return nil
	}) {
		writeJSON(w, 200, in)
	}
}

func (s *Server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	found := false
	if s.editConfig(w, func(c *config.Config) error {
		for i, d := range c.Devices {
			if d.Name == name {
				c.Devices = append(c.Devices[:i], c.Devices[i+1:]...)
				found = true
				return nil
			}
		}
		return fmt.Errorf("no device %q", name)
	}) && found {
		writeJSON(w, 200, map[string]bool{"ok": true})
	}
}

func (s *Server) setRole(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Role string `json:"role"`
	}
	if !decode(w, r, &in) {
		return
	}
	name := r.PathValue("name")
	if s.editConfig(w, func(c *config.Config) error {
		for i := range c.Devices {
			if c.Devices[i].Name == name {
				c.Devices[i].Role = in.Role
				return nil
			}
		}
		return fmt.Errorf("no device %q", name)
	}) {
		writeJSON(w, 200, map[string]bool{"ok": true})
	}
}

func (s *Server) roles(w http.ResponseWriter, r *http.Request) {
	if c := s.loadConfig(w); c != nil {
		writeJSON(w, 200, c.Roles)
	}
}

func (s *Server) putRole(w http.ResponseWriter, r *http.Request) {
	var in config.Role
	if !decode(w, r, &in) {
		return
	}
	if in.BlockedServices == nil {
		in.BlockedServices = []string{}
	}
	if in.Schedules == nil {
		in.Schedules = []config.Schedule{}
	}
	role := r.PathValue("role")
	if s.editConfig(w, func(c *config.Config) error {
		if _, ok := c.Roles[role]; !ok {
			return fmt.Errorf("unknown role %q", role)
		}
		c.Roles[role] = in
		return nil
	}) {
		writeJSON(w, 200, in)
	}
}

// override gives a device (or VPN peer) N more minutes during a curfew.
// It expires by itself, so nobody has to remember to undo it.
func (s *Server) override(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Device  string `json:"device"`
		Minutes int    `json:"minutes"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Minutes < 0 || in.Minutes > 240 {
		fail(w, 400, "minutes must be 0..240 (0 cancels)")
		return
	}
	var until *time.Time
	if in.Minutes > 0 {
		t := time.Now().Add(time.Duration(in.Minutes) * time.Minute).Truncate(time.Second)
		until = &t
	}
	if s.editConfig(w, func(c *config.Config) error {
		for i := range c.Devices {
			if c.Devices[i].Name == in.Device {
				c.Devices[i].CurfewOverrideUntil = until
				return nil
			}
		}
		for i := range c.VPN.Peers {
			if c.VPN.Peers[i].Name == in.Device {
				c.VPN.Peers[i].CurfewOverrideUntil = until
				return nil
			}
		}
		return fmt.Errorf("no device %q", in.Device)
	}) {
		writeJSON(w, 200, map[string]any{"device": in.Device, "until": until})
	}
}

// ---- filtering -------------------------------------------------------------

func (s *Server) filtering(w http.ResponseWriter, r *http.Request) {
	if c := s.loadConfig(w); c != nil {
		writeJSON(w, 200, c.Filtering)
	}
}

func (s *Server) putFiltering(w http.ResponseWriter, r *http.Request) {
	var in config.Filtering
	if !decode(w, r, &in) {
		return
	}
	if in.Rules == nil {
		in.Rules = []string{}
	}
	if s.editConfig(w, func(c *config.Config) error { c.Filtering = in; return nil }) {
		writeJSON(w, 200, in)
	}
}

func (s *Server) check(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("domain")
	if len(name) == 0 || len(name) > 253 || strings.ContainsAny(name, "/ :@?#") {
		fail(w, 400, "domain required")
		return
	}
	out, err := s.AGH.CheckHost(r.Context(), name, r.URL.Query().Get("client"))
	if err != nil {
		fail(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, out)
}

// querylog returns per-device summaries for the last N hours: number of
// queries, number blocked, and the most blocked domains. Never the full
// browsing history (GDPR: data minimisation; the parent sees what was
// stopped, not everything the child looked at).
func (s *Server) querylog(w http.ResponseWriter, r *http.Request) {
	hours, _ := strconv.Atoi(r.URL.Query().Get("hours"))
	if hours <= 0 || hours > 168 {
		hours = 24
	}
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	type entry struct {
		Client   string `json:"client"`
		Time     string `json:"time"`
		Reason   string `json:"reason"`
		Question struct {
			Name string `json:"name"`
		} `json:"question"`
		ClientInfo struct {
			Name string `json:"name"`
		} `json:"client_info"`
	}
	type sum struct {
		Client  string         `json:"client"`
		Name    string         `json:"name"`
		Total   int            `json:"total"`
		Blocked int            `json:"blocked"`
		top     map[string]int `json:"-"`
		Top     []topItem      `json:"top_blocked"`
	}
	sums := map[string]*sum{}
	older := ""
	for page := 0; page < 20; page++ {
		v := url.Values{"limit": {"500"}}
		if older != "" {
			v.Set("older_than", older)
		}
		var res struct {
			Data   []entry `json:"data"`
			Oldest string  `json:"oldest"`
		}
		if err := s.AGH.Get(r.Context(), "/querylog?"+v.Encode(), &res); err != nil {
			fail(w, 502, err.Error())
			return
		}
		stop := len(res.Data) == 0
		for _, e := range res.Data {
			t, _ := time.Parse(time.RFC3339Nano, e.Time)
			if t.Before(since) {
				stop = true
				break
			}
			x := sums[e.Client]
			if x == nil {
				x = &sum{Client: e.Client, Name: strings.TrimPrefix(e.ClientInfo.Name, "custos: "), top: map[string]int{}}
				sums[e.Client] = x
			}
			x.Total++
			if strings.HasPrefix(e.Reason, "Filtered") && e.Reason != "FilteredSafeSearch" || e.Reason == "Rewrite" && strings.Contains(e.Question.Name, "use-application-dns") {
				x.Blocked++
				x.top[e.Question.Name]++
			}
		}
		if stop || res.Oldest == "" {
			break
		}
		older = res.Oldest
	}
	out := []*sum{}
	for _, x := range sums {
		x.Top = topN(x.top, 5)
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Blocked > out[j].Blocked || out[i].Blocked == out[j].Blocked && out[i].Client < out[j].Client
	})
	writeJSON(w, 200, map[string]any{"hours": hours, "devices": out})
}

type topItem struct {
	Domain string `json:"domain"`
	Count  int    `json:"count"`
}

func topN(m map[string]int, n int) []topItem {
	l := []topItem{}
	for k, v := range m {
		l = append(l, topItem{k, v})
	}
	sort.Slice(l, func(i, j int) bool {
		return l[i].Count > l[j].Count || l[i].Count == l[j].Count && l[i].Domain < l[j].Domain
	})
	if len(l) > n {
		l = l[:n]
	}
	return l
}

// ---- connection, events ----------------------------------------------------

func (s *Server) connection(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"wan_address": ifaceAddr("wan")}
	if r.URL.Query().Get("cgnat") == "1" {
		out["cgnat"] = cgnat.Check(r.Context(), "https://am.i.mullvad.net/ip")
	}
	d := net.Dialer{Timeout: 3 * time.Second}
	if c, err := d.DialContext(r.Context(), "tcp", "9.9.9.9:443"); err == nil {
		c.Close()
		out["internet"] = true
	} else {
		out["internet"] = false
	}
	_, mv := os.Stat("/sys/class/net/wg-mullvad")
	out["mullvad_up"] = mv == nil
	writeJSON(w, 200, out)
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	evs, err := alerts.Read(s.P.Events, time.Now().Add(-alerts.Retention), limit)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, evs)
}

func (s *Server) eventsCSV(w http.ResponseWriter, r *http.Request) {
	evs, err := alerts.Read(s.P.Events, time.Now().Add(-alerts.Retention), 0)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="custos-events.csv"`)
	alerts.CSV(w, evs)
}

// ---- VPN -------------------------------------------------------------------

func (s *Server) vpn(w http.ResponseWriter, r *http.Request) {
	c := s.loadConfig(w)
	if c == nil {
		return
	}
	pub, _ := os.ReadFile(s.P.WGPub)
	writeJSON(w, 200, map[string]any{"vpn": c.VPN, "server_public_key": strings.TrimSpace(string(pub))})
}

func (s *Server) putVPN(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled  bool   `json:"enabled"`
		Endpoint string `json:"endpoint"`
		Exit     string `json:"exit"`
		Port     int    `json:"port"`
	}
	if !decode(w, r, &in) {
		return
	}
	if s.editConfig(w, func(c *config.Config) error {
		c.VPN.Enabled, c.VPN.Endpoint, c.VPN.Exit = in.Enabled, in.Endpoint, in.Exit
		if in.Port != 0 {
			c.VPN.Port = in.Port
		}
		return nil
	}) {
		writeJSON(w, 200, map[string]bool{"ok": true})
	}
}

// addPeer creates the key pair here: the private key goes to the parent
// once (config + QR) and is never stored on the router.
func (s *Server) addPeer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
		Role string `json:"role"`
	}
	if !decode(w, r, &in) {
		return
	}
	pub, err := os.ReadFile(s.P.WGPub)
	if err != nil {
		fail(w, 503, "WireGuard server key not ready (is the VPN enabled?)")
		return
	}
	var conf string
	if !s.editConfig(w, func(c *config.Config) error {
		var err error
		conf, err = apply.NewPeer(c, in.Name, in.Role, strings.TrimSpace(string(pub)))
		return err
	}) {
		return
	}
	png, _ := qrpng.PNG(conf)
	writeJSON(w, 200, map[string]any{"config": conf, "qr_png": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)})
}

func (s *Server) deletePeer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.editConfig(w, func(c *config.Config) error {
		for i, p := range c.VPN.Peers {
			if p.Name == name {
				c.VPN.Peers = append(c.VPN.Peers[:i], c.VPN.Peers[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("no peer %q", name)
	}) {
		writeJSON(w, 200, map[string]bool{"ok": true})
	}
}

// ---- Wi-Fi -----------------------------------------------------------------

func (s *Server) wifi(w http.ResponseWriter, r *http.Request) {
	c := s.loadConfig(w)
	if c == nil {
		return
	}
	sec, _ := state.LoadSecrets(s.P.Secrets)
	type ssid struct {
		config.SSID
		Passphrase string `json:"passphrase"`
	}
	l := []ssid{}
	for _, x := range c.WiFi.SSIDs {
		l = append(l, ssid{x, sec.WiFi[x.Name]})
	}
	writeJSON(w, 200, map[string]any{"wifi": c.WiFi, "ssids": l, "radio": hasEntries("/sys/class/ieee80211")})
}

func (s *Server) putWiFi(w http.ResponseWriter, r *http.Request) {
	var in config.WiFi
	if !decode(w, r, &in) {
		return
	}
	if in.ISPSSIDs == nil {
		in.ISPSSIDs = []string{}
	}
	if s.editConfig(w, func(c *config.Config) error { c.WiFi = in; return nil }) {
		writeJSON(w, 200, in)
	}
}

// ---- first-run wizard ------------------------------------------------------

type wizardState struct {
	Done  bool            `json:"done"`
	Steps map[string]bool `json:"steps"`
}

// wizard returns the ISP checklist with automatic checks where possible.
func (s *Server) wizard(w http.ResponseWriter, r *http.Request) {
	var wz wizardState
	readJSONFile(s.P.Wizard, &wz)
	if wz.Steps == nil {
		wz.Steps = map[string]bool{}
	}
	since := time.Now().Add(-30 * time.Minute)
	evs, _ := alerts.Read(s.P.Events, since, 0)
	ispDevices, ispWiFi := 0, 0
	for _, e := range evs {
		switch e.Kind {
		case alerts.KindISPSide:
			ispDevices++
		case alerts.KindISPWiFi:
			ispWiFi++
		}
	}
	auto := map[string]any{
		"only_custos_on_isp_lan": map[string]any{"checked_by": "ARP scan of the ISP router LAN every few minutes", "ok": ispDevices == 0},
		"isp_wifi_off":           map[string]any{"checked_by": "Wi-Fi scan (needs the radio)", "ok": ispWiFi == 0, "radio": hasEntries("/sys/class/ieee80211")},
	}
	writeJSON(w, 200, map[string]any{"done": wz.Done, "steps": wz.Steps, "auto": auto})
}

func (s *Server) putWizard(w http.ResponseWriter, r *http.Request) {
	var in wizardState
	if !decode(w, r, &in) {
		return
	}
	for k := range in.Steps {
		if len(k) > 40 {
			fail(w, 400, "bad step")
			return
		}
	}
	b, _ := jsonMarshal(in)
	if err := config.WriteFileAtomic(s.P.Wizard, b, 0600); err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, in)
}

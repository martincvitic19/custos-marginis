// Package config is the custos.json model: the single source of truth for
// the router's policy (devices, roles, filtering, curfew, VPN, Wi-Fi).
//
// custos-apply reads it and generates everything else (AdGuard Home
// settings, nftables sets and rules, DHCP reservations, WireGuard and
// hostapd configuration). custosd edits it on behalf of the parent. Nothing
// else writes it.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	// Europe/Zagreb must work even if the image has no /usr/share/zoneinfo
	// (curfew times are local wall-clock times).
	_ "time/tzdata"
)

// DefaultPath is where the device keeps its configuration (on /data, so it
// survives a root file system update).
const DefaultPath = "/data/custos/custos.json"

// Role names. A device that is not listed in Devices is always RoleChild.
const (
	RoleChild      = "child"
	RoleTrusted    = "trusted"
	RoleGuest      = "guest"
	RoleUnfiltered = "unfiltered"
)

// Roles lists every valid role, strictest first.
var Roles = []string{RoleChild, RoleGuest, RoleTrusted, RoleUnfiltered}

// Config is the whole custos.json.
type Config struct {
	Version   int        `json:"version"`
	Main      Main       `json:"main"`
	Network   Network    `json:"network"`
	Filtering Filtering  `json:"filtering"`
	Roles     RoleMap    `json:"roles"`
	Devices   []Device   `json:"devices"`
	VPN       VPN        `json:"vpn"`
	WiFi      WiFi       `json:"wifi"`
	Alerts    AlertsConf `json:"alerts"`
}

// Main holds global settings.
type Main struct {
	Name     string `json:"name"`
	Timezone string `json:"timezone"`
}

// Network describes the LAN VLANs. Interface names follow the image's
// network plan (custos-net.inc): <bridge>.<id>, 192.168.<id>.0/24.
type Network struct {
	Bridge    string `json:"bridge"`
	WAN       string `json:"wan"`
	AdminVLAN int    `json:"admin_vlan"`
	VLANs     []VLAN `json:"vlans"`
}

// VLAN is one LAN segment.
type VLAN struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Filtering is the global DNS filtering (AdGuard Home).
type Filtering struct {
	SafeSearch          bool         `json:"safesearch"`
	UpdateIntervalHours int          `json:"update_interval_hours"`
	Lists               []FilterList `json:"lists"`
	// Rules are extra AdGuard rules (adblock syntax), e.g. "||example.com^".
	Rules []string `json:"rules"`
}

// FilterList is one block list URL.
type FilterList struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
}

// RoleMap maps a role name to its policy.
type RoleMap map[string]Role

// Role is what devices of one role may do.
type Role struct {
	// Filtering off means AdGuard Home answers without block lists.
	Filtering  bool `json:"filtering"`
	SafeSearch bool `json:"safesearch"`
	// BlockedServices are AdGuard Home service IDs (e.g. "tiktok").
	BlockedServices []string `json:"blocked_services"`
	// StrictEgress allows only web (tcp 80/443, udp 443) and NTP to the
	// internet; everything else (most VPNs) is dropped.
	StrictEgress bool       `json:"strict_egress"`
	Schedules    []Schedule `json:"schedules"`
}

// Schedule is a curfew window in local time (Main.Timezone). From > To
// means it runs over midnight: it starts on the listed day and ends on the
// next one.
type Schedule struct {
	Days []string `json:"days"`
	From string   `json:"from"`
	To   string   `json:"to"`
}

// Device is a known device. MAC + VLAN identify it: the same MAC on another
// VLAN is a different (unknown, therefore child) device.
type Device struct {
	Name string `json:"name"`
	MAC  string `json:"mac"`
	VLAN int    `json:"vlan"`
	Role string `json:"role"`
	// Schedules replace the role's schedules when set (an empty list means
	// "no curfew for this device").
	Schedules *[]Schedule `json:"schedules,omitempty"`
	// StrictEgress overrides the role's setting when set.
	StrictEgress *bool `json:"strict_egress,omitempty"`
	// CurfewOverrideUntil lifts the curfew until this time ("30 more
	// minutes"); it expires by itself.
	CurfewOverrideUntil *time.Time `json:"curfew_override_until,omitempty"`
}

// VPN is the WireGuard server (wg0) for devices away from home.
type VPN struct {
	Enabled bool   `json:"enabled"`
	Port    int    `json:"port"`
	Subnet  string `json:"subnet"`
	// Endpoint is the address phones connect to (public IP or DNS name of
	// the ISP router, or the relay when behind CGNAT).
	Endpoint string `json:"endpoint"`
	// Exit is where peer traffic leaves: "mullvad" (wg-mullvad, kill
	// switch: no Mullvad = no internet for peers) or "wan" (home line).
	Exit  string `json:"exit"`
	Peers []Peer `json:"peers"`
}

// Peer is one WireGuard client. It has a role like a LAN device.
type Peer struct {
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
	Address   string `json:"address"`
	Role      string `json:"role"`
	// CurfewOverrideUntil: see Device.
	CurfewOverrideUntil *time.Time `json:"curfew_override_until,omitempty"`
}

// WiFi is the access point (hostapd).
type WiFi struct {
	Enabled bool   `json:"enabled"`
	Country string `json:"country"`
	// Band "a" (5 GHz) or "g" (2.4 GHz).
	Band    string `json:"band"`
	Channel int    `json:"channel"`
	SSIDs   []SSID `json:"ssids"`
	// ISPSSIDs are the ISP router's network names; seeing one on the air
	// raises an alert (the ISP Wi-Fi must be off).
	ISPSSIDs []string `json:"isp_ssids"`
}

// SSID is one network name, bridged into one VLAN. The passphrase is not
// here: custos-apply generates it on first use and keeps it in the secrets
// file (no default password).
type SSID struct {
	Name string `json:"name"`
	VLAN int    `json:"vlan"`
}

// AlertsConf holds the detection thresholds.
type AlertsConf struct {
	Bypass   BypassConf `json:"bypass"`
	Upload   UploadConf `json:"upload"`
	ISPWatch ISPWatch   `json:"isp_watch"`
}

// BypassConf: traffic above MinBytes with at most MaxDNS DNS packets in the
// window means the device probably resolves names elsewhere.
type BypassConf struct {
	WindowMinutes int   `json:"window_minutes"`
	MinBytes      int64 `json:"min_bytes"`
	MaxDNS        int64 `json:"max_dns"`
}

// UploadConf: more than MaxBytes sent by one device in the window.
type UploadConf struct {
	WindowMinutes int   `json:"window_minutes"`
	MaxBytes      int64 `json:"max_bytes"`
}

// ISPWatch scans the ISP router's LAN for devices other than the ISP
// router and Custos.
type ISPWatch struct {
	Enabled         bool     `json:"enabled"`
	IntervalMinutes int      `json:"interval_minutes"`
	IgnoreMACs      []string `json:"ignore_macs"`
}

// Default returns a complete configuration with safe values. It is what a
// new device starts with.
func Default() *Config {
	hagezi := func(name, file string, on bool) FilterList {
		return FilterList{Name: name, URL: "https://cdn.jsdelivr.net/gh/hagezi/dns-blocklists@latest/adblock/" + file, Enabled: on}
	}
	nightWeek := []Schedule{
		{Days: []string{"sun", "mon", "tue", "wed", "thu"}, From: "21:00", To: "07:00"},
		{Days: []string{"fri", "sat"}, From: "22:30", To: "08:00"},
	}
	return &Config{
		Version: 1,
		Main:    Main{Name: "Custos", Timezone: "Europe/Zagreb"},
		Network: Network{
			Bridge: "br-lan", WAN: "wan", AdminVLAN: 20,
			VLANs: []VLAN{{10, "djeca"}, {20, "odrasli"}, {30, "gosti"}},
		},
		Filtering: Filtering{
			SafeSearch:          true,
			UpdateIntervalHours: 24,
			Lists: []FilterList{
				hagezi("HaGeZi Multi PRO", "pro.txt", true),
				hagezi("HaGeZi Threat Intelligence Feeds (medium)", "tif.medium.txt", true),
				hagezi("HaGeZi NSFW", "nsfw.txt", true),
				hagezi("HaGeZi DoH/VPN/TOR/Proxy Bypass", "doh-vpn-proxy-bypass.txt", true),
				hagezi("HaGeZi Gambling", "gambling.txt", true),
				hagezi("HaGeZi Search engines without SafeSearch", "nosafesearch.txt", true),
				hagezi("HaGeZi Social Networks", "social.txt", false),
			},
			Rules: []string{},
		},
		Roles: RoleMap{
			RoleChild:      {Filtering: true, SafeSearch: true, BlockedServices: []string{}, StrictEgress: true, Schedules: nightWeek},
			RoleGuest:      {Filtering: true, SafeSearch: true, BlockedServices: []string{}, StrictEgress: false, Schedules: []Schedule{}},
			RoleTrusted:    {Filtering: true, SafeSearch: false, BlockedServices: []string{}, StrictEgress: false, Schedules: []Schedule{}},
			RoleUnfiltered: {Filtering: false, SafeSearch: false, BlockedServices: []string{}, StrictEgress: false, Schedules: []Schedule{}},
		},
		Devices: []Device{},
		VPN: VPN{
			Enabled: true, Port: 51820, Subnet: "192.168.40.0/24", Exit: "mullvad", Peers: []Peer{},
		},
		WiFi: WiFi{
			Enabled: true, Country: "HR", Band: "a", Channel: 36,
			SSIDs:    []SSID{{Name: "Custos-Djeca", VLAN: 10}, {Name: "Custos-Doma", VLAN: 20}},
			ISPSSIDs: []string{},
		},
		Alerts: AlertsConf{
			Bypass:   BypassConf{WindowMinutes: 10, MinBytes: 5 << 20, MaxDNS: 2},
			Upload:   UploadConf{WindowMinutes: 10, MaxBytes: 200 << 20},
			ISPWatch: ISPWatch{Enabled: true, IntervalMinutes: 5, IgnoreMACs: []string{}},
		},
	}
}

var (
	nameRe = regexp.MustCompile(`^[\p{L}\p{N} ._'()-]{1,40}$`)
	listRe = regexp.MustCompile(`^[\p{L}\p{N} ._'()/+&-]{1,80}$`)
	ssidRe = regexp.MustCompile(`^[A-Za-z0-9 ._-]{1,32}$`)
	hhmmRe = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
	ifRe   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,10}$`)
	keyRe  = regexp.MustCompile(`^[A-Za-z0-9+/]{42}[AEIMQUYcgkosw048]=$`)
	svcRe  = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)
)

// Days are the accepted day names (lower case, English, Go's order).
var Days = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

// Validate checks every field. Everything that ends up in a generated file
// (nft, networkd, hostapd, AdGuard Home) is checked against a strict
// pattern here, so generators never have to escape anything.
func (c *Config) Validate() error {
	var errs []error
	add := func(f string, a ...any) { errs = append(errs, fmt.Errorf(f, a...)) }

	if c.Version != 1 {
		add("version: %d is not supported (want 1)", c.Version)
	}
	if !nameRe.MatchString(c.Main.Name) {
		add("main.name: invalid")
	}
	if _, err := time.LoadLocation(c.Main.Timezone); err != nil {
		add("main.timezone: %v", err)
	}
	if !ifRe.MatchString(c.Network.Bridge) || !ifRe.MatchString(c.Network.WAN) {
		add("network: invalid interface name")
	}
	vlans := map[int]bool{}
	for i, v := range c.Network.VLANs {
		if v.ID < 1 || v.ID > 254 {
			add("network.vlans[%d].id: %d not in 1..254", i, v.ID)
		}
		if vlans[v.ID] {
			add("network.vlans[%d].id: duplicate %d", i, v.ID)
		}
		vlans[v.ID] = true
		if !nameRe.MatchString(v.Name) {
			add("network.vlans[%d].name: invalid", i)
		}
	}
	if !vlans[c.Network.AdminVLAN] {
		add("network.admin_vlan: %d is not a configured VLAN", c.Network.AdminVLAN)
	}

	if c.Filtering.UpdateIntervalHours != 0 && c.Filtering.UpdateIntervalHours != 1 &&
		c.Filtering.UpdateIntervalHours != 12 && c.Filtering.UpdateIntervalHours != 24 &&
		c.Filtering.UpdateIntervalHours != 72 && c.Filtering.UpdateIntervalHours != 168 {
		add("filtering.update_interval_hours: must be 0, 1, 12, 24, 72 or 168")
	}
	urls := map[string]bool{}
	for i, l := range c.Filtering.Lists {
		if !listRe.MatchString(l.Name) {
			add("filtering.lists[%d].name: invalid", i)
		}
		if !strings.HasPrefix(l.URL, "https://") || strings.ContainsAny(l.URL, " \t\n\"'") || len(l.URL) > 300 {
			add("filtering.lists[%d].url: must be an https URL", i)
		}
		if urls[l.URL] {
			add("filtering.lists[%d].url: duplicate", i)
		}
		urls[l.URL] = true
	}
	for i, r := range c.Filtering.Rules {
		if r == "" || len(r) > 300 || strings.ContainsAny(r, "\n\r") {
			add("filtering.rules[%d]: invalid", i)
		}
	}

	for _, name := range Roles {
		r, ok := c.Roles[name]
		if !ok {
			add("roles.%s: missing", name)
			continue
		}
		for _, s := range r.BlockedServices {
			if !svcRe.MatchString(s) {
				add("roles.%s.blocked_services: invalid %q", name, s)
			}
		}
		validateSchedules(r.Schedules, "roles."+name, add)
	}
	for name := range c.Roles {
		if !isRole(name) {
			add("roles.%s: unknown role", name)
		}
	}

	seen := map[string]bool{}
	names := map[string]bool{}
	for i, d := range c.Devices {
		p := fmt.Sprintf("devices[%d]", i)
		if !nameRe.MatchString(d.Name) {
			add("%s.name: invalid", p)
		}
		if names[strings.ToLower(d.Name)] {
			add("%s.name: duplicate %q", p, d.Name)
		}
		names[strings.ToLower(d.Name)] = true
		mac, err := NormalizeMAC(d.MAC)
		if err != nil {
			add("%s.mac: %v", p, err)
		}
		if !vlans[d.VLAN] {
			add("%s.vlan: %d is not a configured VLAN", p, d.VLAN)
		}
		k := fmt.Sprintf("%s/%d", mac, d.VLAN)
		if seen[k] {
			add("%s: duplicate MAC %s on VLAN %d", p, mac, d.VLAN)
		}
		seen[k] = true
		if !isRole(d.Role) {
			add("%s.role: unknown role %q", p, d.Role)
		}
		if d.Role == RoleTrusted && d.VLAN != c.Network.AdminVLAN {
			add("%s: role trusted is only allowed on the admin VLAN %d", p, c.Network.AdminVLAN)
		}
		if d.Schedules != nil {
			validateSchedules(*d.Schedules, p, add)
		}
	}

	if c.VPN.Port < 1024 || c.VPN.Port > 65535 {
		add("vpn.port: %d not in 1024..65535", c.VPN.Port)
	}
	subnet, err := netip.ParsePrefix(c.VPN.Subnet)
	if err != nil || !subnet.Addr().Is4() || subnet.Bits() < 16 || subnet.Bits() > 29 {
		add("vpn.subnet: must be an IPv4 /16../29 prefix")
	}
	if c.VPN.Exit != "mullvad" && c.VPN.Exit != "wan" {
		add("vpn.exit: must be \"mullvad\" or \"wan\"")
	}
	if c.VPN.Endpoint != "" && !regexp.MustCompile(`^[A-Za-z0-9.-]{1,253}$`).MatchString(c.VPN.Endpoint) {
		add("vpn.endpoint: must be a host name or IPv4 address")
	}
	pk := map[string]bool{}
	pa := map[string]bool{}
	for i, p := range c.VPN.Peers {
		f := fmt.Sprintf("vpn.peers[%d]", i)
		if !nameRe.MatchString(p.Name) {
			add("%s.name: invalid", f)
		}
		if names[strings.ToLower(p.Name)] {
			add("%s.name: duplicate %q", f, p.Name)
		}
		names[strings.ToLower(p.Name)] = true
		if !keyRe.MatchString(p.PublicKey) || pk[p.PublicKey] {
			add("%s.public_key: invalid or duplicate", f)
		}
		pk[p.PublicKey] = true
		a, err := netip.ParseAddr(p.Address)
		if err != nil || subnet.IsValid() && !subnet.Contains(a) || pa[p.Address] || a == subnet.Addr().Next() {
			add("%s.address: must be a free address in vpn.subnet (not the router's)", f)
		}
		pa[p.Address] = true
		if !isRole(p.Role) {
			add("%s.role: unknown role %q", f, p.Role)
		}
	}

	if c.WiFi.Country != "" && !regexp.MustCompile(`^[A-Z]{2}$`).MatchString(c.WiFi.Country) {
		add("wifi.country: two upper-case letters")
	}
	if c.WiFi.Band != "a" && c.WiFi.Band != "g" {
		add("wifi.band: \"a\" or \"g\"")
	}
	if c.WiFi.Channel < 1 || c.WiFi.Channel > 177 {
		add("wifi.channel: invalid")
	}
	ss := map[string]bool{}
	for i, s := range c.WiFi.SSIDs {
		if !ssidRe.MatchString(s.Name) || ss[s.Name] {
			add("wifi.ssids[%d].name: invalid or duplicate (A-Z a-z 0-9 space . _ -, max 32)", i)
		}
		ss[s.Name] = true
		if !vlans[s.VLAN] {
			add("wifi.ssids[%d].vlan: %d is not a configured VLAN", i, s.VLAN)
		}
	}
	if len(c.WiFi.SSIDs) > 4 {
		add("wifi.ssids: at most 4")
	}
	for i, s := range c.WiFi.ISPSSIDs {
		if s == "" || len(s) > 32 {
			add("wifi.isp_ssids[%d]: invalid", i)
		}
	}

	if c.Alerts.Bypass.WindowMinutes < 1 || c.Alerts.Upload.WindowMinutes < 1 || c.Alerts.ISPWatch.IntervalMinutes < 1 {
		add("alerts: window/interval must be at least 1 minute")
	}
	for i, m := range c.Alerts.ISPWatch.IgnoreMACs {
		if _, err := NormalizeMAC(m); err != nil {
			add("alerts.isp_watch.ignore_macs[%d]: %v", i, err)
		}
	}
	return errors.Join(errs...)
}

func validateSchedules(s []Schedule, p string, add func(string, ...any)) {
	for i, sc := range s {
		if !hhmmRe.MatchString(sc.From) || !hhmmRe.MatchString(sc.To) || sc.From == sc.To {
			add("%s.schedules[%d]: from/to must be different HH:MM times", p, i)
		}
		if len(sc.Days) == 0 {
			add("%s.schedules[%d].days: empty", p, i)
		}
		for _, d := range sc.Days {
			if DayIndex(d) < 0 {
				add("%s.schedules[%d].days: unknown day %q (sun..sat)", p, i, d)
			}
		}
	}
}

func isRole(r string) bool {
	for _, x := range Roles {
		if x == r {
			return true
		}
	}
	return false
}

// DayIndex returns time.Weekday for "sun".."sat", or -1.
func DayIndex(d string) int {
	for i, x := range Days {
		if x == d {
			return i
		}
	}
	return -1
}

// NormalizeMAC returns a lower-case, colon-separated unicast MAC.
func NormalizeMAC(s string) (string, error) {
	hw, err := net.ParseMAC(s)
	if err != nil || len(hw) != 6 {
		return "", fmt.Errorf("invalid MAC %q", s)
	}
	if hw[0]&1 == 1 {
		return "", fmt.Errorf("multicast MAC %q", s)
	}
	return hw.String(), nil
}

// VLANIf is the router's interface for a VLAN, e.g. "br-lan.10".
func (c *Config) VLANIf(id int) string { return fmt.Sprintf("%s.%d", c.Network.Bridge, id) }

// VLANGateway is the router's address on a VLAN.
func VLANGateway(id int) netip.Addr { return netip.AddrFrom4([4]byte{192, 168, byte(id), 1}) }

// LANIfs returns all VLAN interface names, sorted.
func (c *Config) LANIfs() []string {
	var r []string
	for _, v := range c.Network.VLANs {
		r = append(r, c.VLANIf(v.ID))
	}
	sort.Strings(r)
	return r
}

// RoleOf returns the policy for a role name (child for unknown names).
func (c *Config) RoleOf(name string) Role {
	if r, ok := c.Roles[name]; ok {
		return r
	}
	return c.Roles[RoleChild]
}

// Location returns the configured time zone.
func (c *Config) Location() *time.Location {
	l, err := time.LoadLocation(c.Main.Timezone)
	if err != nil {
		return time.UTC
	}
	return l
}

// Normalize lower-cases MACs and sorts nothing else; call after Load.
func (c *Config) Normalize() {
	for i := range c.Devices {
		if m, err := NormalizeMAC(c.Devices[i].MAC); err == nil {
			c.Devices[i].MAC = m
		}
	}
	for i := range c.Alerts.ISPWatch.IgnoreMACs {
		if m, err := NormalizeMAC(c.Alerts.ISPWatch.IgnoreMACs[i]); err == nil {
			c.Alerts.ISPWatch.IgnoreMACs[i] = m
		}
	}
}

// Parse decodes and validates custos.json. Unknown fields are an error, so
// a typo is reported instead of silently ignored.
func Parse(data []byte) (*Config, error) {
	c := &Config{}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(c); err != nil {
		return nil, fmt.Errorf("custos.json: %w", err)
	}
	c.Normalize()
	if c.Devices == nil {
		c.Devices = []Device{}
	}
	if c.VPN.Peers == nil {
		c.VPN.Peers = []Peer{}
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("custos.json is invalid:\n%w", err)
	}
	return c, nil
}

// Load reads and validates a file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Marshal returns the canonical, indented JSON.
func (c *Config) Marshal() ([]byte, error) {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// Save validates and writes atomically (temporary file + rename), so a
// crash never leaves a half-written custos.json.
func (c *Config) Save(path string, mode os.FileMode) error {
	if err := c.Validate(); err != nil {
		return err
	}
	b, err := c.Marshal()
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, b, mode)
}

// WriteFileAtomic writes data to path through a temporary file in the same
// directory, fsync and rename. Readers see the old or the new file, never a
// mix. The mode of an existing file is kept.
func WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
	return nil
}

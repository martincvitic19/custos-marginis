package apply

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/martincvitic19/custos-marginis/custos/internal/agh"
	"github.com/martincvitic19/custos-marginis/custos/internal/config"
	"github.com/martincvitic19/custos-marginis/custos/internal/render"
	"github.com/martincvitic19/custos-marginis/custos/internal/state"
)

// fakeAGH keeps just enough AdGuard Home state for Reconcile.
type fakeAGH struct {
	mu       sync.Mutex
	filters  []map[string]any
	rules    []string
	ss       agh.SafeSearch
	bs       []string
	clients  map[string]agh.AGHClient
	enabled  bool
	interval int
	writes   int
}

func (f *fakeAGH) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var body map[string]any
	raw, _ := io.ReadAll(r.Body)
	json.Unmarshal(raw, &body)
	if r.Method != http.MethodGet {
		f.writes++
	}
	out := func(v any) { json.NewEncoder(w).Encode(v) }
	switch strings.TrimPrefix(r.URL.Path, "/control") {
	case "/status":
		out(map[string]any{"running": true, "protection_enabled": true})
	case "/filtering/status":
		out(map[string]any{"enabled": f.enabled, "interval": f.interval, "filters": f.filters, "user_rules": f.rules})
	case "/filtering/config":
		f.enabled, f.interval = body["enabled"].(bool), int(body["interval"].(float64))
	case "/filtering/add_url":
		f.filters = append(f.filters, map[string]any{"url": body["url"], "name": body["name"], "enabled": true})
	case "/filtering/set_url":
		d := body["data"].(map[string]any)
		for _, x := range f.filters {
			if x["url"] == body["url"] {
				x["enabled"], x["name"] = d["enabled"], d["name"]
			}
		}
	case "/filtering/remove_url":
		var keep []map[string]any
		for _, x := range f.filters {
			if x["url"] != body["url"] {
				keep = append(keep, x)
			}
		}
		f.filters = keep
	case "/filtering/set_rules":
		f.rules = nil
		for _, x := range body["rules"].([]any) {
			f.rules = append(f.rules, x.(string))
		}
	case "/safesearch/status":
		out(f.ss)
	case "/safesearch/settings":
		json.Unmarshal(raw, &f.ss)
	case "/blocked_services/get":
		out(map[string]any{"ids": f.bs, "schedule": map[string]any{"time_zone": "Local"}})
	case "/blocked_services/update":
		var x struct{ IDs []string }
		json.Unmarshal(raw, &x)
		f.bs = x.IDs
	case "/clients":
		var l []agh.AGHClient
		for _, c := range f.clients {
			l = append(l, c)
		}
		out(map[string]any{"clients": l})
	case "/clients/add":
		var c agh.AGHClient
		json.Unmarshal(raw, &c)
		f.clients[c.Name] = c
	case "/clients/update":
		var u struct {
			Name string        `json:"name"`
			Data agh.AGHClient `json:"data"`
		}
		json.Unmarshal(raw, &u)
		delete(f.clients, u.Name)
		f.clients[u.Data.Name] = u.Data
	case "/clients/delete":
		delete(f.clients, body["name"].(string))
	default:
		http.NotFound(w, r)
	}
}

func setup(t *testing.T) (Options, *fakeAGH, *[]string) {
	t.Helper()
	root := t.TempDir()
	f := &fakeAGH{clients: map[string]agh.AGHClient{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	var cmds []string
	loc, _ := time.LoadLocation("Europe/Zagreb")
	now := time.Date(2026, 11, 16, 22, 0, 0, 0, loc) // Monday 22:00: child curfew
	o := Options{
		Mode: ModeFull, ConfigPath: config.DefaultPath, StatePath: state.StatePath, Secrets: state.SecretsPath,
		Root: root, AGH: agh.New(srv.URL + "/control"),
		Run:        func(n string, a ...string) error { cmds = append(cmds, n+" "+strings.Join(a, " ")); return nil },
		Now:        func() time.Time { return now },
		WiFiMAC:    func() string { return "04:f0:21:aa:bb:10" },
		EventsPath: "/data/custos/events.jsonl",
	}
	c := config.Default()
	c.Devices = []config.Device{
		{Name: "laptop", MAC: "02:00:5E:00:53:01", VLAN: 20, Role: config.RoleTrusted},
		{Name: "tablet", MAC: "02:11:22:33:44:55", VLAN: 10, Role: config.RoleChild},
	}
	c.VPN.Exit = "wan"
	c.VPN.Peers = []config.Peer{{Name: "phone", PublicKey: "hSDwCYkwp1R0i33ctD73Wg2/Og0mOBr066SpjqqbTmo=", Address: "192.168.40.2", Role: config.RoleChild}}
	p := root + config.DefaultPath
	os.MkdirAll(filepath.Dir(p), 0755)
	if err := c.Save(p, 0660); err != nil {
		t.Fatal(err)
	}
	return o, f, &cmds
}

// Two consecutive runs: the second one changes nothing (no files, no
// commands, no AdGuard Home writes).
func TestIdempotent(t *testing.T) {
	o, f, cmds := setup(t)
	st, err := Run(context.Background(), o)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if len(st.Changes) == 0 || f.writes == 0 {
		t.Fatalf("first run changed nothing: %v", st.Changes)
	}
	n, w := len(*cmds), f.writes
	st, err = Run(context.Background(), o)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(st.Changes) != 0 {
		t.Errorf("second run changes: %v", st.Changes)
	}
	if len(*cmds) != n {
		t.Errorf("second run ran commands: %v", (*cmds)[n:])
	}
	if f.writes != w {
		t.Errorf("second run wrote to AdGuard Home %d times", f.writes-w)
	}
}

func TestRendered(t *testing.T) {
	o, f, cmds := setup(t)
	if _, err := Run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	nft, _ := os.ReadFile(o.Root + NFTPath)
	s := string(nft)
	for _, want := range []string{
		`add element inet custos dev_trusted { 02:00:5e:00:53:01 . "br-lan.20" }`,
		`add element inet custos curfew_block { 02:11:22:33:44:55 . "br-lan.10" }`, // Monday 22:00
		`ether saddr . iifname != @dev_known counter drop comment "curfew: unknown device (child role)"`,
		`add element inet custos peer_curfew_block { 192.168.40.2 }`,
		`iifname "wg0" oifname "wan" accept`,
		`udp dport 51820 accept`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("nft missing %q", want)
		}
	}
	if strings.Contains(s, `curfew_block { c8:a3`) {
		t.Error("trusted device in curfew")
	}
	lease, err := os.ReadFile(o.Root + render.NetworkdDir + "/05-custos-br-lan.10.network.d/50-custos-leases.conf")
	if err != nil || !strings.Contains(string(lease), "MACAddress=02:11:22:33:44:55\nAddress=192.168.10.10") {
		t.Errorf("lease drop-in: %v\n%s", err, lease)
	}
	hp, err := os.ReadFile(o.Root + render.HostapdPath)
	if err != nil || !strings.Contains(string(hp), "ssid=Custos-Djeca") || !strings.Contains(string(hp), "ieee80211w=2") {
		t.Errorf("hostapd: %v", err)
	}
	if strings.Contains(string(hp), "wpa_passphrase=\n") {
		t.Error("empty Wi-Fi passphrase")
	}
	if _, ok := f.clients["custos: laptop"]; !ok {
		t.Error("AGH client for laptop missing")
	}
	if !f.ss.Enabled || len(f.rules) < 3 {
		t.Error("safesearch/rules not set")
	}
	joined := strings.Join(*cmds, "\n")
	if !strings.Contains(joined, "conntrack -D -f ipv4 -s 192.168.10.10") {
		t.Errorf("no conntrack flush for the tablet:\n%s", joined)
	}
	// Changing a role moves the device between sets; one more run.
	c, _ := config.Load(o.Root + config.DefaultPath)
	c.Devices[1].Role = config.RoleTrusted
	if err := c.Save(o.Root+config.DefaultPath, 0660); err == nil {
		t.Fatal("trusted on VLAN 10 must be rejected")
	}
}

func TestInvalidConfigIsRejected(t *testing.T) {
	o, _, _ := setup(t)
	os.WriteFile(o.Root+config.DefaultPath, []byte(`{"version":1,"devices":[{"name":"x;rm -rf","mac":"zz"}]}`), 0600)
	if _, err := Run(context.Background(), o); err == nil {
		t.Fatal("invalid custos.json accepted")
	}
}

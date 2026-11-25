// Package agh reconciles AdGuard Home with custos.json through its local
// HTTP API (127.0.0.1:3000, loopback only). It reads the current state
// first and sends only the differences, so a second run sends nothing and
// AdGuard Home is never restarted.
package agh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/martincvitic19/custos-marginis/custos/internal/config"
	"github.com/martincvitic19/custos-marginis/custos/internal/state"
)

// DefaultBase is the AdGuard Home API.
const DefaultBase = "http://127.0.0.1:3000/control"

// Client talks to the API.
type Client struct {
	Base string
	HTTP *http.Client
}

// New returns a client with a short timeout.
func New(base string) *Client {
	return &Client{Base: base, HTTP: &http.Client{Timeout: 15 * time.Second}}
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("adguardhome %s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(data)))
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// Get fetches a JSON endpoint (used by custosd and custos-watch too).
func (c *Client) Get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}

// SafeSearch mirrors filtering.SafeSearchConfig.
type SafeSearch struct {
	Enabled    bool `json:"enabled"`
	Bing       bool `json:"bing"`
	DuckDuckGo bool `json:"duckduckgo"`
	Ecosia     bool `json:"ecosia"`
	Google     bool `json:"google"`
	Pixabay    bool `json:"pixabay"`
	Yandex     bool `json:"yandex"`
	YouTube    bool `json:"youtube"`
}

func allEngines(on bool) SafeSearch {
	return SafeSearch{Enabled: on, Bing: true, DuckDuckGo: true, Ecosia: true, Google: true, Pixabay: true, Yandex: true, YouTube: true}
}

// AGHClient mirrors the persistent client JSON (only the fields Custos sets).
type AGHClient struct {
	Name                     string      `json:"name"`
	IDs                      []string    `json:"ids"`
	Tags                     []string    `json:"tags"`
	Upstreams                []string    `json:"upstreams"`
	BlockedServices          []string    `json:"blocked_services"`
	SafeSearch               *SafeSearch `json:"safe_search"`
	UseGlobalSettings        bool        `json:"use_global_settings"`
	UseGlobalBlockedServices bool        `json:"use_global_blocked_services"`
	FilteringEnabled         bool        `json:"filtering_enabled"`
	ParentalEnabled          bool        `json:"parental_enabled"`
	SafebrowsingEnabled      bool        `json:"safebrowsing_enabled"`
	SafesearchEnabled        bool        `json:"safesearch_enabled"`
	IgnoreQuerylog           bool        `json:"ignore_querylog"`
	IgnoreStatistics         bool        `json:"ignore_statistics"`
}

type filterJSON struct {
	URL     string `json:"url"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type filteringStatus struct {
	Enabled   bool         `json:"enabled"`
	Interval  int          `json:"interval"`
	Filters   []filterJSON `json:"filters"`
	UserRules []string     `json:"user_rules"`
}

// FixedRules are always present: they protect the enforcement itself.
var FixedRules = []string{
	// Firefox canary: NXDOMAIN tells Firefox not to switch on its own DoH.
	"||use-application-dns.net^$dnsrewrite=NXDOMAIN",
	// iCloud Private Relay: makes Apple devices use the normal path.
	"||mask.icloud.com^$dnsrewrite=NXDOMAIN",
	"||mask-h2.icloud.com^$dnsrewrite=NXDOMAIN",
}

// ClientName is the AdGuard Home client name for a device or peer. The
// prefix marks clients Custos owns; others (made in the AGH UI) are left
// alone.
func ClientName(name string) string { return "custos: " + name }

func roleTag(role string) string {
	switch role {
	case config.RoleChild:
		return "user_child"
	case config.RoleTrusted:
		return "user_admin"
	default:
		return "user_regular"
	}
}

// Desired builds the persistent clients for every known device and peer.
func Desired(c *config.Config, s *state.State) map[string]AGHClient {
	out := map[string]AGHClient{}
	mk := func(name, role string, ids []string) {
		r := c.RoleOf(role)
		bs := append([]string{}, r.BlockedServices...)
		sort.Strings(bs)
		sort.Strings(ids)
		out[ClientName(name)] = AGHClient{
			Name: ClientName(name), IDs: ids, Tags: []string{roleTag(role)}, Upstreams: []string{},
			BlockedServices: bs, SafeSearch: func() *SafeSearch { x := allEngines(r.SafeSearch); return &x }(),
			UseGlobalSettings: false, UseGlobalBlockedServices: false,
			FilteringEnabled: r.Filtering, SafesearchEnabled: r.SafeSearch,
		}
	}
	for _, d := range c.Devices {
		ids := []string{}
		if a := s.Leases[state.LeaseKey(d.MAC, d.VLAN)]; a != "" {
			ids = append(ids, a)
		}
		ids = append(ids, d.MAC)
		mk(d.Name, d.Role, ids)
	}
	for _, p := range c.VPN.Peers {
		mk(p.Name, p.Role, []string{p.Address})
	}
	return out
}

func sameClient(a, b AGHClient) bool {
	norm := func(x AGHClient) string {
		sort.Strings(x.IDs)
		sort.Strings(x.Tags)
		sort.Strings(x.BlockedServices)
		if x.SafeSearch == nil {
			x.SafeSearch = &SafeSearch{}
		}
		x.Upstreams = nil
		j, _ := json.Marshal(x)
		return string(j)
	}
	return norm(a) == norm(b)
}

// Result lists what a reconcile changed (empty: nothing).
type Result struct {
	Changes []string `json:"changes"`
}

func (r *Result) add(f string, a ...any) { r.Changes = append(r.Changes, fmt.Sprintf(f, a...)) }

// Reconcile makes AdGuard Home match custos.json. Global settings are the
// child policy (unknown devices are children); known devices get their
// role through persistent clients.
func (c *Client) Reconcile(ctx context.Context, cfg *config.Config, st *state.State) (*Result, error) {
	res := &Result{Changes: []string{}}

	// Filtering on, update interval.
	var fs filteringStatus
	if err := c.Get(ctx, "/filtering/status", &fs); err != nil {
		return nil, err
	}
	if !fs.Enabled || fs.Interval != cfg.Filtering.UpdateIntervalHours {
		if err := c.do(ctx, http.MethodPost, "/filtering/config", map[string]any{"enabled": true, "interval": cfg.Filtering.UpdateIntervalHours}, nil); err != nil {
			return nil, err
		}
		res.add("filtering: enabled, update every %dh", cfg.Filtering.UpdateIntervalHours)
	}

	// Block lists: add missing, update changed, remove ones not in custos.json.
	have := map[string]filterJSON{}
	for _, f := range fs.Filters {
		have[f.URL] = f
	}
	want := map[string]bool{}
	for _, l := range cfg.Filtering.Lists {
		want[l.URL] = true
		h, ok := have[l.URL]
		switch {
		case !ok:
			if err := c.do(ctx, http.MethodPost, "/filtering/add_url", map[string]any{"name": l.Name, "url": l.URL, "whitelist": false}, nil); err != nil {
				return nil, err
			}
			res.add("list added: %s", l.Name)
			if !l.Enabled {
				if err := c.setURL(ctx, l); err != nil {
					return nil, err
				}
			}
		case h.Enabled != l.Enabled || h.Name != l.Name:
			if err := c.setURL(ctx, l); err != nil {
				return nil, err
			}
			res.add("list updated: %s (enabled=%v)", l.Name, l.Enabled)
		}
	}
	for u, f := range have {
		if !want[u] {
			if err := c.do(ctx, http.MethodPost, "/filtering/remove_url", map[string]any{"url": u, "whitelist": false}, nil); err != nil {
				return nil, err
			}
			res.add("list removed: %s", f.Name)
		}
	}

	// User rules: fixed rules first, then custos.json rules.
	rules := append(append([]string{}, FixedRules...), cfg.Filtering.Rules...)
	if strings.Join(rules, "\n") != strings.Join(fs.UserRules, "\n") {
		if err := c.do(ctx, http.MethodPost, "/filtering/set_rules", map[string]any{"rules": rules}, nil); err != nil {
			return nil, err
		}
		res.add("user rules: %d", len(rules))
	}

	// Global SafeSearch = child role.
	child := cfg.RoleOf(config.RoleChild)
	var ss SafeSearch
	if err := c.Get(ctx, "/safesearch/status", &ss); err != nil {
		return nil, err
	}
	if wantSS := allEngines(cfg.Filtering.SafeSearch && child.SafeSearch); ss != wantSS {
		if err := c.do(ctx, http.MethodPut, "/safesearch/settings", wantSS, nil); err != nil {
			return nil, err
		}
		res.add("safesearch: %v", wantSS.Enabled)
	}

	// Global blocked services = child role.
	var gbs struct {
		IDs      []string        `json:"ids"`
		Schedule json.RawMessage `json:"schedule"`
	}
	if err := c.Get(ctx, "/blocked_services/get", &gbs); err != nil {
		return nil, err
	}
	wantBS := append([]string{}, child.BlockedServices...)
	sort.Strings(wantBS)
	sort.Strings(gbs.IDs)
	if strings.Join(wantBS, ",") != strings.Join(gbs.IDs, ",") {
		body := map[string]any{"ids": wantBS}
		if len(gbs.Schedule) > 0 {
			body["schedule"] = gbs.Schedule
		}
		if err := c.do(ctx, http.MethodPut, "/blocked_services/update", body, nil); err != nil {
			return nil, err
		}
		res.add("blocked services (child): %v", wantBS)
	}

	// Persistent clients.
	var cl struct {
		Clients []AGHClient `json:"clients"`
	}
	if err := c.Get(ctx, "/clients", &cl); err != nil {
		return nil, err
	}
	cur := map[string]AGHClient{}
	for _, x := range cl.Clients {
		if strings.HasPrefix(x.Name, "custos: ") {
			cur[x.Name] = x
		}
	}
	desired := Desired(cfg, st)
	names := make([]string, 0, len(desired))
	for n := range desired {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		d := desired[n]
		x, ok := cur[n]
		switch {
		case !ok:
			if err := c.do(ctx, http.MethodPost, "/clients/add", d, nil); err != nil {
				return nil, err
			}
			res.add("client added: %s", n)
		case !sameClient(x, d):
			if err := c.do(ctx, http.MethodPost, "/clients/update", map[string]any{"name": n, "data": d}, nil); err != nil {
				return nil, err
			}
			res.add("client updated: %s", n)
		}
	}
	for n := range cur {
		if _, ok := desired[n]; !ok {
			if err := c.do(ctx, http.MethodPost, "/clients/delete", map[string]any{"name": n}, nil); err != nil {
				return nil, err
			}
			res.add("client removed: %s", n)
		}
	}
	return res, nil
}

func (c *Client) setURL(ctx context.Context, l config.FilterList) error {
	return c.do(ctx, http.MethodPost, "/filtering/set_url", map[string]any{
		"url": l.URL, "whitelist": false,
		"data": map[string]any{"name": l.Name, "url": l.URL, "enabled": l.Enabled},
	}, nil)
}

// CheckHost asks AdGuard Home how a name would be filtered.
func (c *Client) CheckHost(ctx context.Context, name, client string) (map[string]any, error) {
	v := url.Values{"name": {name}}
	if client != "" {
		v.Set("client", client)
	}
	var out map[string]any
	err := c.Get(ctx, "/filtering/check_host?"+v.Encode(), &out)
	return out, err
}

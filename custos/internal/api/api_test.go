package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martincvitic19/custos-marginis/custos/internal/agh"
	"github.com/martincvitic19/custos-marginis/custos/internal/applyclient"
	"github.com/martincvitic19/custos-marginis/custos/internal/auth"
	"github.com/martincvitic19/custos-marginis/custos/internal/config"
)

func newServer(t *testing.T) (*httptest.Server, *int) {
	t.Helper()
	dir := t.TempDir()
	p := Paths{Config: filepath.Join(dir, "custos.json"), State: filepath.Join(dir, "state.json"), Secrets: filepath.Join(dir, "secrets.json"),
		Events: filepath.Join(dir, "events.jsonl"), ApplyStatus: filepath.Join(dir, "apply.json"), WGPub: filepath.Join(dir, "wg0.pub"), Wizard: filepath.Join(dir, "wizard.json")}
	if err := config.Default().Save(p.Config, 0660); err != nil {
		t.Fatal(err)
	}
	applies := 0
	s := &Server{Auth: auth.New(filepath.Join(dir, "auth.json")), AGH: agh.New("http://127.0.0.1:1/control"), P: p,
		Apply: func() (*applyclient.Status, error) { applies++; return &applyclient.Status{OK: true}, nil }}
	ts := httptest.NewTLSServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts, &applies
}

func req(t *testing.T, ts *httptest.Server, method, path, body string, hdr map[string]string) *http.Response {
	t.Helper()
	r, _ := http.NewRequest(method, ts.URL+path, bytes.NewBufferString(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	resp, err := ts.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestAPI(t *testing.T) {
	ts, applies := newServer(t)

	// No token: 401. State change without CSRF: 403.
	if r := req(t, ts, "GET", "/api/status", "", nil); r.StatusCode != 401 {
		t.Errorf("status without token: %d", r.StatusCode)
	}
	if r := req(t, ts, "POST", "/api/devices", "{}", nil); r.StatusCode != 403 {
		t.Errorf("POST without CSRF: %d", r.StatusCode)
	}
	// Security headers on everything.
	r := req(t, ts, "GET", "/", "", nil)
	for _, h := range []string{"Content-Security-Policy", "Strict-Transport-Security", "X-Frame-Options", "Referrer-Policy"} {
		if r.Header.Get(h) == "" {
			t.Errorf("missing header %s", h)
		}
	}
	if strings.Contains(r.Header.Get("Content-Security-Policy"), "unsafe-inline") {
		t.Error("CSP allows unsafe-inline")
	}

	// First run: set password (once), get a session.
	r = req(t, ts, "POST", "/api/setup", `{"password":"correct horse"}`, nil)
	if r.StatusCode != 200 {
		t.Fatalf("setup: %d", r.StatusCode)
	}
	var sess struct{ Token, CSRF string }
	json.NewDecoder(r.Body).Decode(&sess)
	if r := req(t, ts, "POST", "/api/setup", `{"password":"another one!"}`, nil); r.StatusCode != 409 {
		t.Errorf("second setup: %d", r.StatusCode)
	}
	bearer := map[string]string{"Authorization": "Bearer " + sess.Token}

	// Add a device through the API: validated, saved, applied.
	r = req(t, ts, "POST", "/api/devices", `{"name":"laptop","mac":"02:00:5E:00:53:01","vlan":20,"role":"trusted"}`, bearer)
	if r.StatusCode != 200 || *applies != 1 {
		t.Errorf("add device: %d, applies %d", r.StatusCode, *applies)
	}
	// Invalid input is rejected before anything is written.
	r = req(t, ts, "POST", "/api/devices", `{"name":"x\"; rm -rf /","mac":"zz","vlan":10,"role":"root"}`, bearer)
	if r.StatusCode != 400 || *applies != 1 {
		t.Errorf("invalid device: %d", r.StatusCode)
	}
	r = req(t, ts, "POST", "/api/devices", `{"name":"tablet","mac":"02:00:00:00:00:01","vlan":10,"role":"trusted"}`, bearer)
	if r.StatusCode != 400 {
		t.Errorf("trusted on child VLAN accepted: %d", r.StatusCode)
	}
	r = req(t, ts, "POST", "/api/devices", `{"name":"x","mac":"02:00:00:00:00:01","vlan":10,"role":"child","extra":1}`, bearer)
	if r.StatusCode != 400 {
		t.Errorf("unknown field accepted: %d", r.StatusCode)
	}

	// Cookie session: state change needs the matching CSRF token.
	cookie := map[string]string{"Cookie": "custos_session=" + sess.Token}
	if r := req(t, ts, "POST", "/api/apply", "{}", cookie); r.StatusCode != 403 {
		t.Errorf("cookie without CSRF: %d", r.StatusCode)
	}
	cookie["X-CSRF-Token"] = sess.CSRF
	if r := req(t, ts, "POST", "/api/apply", "{}", cookie); r.StatusCode != 200 {
		t.Errorf("cookie with CSRF: %d", r.StatusCode)
	}

	// Five wrong passwords, then 429 even with the right one.
	for i := 0; i < 5; i++ {
		req(t, ts, "POST", "/api/login", `{"password":"wrong"}`, nil)
	}
	if r := req(t, ts, "POST", "/api/login", `{"password":"correct horse"}`, nil); r.StatusCode != 429 {
		t.Errorf("6th login: %d, want 429", r.StatusCode)
	}

	// The production build has no demo route.
	if r := req(t, ts, "GET", "/api/demo/vulnerable-read?path=/etc/passwd", "", bearer); r.StatusCode == 200 && extraRoutes == nil {
		t.Error("demo route reachable in production build")
	}
}

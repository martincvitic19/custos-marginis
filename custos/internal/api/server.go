// Package api is custosd's HTTPS JSON API and the static web UI.
//
// Rules that hold for every handler:
//   - no shell, no exec: changes go to custos.json, custos-apply applies them
//   - every input is decoded into a typed struct and validated by
//     config.Validate before it is written
//   - unauthenticated: only /api/setup (first run), /api/login, the UI files
//   - state-changing requests with a session cookie need the CSRF header
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/martincvitic19/custos-marginis/custos/internal/agh"
	"github.com/martincvitic19/custos-marginis/custos/internal/alerts"
	"github.com/martincvitic19/custos-marginis/custos/internal/applyclient"
	"github.com/martincvitic19/custos-marginis/custos/internal/auth"
	"github.com/martincvitic19/custos-marginis/custos/internal/config"
	"github.com/martincvitic19/custos-marginis/custos/internal/ui"
)

// Paths custosd uses.
type Paths struct {
	Config      string // /data/custos/custos.json (rw, group custos)
	State       string // /data/custos/state.json (read: lease addresses)
	Secrets     string // /data/custos/secrets.json (read: Wi-Fi passwords)
	Events      string // /data/custos/events.jsonl
	ApplyStatus string // /run/custos/apply.json
	ApplySocket string
	WGPub       string // /data/custos/wg/wg0.pub
	Wizard      string // /data/custosd/wizard.json
	XDPStats    string // /run/custos/xdp-stats.json (written by custos-xdp)
}

// DefaultPaths are the device paths.
func DefaultPaths() Paths {
	return Paths{
		Config: config.DefaultPath, State: "/data/custos/state.json", Secrets: "/data/custos/secrets.json",
		Events: alerts.Path, ApplyStatus: "/run/custos/apply.json", ApplySocket: applyclient.Socket,
		WGPub: "/data/custos/wg/wg0.pub", Wizard: "/data/custosd/wizard.json",
		XDPStats: "/run/custos/xdp-stats.json",
	}
}

// Server holds the dependencies.
type Server struct {
	Auth  *auth.Store
	AGH   *agh.Client
	P     Paths
	Apply func() (*applyclient.Status, error)
	Start time.Time
	cfgMu sync.Mutex // serialises read-modify-write of custos.json
	mux   *http.ServeMux
}

// extraRoutes lets a build-tagged file add routes (see demo_*.go). It is
// nil in the production build.
var extraRoutes func(s *Server, mux *http.ServeMux)

// Handler returns the full handler with security headers.
func (s *Server) Handler() http.Handler {
	if s.Apply == nil {
		s.Apply = func() (*applyclient.Status, error) { return applyclient.Apply(s.P.ApplySocket) }
	}
	m := http.NewServeMux()
	s.mux = m
	// Public.
	m.HandleFunc("GET /api/setup", s.setupState)
	m.HandleFunc("POST /api/setup", s.setup)
	m.HandleFunc("POST /api/login", s.login)
	m.Handle("GET /", ui.Handler())

	// Authenticated.
	a := func(p string, h http.HandlerFunc) { m.Handle(p, s.authed(h)) }
	a("POST /api/logout", s.logout)
	a("POST /api/password", s.password)
	a("GET /api/status", s.status)
	a("GET /api/devices", s.devices)
	a("POST /api/devices", s.setDevice)
	a("DELETE /api/devices/{name}", s.deleteDevice)
	a("POST /api/devices/{name}/role", s.setRole)
	a("GET /api/roles", s.roles)
	a("PUT /api/roles/{role}", s.putRole)
	a("POST /api/curfew/override", s.override)
	a("GET /api/filtering", s.filtering)
	a("PUT /api/filtering", s.putFiltering)
	a("GET /api/filtering/check", s.check)
	a("GET /api/querylog", s.querylog)
	a("POST /api/apply", s.apply)
	a("GET /api/connection", s.connection)
	a("GET /api/events", s.events)
	a("GET /api/events.csv", s.eventsCSV)
	a("GET /api/vpn", s.vpn)
	a("PUT /api/vpn", s.putVPN)
	a("POST /api/vpn/peers", s.addPeer)
	a("DELETE /api/vpn/peers/{name}", s.deletePeer)
	a("GET /api/wifi", s.wifi)
	a("PUT /api/wifi", s.putWiFi)
	a("GET /api/wizard", s.wizard)
	a("PUT /api/wizard", s.putWizard)
	if extraRoutes != nil {
		extraRoutes(s, m)
	}
	return securityHeaders(m)
}

func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'; object-src 'none'")
		hd.Set("Strict-Transport-Security", "max-age=31536000")
		hd.Set("X-Frame-Options", "DENY")
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("Referrer-Policy", "no-referrer")
		hd.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			hd.Set("Cache-Control", "no-store")
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		h.ServeHTTP(w, r)
	})
}

// ---- helpers ---------------------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// decode reads a JSON body into v: application/json only (a cross-site
// HTML form cannot send it without a CORS preflight), unknown fields
// rejected.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		fail(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return false
	}
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

func clientIP(r *http.Request) string {
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

func readJSONFile(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// editConfig loads custos.json, lets fn change it, validates, saves and
// asks custos-apply to apply. Validation errors come back as 400.
func (s *Server) editConfig(w http.ResponseWriter, fn func(c *config.Config) error) bool {
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	c, err := config.Load(s.P.Config)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return false
	}
	if err := fn(c); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return false
	}
	if err := c.Validate(); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return false
	}
	if err := c.Save(s.P.Config, 0660); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return false
	}
	st, err := s.Apply()
	if err != nil {
		fail(w, http.StatusBadGateway, "saved, but apply failed: "+err.Error())
		return false
	}
	if !st.OK {
		fail(w, http.StatusBadGateway, "saved, but apply failed: "+st.Error)
		return false
	}
	return true
}

func (s *Server) loadConfig(w http.ResponseWriter) *config.Config {
	c, err := config.Load(s.P.Config)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return nil
	}
	return c
}

// ---- auth ------------------------------------------------------------------

type ctxKey int

const sessKey ctxKey = 0

// authed accepts "Authorization: Bearer <token>" (API clients) or the
// session cookie (UI). With the cookie, state-changing requests also need
// X-CSRF-Token: another site open in the parent's browser can make the
// browser send the cookie, but cannot read or send the token.
func (s *Server) authed(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		unsafe := r.Method != http.MethodGet && r.Method != http.MethodHead
		var sess *auth.Session
		if t, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
			sess = s.Auth.Lookup(t)
		} else {
			if unsafe {
				c, err := r.Cookie("custos_session")
				if err != nil || r.Header.Get("X-CSRF-Token") == "" {
					fail(w, http.StatusForbidden, "missing CSRF token")
					return
				}
				sess = s.Auth.Lookup(c.Value)
				if sess != nil && r.Header.Get("X-CSRF-Token") != sess.CSRF {
					fail(w, http.StatusForbidden, "bad CSRF token")
					return
				}
			} else if c, err := r.Cookie("custos_session"); err == nil {
				sess = s.Auth.Lookup(c.Value)
			}
		}
		if sess == nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="custos"`)
			fail(w, http.StatusUnauthorized, "login required")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), sessKey, sess)))
	})
}

func (s *Server) setupState(w http.ResponseWriter, r *http.Request) {
	var wz wizardState
	readJSONFile(s.P.Wizard, &wz)
	writeJSON(w, 200, map[string]any{"password_set": s.Auth.IsSet(), "wizard_done": wz.Done})
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	switch err := s.Auth.Setup(in.Password); {
	case errors.Is(err, auth.ErrAlreadySet):
		fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, auth.ErrWeak):
		fail(w, http.StatusBadRequest, err.Error())
	case err != nil:
		fail(w, http.StatusInternalServerError, err.Error())
	default:
		alerts.Append(s.P.Events, alerts.Event{Kind: alerts.KindLogin, Severity: "info", IP: clientIP(r), Message: "admin password set (first run)"})
		s.login(w, r.WithContext(context.WithValue(r.Context(), ctxKey(1), in.Password)))
	}
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	pw, _ := r.Context().Value(ctxKey(1)).(string)
	if pw == "" {
		var in struct {
			Password string `json:"password"`
		}
		if !decode(w, r, &in) {
			return
		}
		pw = in.Password
	}
	sess, err := s.Auth.Login(clientIP(r), pw)
	switch {
	case errors.Is(err, auth.ErrTooMany):
		w.Header().Set("Retry-After", "900")
		fail(w, http.StatusTooManyRequests, "too many failed logins, try again in 15 minutes")
		return
	case errors.Is(err, auth.ErrNotSet):
		fail(w, http.StatusConflict, "no password yet: run the setup first")
		return
	case err != nil:
		alerts.Append(s.P.Events, alerts.Event{Kind: alerts.KindLogin, Severity: "warning", IP: clientIP(r), Message: "failed login"})
		fail(w, http.StatusUnauthorized, "wrong password")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "custos_session", Value: sess.Token, Path: "/", HttpOnly: true,
		Secure: true, SameSite: http.SameSiteStrictMode, Expires: sess.Expires})
	writeJSON(w, 200, map[string]any{"token": sess.Token, "csrf": sess.CSRF, "expires": sess.Expires})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	s.Auth.Logout(r.Context().Value(sessKey).(*auth.Session).Token)
	http.SetCookie(w, &http.Cookie{Name: "custos_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) password(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.Auth.Change(in.Old, in.New); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) apply(w http.ResponseWriter, r *http.Request) {
	st, err := s.Apply()
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, 200, st)
}

// readAll is a small helper for proc/sys files.
func readAll(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	b, _ := io.ReadAll(io.LimitReader(f, 1<<20))
	return string(b)
}

func jsonMarshal(v any) ([]byte, error) { return json.MarshalIndent(v, "", "  ") }

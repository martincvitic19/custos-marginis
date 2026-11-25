//go:build vulnerable_demo

// This file exists only in custos-image-demo (build tag vulnerable_demo).
// It adds one method WITHOUT input validation on purpose, for scenario S3
// (A11): it shows that a compromised custosd still cannot read the
// WireGuard keys when SELinux confines it. Never in the production image;
// tests/A5-api.sh proves the production binary does not contain it.
package api

import (
	"net/http"
	"os"
)

func init() {
	extraRoutes = func(s *Server, mux *http.ServeMux) {
		mux.Handle("GET /api/demo/vulnerable-read", s.authed(func(w http.ResponseWriter, r *http.Request) {
			b, err := os.ReadFile(r.URL.Query().Get("path")) // no validation: deliberate
			if err != nil {
				fail(w, 500, err.Error())
				return
			}
			w.Write(b)
		}))
	}
}

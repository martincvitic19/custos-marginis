// custosd is the management API and web UI (HTTPS). It runs as the
// unprivileged user custosd (only CAP_NET_BIND_SERVICE) and listens only on
// the admin VLAN gateway address and the VPN address: children's and
// guests' VLANs cannot even connect. Changes are written to custos.json and
// applied by custos-apply (root) through /run/custos/apply.sock.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/martincvitic19/custos-marginis/custos/internal/agh"
	"github.com/martincvitic19/custos-marginis/custos/internal/api"
	"github.com/martincvitic19/custos-marginis/custos/internal/auth"
	"github.com/martincvitic19/custos-marginis/custos/internal/config"
	"github.com/martincvitic19/custos-marginis/custos/internal/tlscert"
)

// listenFreebind can bind an address that is not configured yet
// (IP_FREEBIND), so custosd does not depend on interface timing at boot.
func listenFreebind(addr string) (net.Listener, error) {
	lc := net.ListenConfig{Control: func(_, _ string, c syscall.RawConn) error {
		var serr error
		c.Control(func(fd uintptr) { serr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_FREEBIND, 1) })
		return serr
	}}
	return lc.Listen(context.Background(), "tcp4", addr)
}

func main() {
	dataDir := flag.String("data", "/data/custosd", "state directory (auth, TLS)")
	flag.Parse()
	log.SetFlags(0)

	cfg, err := config.Load(config.DefaultPath)
	if err != nil {
		// custos-apply creates custos.json on first boot; wait for it.
		log.Printf("custos.json not ready (%v); using defaults for addresses", err)
		cfg = config.Default()
	}
	addrs := []string{config.VLANGateway(cfg.Network.AdminVLAN).String()}
	ips := []net.IP{net.ParseIP(addrs[0])}
	if p, err := netip.ParsePrefix(cfg.VPN.Subnet); err == nil && cfg.VPN.Enabled {
		a := p.Addr().Next().String()
		addrs = append(addrs, a)
		ips = append(ips, net.ParseIP(a))
	}
	cert, err := tlscert.Ensure(*dataDir+"/tls", []string{"custos.local", "custos"}, ips)
	if err != nil {
		log.Fatalf("TLS certificate: %v", err)
	}
	srv := &api.Server{Auth: auth.New(*dataDir + "/auth.json"), AGH: agh.New(agh.DefaultBase), P: api.DefaultPaths(), Start: time.Now()}
	h := srv.Handler()
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}}

	var servers []*http.Server
	for _, a := range addrs {
		for _, port := range []string{"443", "8443"} {
			ln, err := listenFreebind(net.JoinHostPort(a, port))
			if err != nil {
				log.Fatalf("listen %s:%s: %v", a, port, err)
			}
			s := &http.Server{Handler: h, TLSConfig: tlsCfg, ReadHeaderTimeout: 10 * time.Second,
				ReadTimeout: 30 * time.Second, WriteTimeout: 3 * time.Minute, IdleTimeout: 2 * time.Minute}
			servers = append(servers, s)
			go func() { log.Println(s.ServeTLS(ln, "", "")) }()
			log.Printf("listening on https://%s:%s", a, port)
		}
		// Plain HTTP only redirects to HTTPS (http://custos.local works).
		ln, err := listenFreebind(net.JoinHostPort(a, "80"))
		if err != nil {
			log.Fatalf("listen %s:80: %v", a, err)
		}
		rs := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host, _, err := net.SplitHostPort(r.Host)
			if err != nil {
				host = r.Host
			}
			if host == "" || len(host) > 64 {
				host = "custos.local"
			}
			http.Redirect(w, r, "https://"+host+"/", http.StatusMovedPermanently)
		})}
		servers = append(servers, rs)
		go func() { log.Println(rs.Serve(ln)) }()
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	<-sig
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, s := range servers {
		s.Shutdown(ctx)
	}
}

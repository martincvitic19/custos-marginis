// Package cgnat finds out whether the home line has a public IPv4 address
// (inbound WireGuard works after a port forward on the ISP router) or sits
// behind carrier-grade NAT (it cannot work; use the relay variant, see
// docs/vpn.md).
package cgnat

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Result of a check.
type Result struct {
	PublicIP   string `json:"public_ip"`   // as seen from the internet
	RouterWAN  string `json:"router_wan"`  // ISP router's WAN address (UPnP), "" if unknown
	Verdict    string `json:"verdict"`     // "public", "cgnat", "unknown"
	Explanation string `json:"explanation"`
}

var shared = netip.MustParsePrefix("100.64.0.0/10") // RFC 6598, CGNAT

func private(a netip.Addr) bool { return a.IsPrivate() || shared.Contains(a) }

// Check runs the test. echoURL returns the caller's address as text.
func Check(ctx context.Context, echoURL string) Result {
	r := Result{Verdict: "unknown"}
	if ip, err := publicIP(ctx, echoURL); err == nil {
		r.PublicIP = ip.String()
	}
	if ip, err := upnpWAN(ctx); err == nil {
		r.RouterWAN = ip.String()
	}
	pub, perr := netip.ParseAddr(r.PublicIP)
	wan, werr := netip.ParseAddr(r.RouterWAN)
	switch {
	case werr == nil && private(wan):
		r.Verdict = "cgnat"
		r.Explanation = "the ISP router's WAN address is private/shared (" + r.RouterWAN + "): CGNAT, inbound VPN impossible; use a relay"
	case werr == nil && perr == nil && wan == pub:
		r.Verdict = "public"
		r.Explanation = "public IPv4 on the ISP router: forward UDP to Custos (or use DMZ)"
	case werr == nil && perr == nil:
		r.Verdict = "cgnat"
		r.Explanation = "ISP router WAN " + r.RouterWAN + " differs from the public address " + r.PublicIP + ": another NAT in between"
	case perr == nil && shared.Contains(pub):
		r.Verdict = "cgnat"
		r.Explanation = "the public address itself is in 100.64.0.0/10"
	case perr == nil:
		r.Explanation = "ISP router does not answer UPnP: compare " + r.PublicIP + " with the WAN address on its status page"
	default:
		r.Explanation = "no internet connection or echo service unreachable"
	}
	return r
}

func publicIP(ctx context.Context, u string) (netip.Addr, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return netip.Addr{}, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	return netip.ParseAddr(strings.TrimSpace(string(b)))
}

var locRe = regexp.MustCompile(`(?im)^location:\s*(\S+)`)

// upnpWAN asks the ISP router (UPnP IGD) for its external address.
func upnpWAN(ctx context.Context) (netip.Addr, error) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return netip.Addr{}, err
	}
	defer conn.Close()
	msg := "M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 2\r\n" +
		"ST: urn:schemas-upnp-org:service:WANIPConnection:1\r\n\r\n"
	dst := &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900}
	conn.SetDeadline(time.Now().Add(4 * time.Second))
	conn.WriteTo([]byte(msg), dst)
	buf := make([]byte, 2048)
	var loc string
	for loc == "" {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			return netip.Addr{}, fmt.Errorf("no UPnP answer")
		}
		if m := locRe.FindSubmatch(buf[:n]); m != nil {
			loc = string(m[1])
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, loc, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return netip.Addr{}, err
	}
	desc, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	resp.Body.Close()
	ctrl, svc := controlURL(desc)
	if ctrl == "" {
		return netip.Addr{}, fmt.Errorf("no WANIPConnection service")
	}
	base, _ := url.Parse(loc)
	cu, _ := base.Parse(ctrl)
	body := `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:GetExternalIPAddress xmlns:u="` + svc + `"/></s:Body></s:Envelope>`
	req, _ = http.NewRequestWithContext(ctx, http.MethodPost, cu.String(), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPAction", `"`+svc+`#GetExternalIPAddress"`)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return netip.Addr{}, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	m := regexp.MustCompile(`<NewExternalIPAddress>\s*([0-9.]+)\s*</NewExternalIPAddress>`).FindSubmatch(b)
	if m == nil {
		return netip.Addr{}, fmt.Errorf("no external address in answer")
	}
	return netip.ParseAddr(string(m[1]))
}

// controlURL finds the WANIPConnection (or WANPPPConnection) control URL in
// a device description.
func controlURL(desc []byte) (string, string) {
	d := xml.NewDecoder(bytes.NewReader(desc))
	var svcType, ctrl string
	var cur string
	for {
		t, err := d.Token()
		if err != nil {
			return "", ""
		}
		switch e := t.(type) {
		case xml.StartElement:
			cur = e.Name.Local
			if cur == "service" {
				svcType, ctrl = "", ""
			}
		case xml.CharData:
			switch cur {
			case "serviceType":
				svcType = strings.TrimSpace(string(e))
			case "controlURL":
				ctrl = strings.TrimSpace(string(e))
			}
		case xml.EndElement:
			cur = ""
			if e.Name.Local == "service" && ctrl != "" &&
				(strings.Contains(svcType, "WANIPConnection") || strings.Contains(svcType, "WANPPPConnection")) {
				return ctrl, svcType
			}
		}
	}
}

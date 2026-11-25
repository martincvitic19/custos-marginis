package apply

import (
	"fmt"
	"net/netip"

	"github.com/martincvitic19/custos-marginis/custos/internal/config"
	"github.com/martincvitic19/custos-marginis/custos/internal/wgkeys"
)

// NewPeer adds a VPN peer to cfg with a fresh key pair and the next free
// address, and returns the wg-quick configuration for the phone. The
// private key is shown once and never stored on the router.
func NewPeer(cfg *config.Config, name, role, serverPub string) (string, error) {
	priv, pub, err := wgkeys.Generate()
	if err != nil {
		return "", err
	}
	subnet := netip.MustParsePrefix(cfg.VPN.Subnet)
	used := map[string]bool{subnet.Addr().Next().String(): true}
	for _, p := range cfg.VPN.Peers {
		used[p.Address] = true
	}
	var addr netip.Addr
	for a := subnet.Addr().Next().Next(); subnet.Contains(a); a = a.Next() {
		if !used[a.String()] && a.As4()[3] != 255 {
			addr = a
			break
		}
	}
	if !addr.IsValid() {
		return "", fmt.Errorf("vpn.subnet is full")
	}
	cfg.VPN.Peers = append(cfg.VPN.Peers, config.Peer{Name: name, PublicKey: pub, Address: addr.String(), Role: role})
	if err := cfg.Validate(); err != nil {
		cfg.VPN.Peers = cfg.VPN.Peers[:len(cfg.VPN.Peers)-1]
		return "", err
	}
	return PeerConfig(cfg, priv, addr.String(), serverPub), nil
}

// PeerConfig is the wg-quick file a phone imports. DNS is the router: the
// forced-DNS rules apply to wg0 too. AllowedIPs 0.0.0.0/0: everything goes
// home (Android: Always-on VPN + "Block connections without VPN").
func PeerConfig(cfg *config.Config, priv, addr, serverPub string) string {
	subnet := netip.MustParsePrefix(cfg.VPN.Subnet)
	endpoint := cfg.VPN.Endpoint
	if endpoint == "" {
		endpoint = "SET-vpn.endpoint-IN-custos.json"
	}
	return fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s/32
DNS = %s

[Peer]
PublicKey = %s
Endpoint = %s:%d
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 25
`, priv, addr, subnet.Addr().Next(), serverPub, endpoint, cfg.VPN.Port)
}

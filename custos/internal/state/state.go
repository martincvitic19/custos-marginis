// Package state keeps what custos-apply derives once and must keep stable
// across runs: DHCP reservations and generated secrets. It is not
// configuration (custos.json) and nobody edits it by hand.
package state

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/netip"
	"os"
	"sort"

	"github.com/martincvitic19/custos-marginis/custos/internal/config"
)

// Paths on /data.
const (
	StatePath   = "/data/custos/state.json"
	SecretsPath = "/data/custos/secrets.json"
)

// State is /data/custos/state.json.
type State struct {
	// Leases maps "<mac>/<vlan>" to the reserved IPv4 address.
	Leases map[string]string `json:"leases"`
}

// Secrets is /data/custos/secrets.json (0640 root:custos): readable by
// custosd so the parent can see the Wi-Fi passwords in the UI.
type Secrets struct {
	WiFi map[string]string `json:"wifi"`
}

// LeaseKey identifies a device.
func LeaseKey(mac string, vlan int) string { return fmt.Sprintf("%s/%d", mac, vlan) }

// Reserved addresses are .10-.99; the DHCP pool is .100-.199 (custos-network).
const firstHost, lastHost = 10, 99

func loadJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func saveJSON(path string, v any, mode os.FileMode) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return config.WriteFileAtomic(path, append(b, '\n'), mode)
}

// Load reads the state (empty when missing).
func Load(path string) (*State, error) {
	s := &State{}
	if err := loadJSON(path, s); err != nil {
		return nil, err
	}
	if s.Leases == nil {
		s.Leases = map[string]string{}
	}
	return s, nil
}

// Save writes it atomically.
func (s *State) Save(path string) error { return saveJSON(path, s, 0640) }

// Assign gives every device a stable address in its VLAN and drops
// reservations of devices that no longer exist. It reports whether
// anything changed.
func (s *State) Assign(c *config.Config) (bool, error) {
	changed := false
	want := map[string]bool{}
	for _, d := range c.Devices {
		want[LeaseKey(d.MAC, d.VLAN)] = true
	}
	for k := range s.Leases {
		if !want[k] {
			delete(s.Leases, k)
			changed = true
		}
	}
	used := map[string]bool{}
	for _, a := range s.Leases {
		used[a] = true
	}
	devs := append([]config.Device(nil), c.Devices...)
	sort.Slice(devs, func(i, j int) bool { return devs[i].Name < devs[j].Name })
	for _, d := range devs {
		k := LeaseKey(d.MAC, d.VLAN)
		if a, ok := s.Leases[k]; ok {
			if p, err := netip.ParseAddr(a); err == nil && p.As4()[2] == byte(d.VLAN) {
				continue
			}
			delete(s.Leases, k)
		}
		found := false
		for h := firstHost; h <= lastHost; h++ {
			a := netip.AddrFrom4([4]byte{192, 168, byte(d.VLAN), byte(h)}).String()
			if !used[a] {
				s.Leases[k] = a
				used[a] = true
				found, changed = true, true
				break
			}
		}
		if !found {
			return changed, fmt.Errorf("VLAN %d: no free reserved address for %s", d.VLAN, d.Name)
		}
	}
	return changed, nil
}

// LoadSecrets reads the secrets file (empty when missing).
func LoadSecrets(path string) (*Secrets, error) {
	s := &Secrets{}
	if err := loadJSON(path, s); err != nil {
		return nil, err
	}
	if s.WiFi == nil {
		s.WiFi = map[string]string{}
	}
	return s, nil
}

// Save writes the secrets atomically.
func (s *Secrets) Save(path string) error { return saveJSON(path, s, 0640) }

// EnsureWiFi creates a random passphrase for every SSID that has none.
// There is never a default password.
func (s *Secrets) EnsureWiFi(c *config.Config) (bool, error) {
	changed := false
	for _, ss := range c.WiFi.SSIDs {
		if _, ok := s.WiFi[ss.Name]; ok {
			continue
		}
		p, err := Passphrase(4)
		if err != nil {
			return changed, err
		}
		s.WiFi[ss.Name] = p
		changed = true
	}
	return changed, nil
}

// Passphrase returns groups of 5 characters from an alphabet without
// look-alikes (no 0/O, 1/l/I), e.g. "k7pqe-3mzrw-...": about 25 bits per
// group, 100 bits for 4 groups.
func Passphrase(groups int) (string, error) {
	const alpha = "abcdefghijkmnpqrstuvwxyz23456789"
	out := make([]byte, 0, groups*6)
	for g := 0; g < groups; g++ {
		if g > 0 {
			out = append(out, '-')
		}
		for i := 0; i < 5; i++ {
			n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alpha))))
			if err != nil {
				return "", err
			}
			out = append(out, alpha[n.Int64()])
		}
	}
	return string(out), nil
}

package render

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/martincvitic19/custos-marginis/custos/internal/config"
	"github.com/martincvitic19/custos-marginis/custos/internal/state"
)

// TestNFTDump renders a policy with every feature on; with
// CUSTOS_NFT_DUMP=<file> it writes it out (used to load-test the ruleset
// in a network namespace).
func TestNFTDump(t *testing.T) {
	c := config.Default()
	c.Devices = []config.Device{
		{Name: "laptop", MAC: "02:00:5e:00:53:01", VLAN: 20, Role: config.RoleTrusted},
		{Name: "tablet", MAC: "02:11:22:33:44:55", VLAN: 10, Role: config.RoleChild},
	}
	c.VPN.Peers = []config.Peer{{Name: "phone", PublicKey: "hSDwCYkwp1R0i33ctD73Wg2/Og0mOBr066SpjqqbTmo=", Address: "192.168.40.2", Role: config.RoleChild}}
	loc, _ := time.LoadLocation("Europe/Zagreb")
	s := &state.State{Leases: map[string]string{}}
	s.Assign(c)
	out := NFT(Input{Cfg: c, State: s, Now: time.Date(2026, 11, 16, 22, 0, 0, 0, loc), MullvadUp: true, WGIf: "wg0"})
	if !strings.Contains(out, "wg-mullvad") || strings.Count(out, "flush chain") != 7 {
		t.Errorf("unexpected policy:\n%s", out)
	}
	if p := os.Getenv("CUSTOS_NFT_DUMP"); p != "" {
		os.WriteFile(p, []byte(out), 0644)
	}
}

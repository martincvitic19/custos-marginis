package config

import (
	"os"
	"testing"
)

// TestExample keeps docs/examples/custos.json valid; with
// CUSTOS_EXAMPLE=<file> it (re)writes it from Default() plus two devices.
func TestExample(t *testing.T) {
	c := Default()
	c.Devices = []Device{
		{Name: "laptop", MAC: "02:00:5e:00:53:01", VLAN: 20, Role: RoleTrusted},
		{Name: "Ivin tablet", MAC: "02:11:22:33:44:55", VLAN: 10, Role: RoleChild},
	}
	c.VPN.Endpoint = "203.0.113.10"
	b, err := c.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(b); err != nil {
		t.Fatal(err)
	}
	if p := os.Getenv("CUSTOS_EXAMPLE"); p != "" {
		os.WriteFile(p, b, 0644)
	}
}

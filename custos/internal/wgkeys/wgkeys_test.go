package wgkeys

import "testing"

// Test vector from RFC 7748 section 6.1 (Alice).
func TestPublicRFC7748(t *testing.T) {
	// a = 77076d0a7318a57d3c16c17251b26645df4c2f87ebc0992ab177fba51db92c2a
	priv := "dwdtCnMYpX08FsFyUbJmRd9ML4frwJkqsXf7pR25LCo="
	want := "hSDwCYkwp1R0i33ctD73Wg2/Og0mOBr066SpjqqbTmo="
	got, err := Public(priv)
	if err != nil || got != want {
		t.Fatalf("Public = %q, %v; want %q", got, err, want)
	}
}

func TestGenerate(t *testing.T) {
	priv, pub, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	p2, _ := Public(priv)
	if p2 != pub || len(pub) != 44 {
		t.Fatalf("mismatch %q %q", pub, p2)
	}
}

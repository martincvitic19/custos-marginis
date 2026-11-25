package watch

import (
	"testing"
	"time"

	"github.com/martincvitic19/custos-marginis/custos/internal/alerts"
	"github.com/martincvitic19/custos-marginis/custos/internal/config"
)

// Output of "nft -j list set inet custos acct_tx" on the board (nft 1.1.6).
const sample = `{"nftables": [{"metainfo": {"version": "1.1.6", "release_name": "Commodore Bullmoose #7", "json_schema_version": 1}}, {"set": {"family": "inet", "name": "acct", "table": "ctest", "type": ["ether_addr", "ifname"], "handle": 3, "size": 4096, "flags": ["dynamic"], "elem": [{"elem": {"val": {"concat": ["02:00:5e:00:53:01", "br-lan.20"]}, "counter": {"packets": 2, "bytes": 104}}}], "stmt": [{"counter": null}]}}]}`

func TestParse(t *testing.T) {
	b, p, err := parseNFTSet([]byte(sample))
	if err != nil || b["02:00:5e:00:53:01 . br-lan.20"] != 104 || p["02:00:5e:00:53:01 . br-lan.20"] != 2 {
		t.Fatalf("got %v %v %v", b, p, err)
	}
}

func TestBypassAndUpload(t *testing.T) {
	c := config.Default()
	c.Devices = []config.Device{{Name: "tablet", MAC: "02:00:00:00:00:01", VLAN: 10, Role: "child"}}
	w := New()
	var got []alerts.Event
	w.Emit = func(e alerts.Event) { got = append(got, e) }
	k := "02:00:00:00:00:01 . br-lan.10"
	t0 := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	// 12 minutes: 1 MB/min sent, no DNS -> bypass after the 10-minute window.
	for i := 0; i <= 12; i++ {
		w.Traffic(c, t0.Add(time.Duration(i)*time.Minute), map[string]int64{k: int64(i) << 20}, map[string]int64{k: 5})
	}
	if len(got) != 1 || got[0].Kind != alerts.KindBypass || got[0].Device != "tablet" {
		t.Fatalf("bypass alerts: %+v", got)
	}
	// Normal device: same traffic, but asks DNS all the time -> no alert.
	got = nil
	k2 := "02:00:00:00:00:02 . br-lan.10"
	for i := 0; i <= 12; i++ {
		w.Traffic(c, t0.Add(time.Duration(i)*time.Minute), map[string]int64{k2: int64(i) << 20}, map[string]int64{k2: int64(i * 20)})
	}
	if len(got) != 0 {
		t.Fatalf("false alarm: %+v", got)
	}
	// Large upload: 300 MB in 10 minutes (with DNS, so not a bypass).
	k3 := "02:00:00:00:00:03 . br-lan.20"
	for i := 0; i <= 10; i++ {
		w.Traffic(c, t0.Add(time.Duration(i)*time.Minute), map[string]int64{k3: int64(i) * 30 << 20}, map[string]int64{k3: int64(i * 50)})
	}
	if len(got) != 1 || got[0].Kind != alerts.KindUpload {
		t.Fatalf("upload alerts: %+v", got)
	}
}

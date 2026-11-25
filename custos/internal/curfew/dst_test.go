package curfew

import (
	"testing"
	"time"

	"github.com/martincvitic19/custos-marginis/custos/internal/config"
)

func zagreb(t *testing.T) *time.Location {
	t.Helper()
	l, err := time.LoadLocation("Europe/Zagreb")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	return l
}

var school = []config.Schedule{{Days: []string{"sun", "mon", "tue", "wed", "thu"}, From: "21:00", To: "07:00"}}

func TestOverMidnight(t *testing.T) {
	loc := zagreb(t)
	cases := []struct {
		when string
		want bool
	}{
		{"2026-11-16 20:59", false}, // Monday evening, before
		{"2026-11-16 21:00", true},  // Monday 21:00 starts
		{"2026-11-17 06:59", true},  // Tuesday morning, from Monday
		{"2026-11-17 07:00", false}, // ends
		{"2026-11-20 23:00", false}, // Friday: not a listed day
		{"2026-11-21 06:00", false}, // Saturday morning: Friday not listed
		{"2026-11-22 21:30", true},  // Sunday evening
	}
	for _, c := range cases {
		now, _ := time.ParseInLocation("2006-01-02 15:04", c.when, loc)
		if got := Active(school, now, loc); got != c.want {
			t.Errorf("%s: got %v, want %v", c.when, got, c.want)
		}
	}
}

// The schedule is wall-clock time. On both DST changes 21:00 local must
// still be the start, although the UTC offset differs by one hour.
func TestDST(t *testing.T) {
	loc := zagreb(t)
	// 2026-03-29: clocks go 02:00 -> 03:00 (CET -> CEST).
	// 2026-10-25: clocks go 03:00 -> 02:00 (CEST -> CET).
	for _, day := range []string{"2026-03-29", "2026-10-25"} { // both Sundays
		before, _ := time.ParseInLocation("2006-01-02 15:04", day+" 20:59", loc)
		start, _ := time.ParseInLocation("2006-01-02 15:04", day+" 21:00", loc)
		if Active(school, before, loc) {
			t.Errorf("%s 20:59 local: active, want inactive", day)
		}
		if !Active(school, start, loc) {
			t.Errorf("%s 21:00 local (%s UTC): inactive, want active", day, start.UTC().Format("15:04"))
		}
	}
	// The same UTC instant is 21:00 in winter but 22:00 in summer: a rule
	// fixed in UTC would be wrong in one of them.
	w := time.Date(2026, 1, 11, 20, 0, 0, 0, time.UTC) // Sun 21:00 CET
	s := time.Date(2026, 7, 12, 19, 0, 0, 0, time.UTC) // Sun 21:00 CEST
	if !Active(school, w, loc) || !Active(school, s, loc) {
		t.Error("21:00 local not active in winter or summer")
	}
	// Morning end across the autumn change (03:00 -> 02:00 happens inside
	// the window): still ends at 07:00 local.
	end, _ := time.ParseInLocation("2006-01-02 15:04", "2026-10-26 07:00", loc)
	if Active(school, end, loc) {
		t.Error("2026-10-26 07:00: still active")
	}
}

func TestOverride(t *testing.T) {
	loc := zagreb(t)
	c := config.Default()
	now, _ := time.ParseInLocation("2006-01-02 15:04", "2026-11-16 22:00", loc)
	until := now.Add(30 * time.Minute)
	d := config.Device{Name: "tablet", MAC: "02:00:00:00:00:01", VLAN: 10, Role: config.RoleChild}
	if !DeviceBlocked(c, d, now) {
		t.Fatal("child at 22:00 Monday not blocked")
	}
	d.CurfewOverrideUntil = &until
	if DeviceBlocked(c, d, now) {
		t.Error("override active, still blocked")
	}
	if !DeviceBlocked(c, d, now.Add(31*time.Minute)) {
		t.Error("override expired, not blocked")
	}
	d.Role = config.RoleTrusted
	d.CurfewOverrideUntil = nil
	if DeviceBlocked(c, d, now) {
		t.Error("trusted blocked")
	}
	if !UnknownBlocked(c, now) {
		t.Error("unknown device (child) not blocked")
	}
}

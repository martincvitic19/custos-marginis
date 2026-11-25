// Package curfew decides whether a schedule blocks internet access at a
// given moment.
//
// The decision is made here, in the configured time zone, and not with nft
// "meta hour"/"meta day" expressions: nft converts the hour to UTC when the
// rule is loaded, so a rule written in winter would be one hour off in
// summer. custos-apply re-evaluates every minute (custos-curfew.timer) and
// moves devices in and out of the nft set curfew_block.
package curfew

import (
	"strconv"
	"time"

	"github.com/martincvitic19/custos-marginis/custos/internal/config"
)

// minutes converts "HH:MM" (already validated) to minutes after midnight.
func minutes(hhmm string) int {
	h, _ := strconv.Atoi(hhmm[:2])
	m, _ := strconv.Atoi(hhmm[3:])
	return h*60 + m
}

func hasDay(days []string, wd time.Weekday) bool {
	for _, d := range days {
		if config.DayIndex(d) == int(wd) {
			return true
		}
	}
	return false
}

// Active reports whether any schedule is active at now, read as wall-clock
// time in loc. A window with From > To runs over midnight: it belongs to
// the day it starts on.
func Active(schedules []config.Schedule, now time.Time, loc *time.Location) bool {
	t := now.In(loc)
	cur := t.Hour()*60 + t.Minute()
	today := t.Weekday()
	yesterday := (today + 6) % 7
	for _, s := range schedules {
		from, to := minutes(s.From), minutes(s.To)
		if from < to {
			if hasDay(s.Days, today) && cur >= from && cur < to {
				return true
			}
			continue
		}
		// Over midnight: evening part today, morning part from yesterday.
		if hasDay(s.Days, today) && cur >= from {
			return true
		}
		if hasDay(s.Days, yesterday) && cur < to {
			return true
		}
	}
	return false
}

// Overridden reports whether a "more time" override is still running.
func Overridden(until *time.Time, now time.Time) bool {
	return until != nil && now.Before(*until)
}

// DeviceSchedules returns the schedules that apply to a device: its own
// when set, otherwise its role's.
func DeviceSchedules(c *config.Config, d config.Device) []config.Schedule {
	if d.Schedules != nil {
		return *d.Schedules
	}
	return c.RoleOf(d.Role).Schedules
}

// DeviceBlocked is the full decision for one known LAN device.
func DeviceBlocked(c *config.Config, d config.Device, now time.Time) bool {
	if d.Role == config.RoleUnfiltered || Overridden(d.CurfewOverrideUntil, now) {
		return false
	}
	return Active(DeviceSchedules(c, d), now, c.Location())
}

// PeerBlocked is the decision for one VPN peer (role schedules only).
func PeerBlocked(c *config.Config, p config.Peer, now time.Time) bool {
	if p.Role == config.RoleUnfiltered || Overridden(p.CurfewOverrideUntil, now) {
		return false
	}
	return Active(c.RoleOf(p.Role).Schedules, now, c.Location())
}

// UnknownBlocked is the decision for devices that are not in custos.json:
// they are children.
func UnknownBlocked(c *config.Config, now time.Time) bool {
	return Active(c.RoleOf(config.RoleChild).Schedules, now, c.Location())
}

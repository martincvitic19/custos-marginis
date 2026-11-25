// Package alerts is the event log: alerts from custos-watch (bypass, large
// upload, devices on the ISP side, ISP Wi-Fi, ARP spoofing) and changes
// from custos-apply. One JSON object per line in /data/custos/events.jsonl,
// kept for a limited time (GDPR: storage limitation). custosd shows it and
// exports it as CSV (incident report within 72 h, GDPR art. 33).
package alerts

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

// Path is the event log.
const Path = "/data/custos/events.jsonl"

// Retention: older events are dropped by Prune.
const Retention = 30 * 24 * time.Hour

// Kinds of events.
const (
	KindBypass   = "bypass"
	KindUpload   = "upload"
	KindISPSide  = "isp_device"
	KindISPWiFi  = "isp_wifi"
	KindARPSpoof = "arp_spoof"
	KindApply    = "apply"
	KindLogin    = "login"
)

// Event is one line.
type Event struct {
	Time     time.Time `json:"time"`
	Kind     string    `json:"kind"`
	Severity string    `json:"severity"` // info, warning, alert
	Device   string    `json:"device,omitempty"`
	MAC      string    `json:"mac,omitempty"`
	IP       string    `json:"ip,omitempty"`
	Message  string    `json:"message"`
}

// Append adds events (O_APPEND writes of single lines are atomic enough for
// two writers on one device).
func Append(path string, evs ...Event) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0660)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, e := range evs {
		if e.Time.IsZero() {
			e.Time = time.Now()
		}
		b, _ := json.Marshal(e)
		w.Write(append(b, '\n'))
	}
	return w.Flush()
}

// Read returns events newer than since, newest first, at most limit (0 =
// all).
func Read(path string, since time.Time, limit int) ([]Event, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return []Event{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var e Event
		if json.Unmarshal(sc.Bytes(), &e) == nil && !e.Time.Before(since) {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	if out == nil {
		out = []Event{}
	}
	return out, sc.Err()
}

// Prune rewrites the log without events older than the retention period.
func Prune(path string, now time.Time) error {
	evs, err := Read(path, now.Add(-Retention), 0)
	if err != nil {
		return err
	}
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].Time.Before(evs[j].Time) })
	tmp := path + ".prune"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0660)
	if err != nil {
		return err
	}
	for _, e := range evs {
		b, _ := json.Marshal(e)
		f.Write(append(b, '\n'))
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// CSV writes events as CSV (for a GDPR art. 33 notification to AZOP).
func CSV(w io.Writer, evs []Event) error {
	cw := csv.NewWriter(w)
	cw.Write([]string{"time", "kind", "severity", "device", "mac", "ip", "message"})
	for _, e := range evs {
		cw.Write([]string{e.Time.Format(time.RFC3339), e.Kind, e.Severity, e.Device, e.MAC, e.IP,
			strings.ReplaceAll(e.Message, "\n", " ")})
	}
	cw.Flush()
	return cw.Error()
}

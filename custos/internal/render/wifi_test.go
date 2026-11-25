package render

import (
	"strings"
	"testing"

	"github.com/martincvitic19/custos-marginis/custos/internal/config"
	"github.com/martincvitic19/custos-marginis/custos/internal/state"
)

func TestWiFi(t *testing.T) {
	c := config.Default()
	sec := &state.Secrets{WiFi: map[string]string{}}
	sec.EnsureWiFi(c)
	files := WiFi(c, sec, "04:f0:21:aa:bb:10")
	if len(files) != 3 {
		t.Fatalf("want 2 networkd files + hostapd.conf, got %d", len(files))
	}
	var h string
	for _, f := range files {
		if f.Path == HostapdPath {
			h = f.Content
		}
	}
	for _, want := range []string{"interface=wlan0\n", "bss=wlan0-1\nbssid=06:f0:21:aa:bb:11\n", "ssid=Custos-Djeca\n",
		"ssid=Custos-Doma\n", "ieee80211w=2\n", "country_code=HR\n", "ap_isolate=1\n"} {
		if !strings.Contains(h, want) {
			t.Errorf("hostapd.conf missing %q", want)
		}
	}
	if strings.Count(h, "wpa_passphrase=") != 2 || strings.Contains(h, "wpa_passphrase=\n") {
		t.Error("passphrases")
	}
	if testing.Verbose() {
		t.Log("\n" + h)
		t.Log("\n" + files[0].Content)
	}
	if WiFi(c, sec, "") != nil {
		t.Error("rendered without a radio")
	}
}

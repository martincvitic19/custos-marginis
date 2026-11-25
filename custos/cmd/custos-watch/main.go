// custos-watch raises alerts when someone tries to get around Custos (A9):
// per-device traffic without DNS (bypass), large uploads, devices on the
// ISP router's LAN, the ISP router's Wi-Fi, ARP spoofing on the LAN.
// It runs as user custoswatch with CAP_NET_RAW (raw ARP) and CAP_NET_ADMIN
// (read nft counters, Wi-Fi scan) only.
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/martincvitic19/custos-marginis/custos/internal/alerts"
	"github.com/martincvitic19/custos-marginis/custos/internal/config"
	"github.com/martincvitic19/custos-marginis/custos/internal/watch"
)

func main() {
	log.SetFlags(0)
	w := watch.New()
	stop := make(chan struct{})
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)

	cfg, err := config.Load(w.ConfigPath)
	for err != nil {
		log.Printf("waiting for custos.json: %v", err)
		time.Sleep(30 * time.Second)
		cfg, err = config.Load(w.ConfigPath)
	}
	w.ARPGuard(cfg, stop)

	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	var lastScan, lastWiFi, lastPrune time.Time
	gwMAC := ""
	for {
		now := time.Now()
		if c, err := config.Load(w.ConfigPath); err == nil {
			cfg = c
		}
		if tx, _, err := watch.NFTCounters("acct_tx"); err != nil {
			log.Print(err)
		} else if _, dns, err := watch.NFTCounters("acct_dns"); err != nil {
			log.Print(err)
		} else {
			w.Traffic(cfg, now, tx, dns)
		}
		if cfg.Alerts.ISPWatch.Enabled && now.Sub(lastScan) >= time.Duration(cfg.Alerts.ISPWatch.IntervalMinutes)*time.Minute {
			lastScan = now
			if m, err := w.ISPSide(cfg, now); err != nil {
				log.Printf("ISP side scan: %v", err)
			} else if m != "" {
				gwMAC = m
			}
		}
		if cfg.WiFi.Enabled && now.Sub(lastWiFi) >= 30*time.Minute {
			lastWiFi = now
			if err := w.ISPWiFi(cfg, gwMAC, now); err != nil {
				log.Printf("Wi-Fi scan: %v", err)
			}
		}
		if now.Sub(lastPrune) >= 24*time.Hour {
			lastPrune = now
			alerts.Prune(w.EventsPath, now)
		}
		select {
		case <-sig:
			close(stop)
			return
		case <-tick.C:
		}
	}
}

// custos-apply is the reconciler: custos.json -> AdGuard Home, nftables,
// DHCP reservations, WireGuard, hostapd. It runs as root, from systemd only:
//
//	custos-apply --mode=boot     before networking (custos-apply-boot.service)
//	custos-apply --mode=full     after AdGuard Home started (custos-apply.service)
//	custos-apply --mode=retry    every minute (custos-apply.timer): retries
//	                             and moves curfew boundaries
//	custos-apply --mode=socket   one request from custosd on stdin
//	                             (custos-apply.socket, Accept=yes)
//
// Tools for the console:
//
//	custos-apply add-peer <name> <role>   new VPN peer: config + QR code
//	custos-apply cgnat-check              public IPv4 or CGNAT?
//	custos-apply validate [file]          check a custos.json
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/martincvitic19/custos-marginis/custos/internal/apply"
	"github.com/martincvitic19/custos-marginis/custos/internal/cgnat"
	"github.com/martincvitic19/custos-marginis/custos/internal/config"
	"github.com/martincvitic19/custos-marginis/custos/internal/qrpng"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "add-peer":
			os.Exit(addPeer(os.Args[2:]))
		case "cgnat-check":
			r := cgnat.Check(context.Background(), "https://am.i.mullvad.net/ip")
			b, _ := json.MarshalIndent(r, "", "  ")
			fmt.Println(string(b))
			if r.Verdict == "cgnat" {
				fmt.Println("CGNAT detected")
				os.Exit(2)
			} else if r.Verdict == "public" {
				fmt.Println("public IPv4")
			}
			return
		case "validate":
			p := config.DefaultPath
			if len(os.Args) > 2 {
				p = os.Args[2]
			}
			if _, err := config.Load(p); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			fmt.Println("valid")
			return
		}
	}
	mode := flag.String("mode", apply.ModeFull, "boot, full, retry or socket")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if *mode == "socket" {
		// One line from custosd. The only request is "apply": custosd has
		// already written custos.json; we validate and apply it ourselves.
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		var req struct {
			Action string `json:"action"`
		}
		out := json.NewEncoder(os.Stdout)
		if json.Unmarshal([]byte(line), &req) != nil || req.Action != "apply" {
			out.Encode(map[string]any{"ok": false, "error": "unknown request"})
			return
		}
		st, _ := apply.Run(ctx, apply.DeviceOptions(apply.ModeFull))
		out.Encode(st)
		return
	}
	switch *mode {
	case apply.ModeBoot, apply.ModeFull, apply.ModeRetry:
	default:
		fmt.Fprintln(os.Stderr, "unknown mode", *mode)
		os.Exit(64)
	}
	opts := apply.DeviceOptions(*mode)
	if *mode == apply.ModeFull {
		opts.AGHWait = 90 * time.Second // custos-apply.service right after boot
	}
	st, err := apply.Run(ctx, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "custos-apply:", err)
		os.Exit(1)
	}
	if len(st.Changes) == 0 {
		fmt.Println("no changes")
		return
	}
	for _, c := range st.Changes {
		fmt.Println(c)
	}
}

func addPeer(args []string) int {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: custos-apply add-peer <name> <role: child|guest|trusted|unfiltered>")
		return 64
	}
	cfg, err := config.Load(config.DefaultPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	// Make sure the server key exists.
	if _, err := apply.Run(context.Background(), apply.DeviceOptions(apply.ModeFull)); err != nil {
		fmt.Fprintln(os.Stderr, "warning:", err)
	}
	pub, err := apply.WireGuardServerPublicKey("")
	if err != nil {
		fmt.Fprintln(os.Stderr, "no server key:", err)
		return 1
	}
	conf, err := apply.NewPeer(cfg, args[0], args[1], pub)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := cfg.Save(config.DefaultPath, 0660); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if _, err := apply.Run(context.Background(), apply.DeviceOptions(apply.ModeFull)); err != nil {
		fmt.Fprintln(os.Stderr, "apply:", err)
	}
	fmt.Print(conf)
	if q, err := qrpng.Terminal(conf); err == nil {
		fmt.Println()
		fmt.Print(q)
	}
	fmt.Println("\nThe private key above is not stored on Custos. Scan it now.")
	return 0
}

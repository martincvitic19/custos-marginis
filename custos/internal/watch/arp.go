// Package watch is custos-watch: it looks for attempts to get around Custos
// and raises alerts (events.jsonl -> UI overview).
//
//   - bypass: a device sends traffic but (almost) never asks Custos for DNS
//     -> it resolves names elsewhere (own DoH server, VPN over 443)
//   - upload: unusually large upload from one device (exfiltration, S18)
//   - isp_device: something other than the ISP router answers ARP on the
//     ISP router's LAN -> a device is connected around Custos
//   - isp_wifi: the ISP router's Wi-Fi is still on (needs the radio)
//   - arp_spoof: someone on a LAN VLAN claims the router's address
package watch

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"syscall"
	"time"
)

const ethPARP = 0x0806

func htons(v uint16) uint16 { return v<<8 | v>>8 }

// arpSocket opens an AF_PACKET socket for ARP on one interface.
func arpSocket(ifi *net.Interface) (int, error) {
	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, int(htons(ethPARP)))
	if err != nil {
		return -1, err
	}
	if err := syscall.Bind(fd, &syscall.SockaddrLinklayer{Protocol: htons(ethPARP), Ifindex: ifi.Index}); err != nil {
		syscall.Close(fd)
		return -1, err
	}
	return fd, nil
}

// arpPacket is a parsed ARP frame (Ethernet/IPv4 only).
type arpPacket struct {
	Op        uint16
	SenderMAC net.HardwareAddr
	SenderIP  netip.Addr
	TargetIP  netip.Addr
}

func parseARP(b []byte) (arpPacket, bool) {
	if len(b) < 42 || binary.BigEndian.Uint16(b[12:14]) != ethPARP {
		return arpPacket{}, false
	}
	a := b[14:]
	if binary.BigEndian.Uint16(a[0:2]) != 1 || binary.BigEndian.Uint16(a[2:4]) != 0x0800 || a[4] != 6 || a[5] != 4 {
		return arpPacket{}, false
	}
	return arpPacket{
		Op:        binary.BigEndian.Uint16(a[6:8]),
		SenderMAC: net.HardwareAddr(append([]byte(nil), a[8:14]...)),
		SenderIP:  netip.AddrFrom4([4]byte(a[14:18])),
		TargetIP:  netip.AddrFrom4([4]byte(a[24:28])),
	}, true
}

func arpRequest(src net.HardwareAddr, srcIP, dstIP netip.Addr) []byte {
	f := make([]byte, 42)
	copy(f[0:6], []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	copy(f[6:12], src)
	binary.BigEndian.PutUint16(f[12:14], ethPARP)
	a := f[14:]
	binary.BigEndian.PutUint16(a[0:2], 1)
	binary.BigEndian.PutUint16(a[2:4], 0x0800)
	a[4], a[5] = 6, 4
	binary.BigEndian.PutUint16(a[6:8], 1) // request
	copy(a[8:14], src)
	s4, d4 := srcIP.As4(), dstIP.As4()
	copy(a[14:18], s4[:])
	copy(a[24:28], d4[:])
	return f
}

func ifaceIPv4(ifi *net.Interface) (netip.Prefix, error) {
	as, err := ifi.Addrs()
	if err != nil {
		return netip.Prefix{}, err
	}
	for _, a := range as {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
			ones, _ := n.Mask.Size()
			ip, _ := netip.AddrFromSlice(n.IP.To4())
			return netip.PrefixFrom(ip, ones), nil
		}
	}
	return netip.Prefix{}, fmt.Errorf("%s has no IPv4 address", ifi.Name)
}

// ScanResult is one host on the ISP side.
type ScanResult struct {
	IP  netip.Addr
	MAC string
}

// ARPScan asks every address of the interface's /24 (or smaller) subnet
// and returns who answered. Raw ARP: works although the firewall drops all
// inbound traffic on wan, and finds hosts that ignore ping.
func ARPScan(ifname string, wait time.Duration) ([]ScanResult, netip.Prefix, error) {
	ifi, err := net.InterfaceByName(ifname)
	if err != nil {
		return nil, netip.Prefix{}, err
	}
	pfx, err := ifaceIPv4(ifi)
	if err != nil {
		return nil, pfx, err
	}
	if pfx.Bits() < 24 {
		pfx = netip.PrefixFrom(pfx.Addr(), 24) // never sweep more than 254 hosts
	}
	fd, err := arpSocket(ifi)
	if err != nil {
		return nil, pfx, err
	}
	defer syscall.Close(fd)
	to := &syscall.SockaddrLinklayer{Protocol: htons(ethPARP), Ifindex: ifi.Index, Halen: 6}
	copy(to.Addr[:], []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	for a := pfx.Masked().Addr().Next(); pfx.Contains(a); a = a.Next() {
		if a == pfx.Addr() || a.As4()[3] == 255 {
			continue
		}
		syscall.Sendto(fd, arpRequest(ifi.HardwareAddr, pfx.Addr(), a), 0, to)
		time.Sleep(2 * time.Millisecond) // ~0.5 s for a /24, gentle on the ISP router
	}
	tv := syscall.NsecToTimeval(int64(200 * time.Millisecond))
	syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv)
	seen := map[netip.Addr]string{}
	deadline := time.Now().Add(wait)
	buf := make([]byte, 128)
	for time.Now().Before(deadline) {
		n, _, err := syscall.Recvfrom(fd, buf, 0)
		if err != nil {
			continue
		}
		p, ok := parseARP(buf[:n])
		if !ok || p.Op != 2 || !pfx.Contains(p.SenderIP) || p.SenderIP == pfx.Addr() {
			continue
		}
		seen[p.SenderIP] = p.SenderMAC.String()
	}
	var out []ScanResult
	for ip, mac := range seen {
		out = append(out, ScanResult{ip, mac})
	}
	return out, pfx, nil
}

// ListenARP calls fn for every ARP packet on the interface until stop is
// closed (passive: sends nothing).
func ListenARP(ifname string, stop <-chan struct{}, fn func(arpPacket)) error {
	ifi, err := net.InterfaceByName(ifname)
	if err != nil {
		return err
	}
	fd, err := arpSocket(ifi)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)
	tv := syscall.NsecToTimeval(int64(time.Second))
	syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv)
	buf := make([]byte, 128)
	for {
		select {
		case <-stop:
			return nil
		default:
		}
		n, _, err := syscall.Recvfrom(fd, buf, 0)
		if err != nil {
			continue
		}
		if p, ok := parseARP(buf[:n]); ok {
			fn(p)
		}
	}
}

# Live demo on one image: switch off, show, switch on

Every "off" below is a run-time change (nftables, /run, a stopped service).
Nothing is written to the root file system, so **a reboot always restores
the hardened state**. Lab only: your own devices, invented data.

Status column:

- **tested**: the hardened behaviour was seen with real traffic or a real attempt.
- **rehearsed**: the demo itself was run.
- **syntax**: the rule passed `nft -c`; the off/on cycle was not run.
- **untested**: proposal, try it before recording.

| S | Scenario | Off switch | Status |
|---|---|---|---|
| S1 | Password guessing | stop sshguard | on: tested |
| S2 | Management from child VLAN | nft rule | syntax |
| S3 | Compromised custosd | demo image + SELinux | untested |
| S4 | Lateral movement | nft rule | syntax |
| S5 | Forged update | `--no-verify` | rehearsed |
| S6 | Broken update | none, staged failure | untested with this bundle |
| S7 | Modified rootfs | none, works on a copy | rehearsed |
| S8 | Kernel module | none (one-way until reboot) | untested |
| S9 | Unknown device | nft rule | on: tested, off: syntax |
| S10 | Curfew | nft set element | syntax |
| S11 | DNS bypass | nft rules | on: tested, off: syntax |
| S12 | VPN / Tor from a child | nft rule | syntax |
| S13 | WAN flood | stop XDP | drop logic tested with synthetic packets |
| S14 | ARP spoofing | stop two services | untested |
| S15 | Silent key access | unload audit rules | untested |
| S16 | Weak Wi-Fi | edit the run-time hostapd.conf | untested |
| S17 | Stolen SD card | no protection yet | shows the gap |
| S18 | Phishing and exfiltration | S11 + S4 + stop watch | untested |

## Before recording

On the PC, from the repository root:

    alias B='ssh -i keys/espressobin_ed25519 root@192.168.20.1'
    sudo ip route add default via 192.168.20.1 dev <usb-ethernet> metric 50
    ip route get 1.1.1.1          # must say "via 192.168.20.1"

On the board (`B`), so that the timer does not rewrite demo rules:

    systemctl stop custos-apply.timer

Do not change settings in the web UI during a demo (it triggers custos-apply).
Do not show keys, /data/custos/secrets.json, the Wi-Fi passphrases or your
public IP on screen. Keep a serial console open for S1, S2 and S4.

## Switch everything on again

    nft -f /etc/custos/firewall.nft       # atomic, removes every demo rule
    auditctl -R /etc/audit/audit.rules
    systemctl start custos-apply.timer custos-xdp custos-sshguard custos-watch custos-suricata custos-arpwatch@20

or simply `reboot`. Then prove it: `tests/run-all.sh`.

Never use `systemctl restart custos-firewall`: networkd and the SSH socket
require it and are restarted with it.

The nft commands below are typed on the board, where the quotes are needed
exactly as written.

## 1. Network edge

### S4 Lateral movement (two clients: lan0 = child, lan1 = adult)

Attack (child): reach a service on the adult PC, e.g. `nc -zv 192.168.20.2 8000`
with `python3 -m http.server 8000` running on the adult PC.

    # off
    nft 'insert rule inet custos forward iifname "br-lan.10" oifname "br-lan.20" accept comment "demo"'
    # on
    nft -f /etc/custos/firewall.nft

Off: connects. On: no answer, the "forward: inter-VLAN" counter rises.

### S13 WAN flood and spoofed sources

Attack: SYN flood or bogon sources from a lab host on the WAN side (standard
packet generator such as hping3), never through the ISP.

    systemctl stop custos-xdp             # off
    systemctl start custos-xdp            # on
    custos-xdp stats                      # drop counters

Off: nftables still drops, but at a higher CPU cost (show `top`).
On: the bogon/syn counters rise. The SYN limit is 200 per second per source.

### S16 Weak Wi-Fi (RED, EN 18031)

Attack: only against your own SSID: show the security type in a Wi-Fi scanner.

    # off
    cp /run/custos/hostapd.conf /run/custos/hostapd.demo-backup
    sed -i -e 's/^ieee80211w=2/ieee80211w=0/' -e 's/^wpa_key_mgmt=SAE.*/wpa_key_mgmt=WPA-PSK/' /run/custos/hostapd.conf
    systemctl restart custos-hostapd
    # on
    mv /run/custos/hostapd.demo-backup /run/custos/hostapd.conf
    systemctl restart custos-hostapd

## 2. Child policy (a PC that is not in custos.json has the child role)

### S9 Unknown device gets adult rights

Attack (PC): `ping -c2 1.1.1.1`, `nc -zv -w3 github.com 22`

    # off
    nft 'insert rule inet custos roles iifname "br-lan.20" return comment "demo"'
    # on
    nft -f /etc/custos/firewall.nft

Off: both work. On: both fail, HTTPS still works.

### S10 Curfew

Attack (PC): keep browsing. SSH to the board keeps working (only forwarded
traffic is blocked). Use the PC's MAC on the USB adapter.

    # curfew on
    nft 'add element inet custos curfew_block { <pc-mac> . "br-lan.20" }'
    # curfew off
    nft 'delete element inet custos curfew_block { <pc-mac> . "br-lan.20" }'

### S11 DNS filter bypass

Attack (PC):

    dig @8.8.8.8 pagead2.googlesyndication.com +short
    nc -zv -w3 1.1.1.1 853
    curl -sI --max-time 5 https://1.1.1.1

    # off: all three rules are needed, child egress blocks 853 by itself
    nft 'insert rule inet custos dstnat iifname "br-lan.20" return comment "demo"'
    nft 'insert rule inet custos dns_guard iifname "br-lan.20" return comment "demo"'
    nft 'insert rule inet custos roles iifname "br-lan.20" return comment "demo"'
    # on
    nft -f /etc/custos/firewall.nft

Off: real address, port open, DoH answers. On: 0.0.0.0, refused, reset.

### S12 VPN or Tor from a child device

Attack (PC): `nc -zv -w3 <any host> 9001` (Tor relay port) or an OpenVPN client.

Off/on: the same rule as S9. On: dropped, and Suricata logs the attempt:

    tail -n1 /data/suricata/eve.json | jq .alert.signature

Limit to say aloud: a tunnel over 443 is at best detected.

## 3. Management

### S1 Password guessing

Attack from the board to itself, so the PC is not locked out for 10 minutes:

    for i in 1 2 3 4 5; do timeout 6 dbclient -y -y -l nobody-demo 192.168.20.1 true </dev/null; echo "attempt $i rc=$?"; sleep 2; done

    systemctl stop custos-sshguard        # off: all five answered (rc=1)
    systemctl start custos-sshguard       # on: the 4th times out (rc=124)
    nft list set ip sshguard attackers    # show the blocked address
    systemctl restart custos-sshguard     # unblock

### S2 Management from the child VLAN

Attack (client on lan0): `nc -zv -w3 192.168.10.1 22`

    # off
    nft 'insert rule inet custos input iifname "br-lan.10" tcp dport { 22, 443, 8443 } accept comment "demo"'
    # on
    nft -f /etc/custos/firewall.nft

The web UI does not listen on 192.168.10.1 at all, so use port 22.

## 4. Containment

### S3 Compromised custosd reads the WireGuard key

The only scenario that needs another image: `custos-image-demo` has one
deliberately vulnerable API method. It is otherwise the hardened image, but
there is no bundle recipe for it yet and it has never been built.

Two layers protect the key: file permissions and SELinux. To show SELinux,
first remove the first layer (this is what a UID mix-up did on 2026-11-22):

    mkdir -p /run/systemd/system/custosd.service.d
    printf '[Service]\nSupplementaryGroups=systemd-network\n' > /run/systemd/system/custosd.service.d/zz-demo.conf
    systemctl daemon-reload; systemctl restart custosd
    setenforce 0        # off: the method returns the key (use a throw-away key)
    setenforce 1        # on: denied, "avc: denied ... custos_wg_key_t" in the journal
    # clean up
    rm /run/systemd/system/custosd.service.d/zz-demo.conf; systemctl daemon-reload; systemctl restart custosd

Without the demo image, show the policy decision only:

    compute_av system_u:system_r:custosd_t:s0 system_u:object_r:custos_wg_key_t:s0 file

## 5. Platform integrity

### S7 Modified root file system (on the PC, on a copy)

Never remount the active slot read-write: the first write changes its root
hash for good. On the board show `touch /usr/x` (read-only) and
`/usr/lib/custos/custos-verity-check` (matches). On the PC:

    V=$(ls build/espressobin-wrynose/build/tmp/work/*/custos-image/1.0/recipe-sysroot-native/usr/sbin/veritysetup)
    head -c 1048576 /dev/urandom > data; : > hash
    H=$($V format data hash | sed -n 's/^Root hash:[[:space:]]*//p')
    $V verify data hash $H                                  # OK
    printf X | dd of=data bs=1 seek=4242 conv=notrunc
    $V verify data hash $H                                  # verification failed

### S8 Malicious kernel module

Lockdown and signature enforcement cannot be switched off at run time, which
is the point: show that root cannot do it either.

    cat /sys/kernel/security/lockdown
    echo none > /sys/kernel/security/lockdown    # refused
    insmod /tmp/unsigned.ko                      # rejected (bring your own test module)

### S17 Stolen SD card

No protection until the TPM exists: /data is readable in any card reader.
Show it with invented data only, as the reason for W-TPM.

## 6. Detection

### S14 ARP spoofing

Attack: ARP spoofing tool (dsniff package) between two of your own lab hosts.

    systemctl stop custos-arpwatch@20 custos-watch     # off: silence
    systemctl start custos-arpwatch@20 custos-watch    # on
    journalctl -b -u custos-arpwatch@20 | tail         # and "arp_spoof" in the UI

Detection only: Custos does not prevent it.

### S15 Silent access to keys

Attack: `cat /data/custos/wg/wg0.key > /dev/null`

    auditctl -D                            # off: no trace
    auditctl -R /etc/audit/audit.rules     # on
    ausearch -k custos_wg_keys | tail      # who, when, which program

### S18 Phishing and exfiltration

Off: S11 + S4 + `systemctl stop custos-watch`. On: the restore block at the
top. Steps and limits: [S18.md](S18.md).

## 7. Updates

### S5 Forged update bundle

On the PC serve the wrongly signed bundle, then look at it, without installing:

    python3 tools/serve-bundle.py <path>/evil.raucb 38471 &
    ssh -i keys/espressobin_ed25519 -R 127.0.0.1:38471:127.0.0.1:38471 root@192.168.20.1
    rauc info --no-verify http://127.0.0.1:38471/bundle.raucb   # off: looks like a normal update
    rauc info http://127.0.0.1:38471/bundle.raucb               # on: "signature verification failed"

### S6 Broken update

Takes about 15 minutes, record it separately. Install the good bundle into
the inactive slot, reboot, and in each of the next boots run
`systemctl stop custosd` within 3 minutes. The health check fails, the board
reboots, and after three tries it returns to the previous slot.

## Tools for the attacker PC

dig, nc, nmap, curl and iperf3 are common. For S13 and S14 also install, in
the lab only: `sudo apt install hping3 dsniff iputils-arping`.

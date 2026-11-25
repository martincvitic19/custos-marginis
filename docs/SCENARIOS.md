# Custos Marginis: attack scenarios

What the hardened image protects against, how that is shown, and where the
protection ends. S3, S11, S16, S17 and S18 come from the project plan; the
other numbers are a proposal and may still change.

## How to read "evidence"

- **config**: the rule or setting is loaded on the device. No attack traffic
  was sent, so this alone does not prove that an attack is blocked.
- **live**: the state of the running system was measured.
- **traffic**: needs real traffic from a lab host.

`tests/demo/S<N>-verify.sh` only reads the device state. The attack side is
run by hand in an isolated lab and is described here by technique only.
How to show each scenario on one image: [demo/LIVE-DEMO.md](demo/LIVE-DEMO.md).
The baseline (unprotected) column has not been measured yet.

## Scenarios by functional area

### 1. Network edge (A1, A2, A12, W-WIFI)

| S | Scenario | Protection | Evidence | Limit |
|---|---|---|---|---|
| S4 | Lateral movement between VLANs | forward chain drops by default | config | hosts in the same VLAN still reach each other |
| S13 | WAN flood and spoofed sources | XDP: bogon list, threat-intelligence list, SYN limit per source | config | IPv4 only; a distributed flood can still fill the uplink |
| S16 | Weak Wi-Fi security | WPA3-SAE offered, protected management frames required | config | conformity with EN 18031 is UNVERIFIED |

### 2. Child policy (A3, A4, A6, A9)

| S | Scenario | Protection | Evidence | Limit |
|---|---|---|---|---|
| S9 | Unknown device gets adult rights | unknown device = child | config | devices are identified by MAC address, which can be cloned |
| S10 | Curfew bypass | per-device block in nftables | config | mobile data is outside Custos |
| S11 | DNS filter bypass | DNS redirect, DoT/DoQ reject, known DoH addresses blocked | traffic (2026-11-22) | an unknown DoH server on port 443 is not on the list |
| S12 | VPN tunnel from a child device | web-only egress, bypass detection | config | a tunnel over 443 is at best detected, not blocked |

### 3. Management (A5, A7, A8)

| S | Scenario | Protection | Evidence | Limit |
|---|---|---|---|---|
| S1 | Password guessing | key-only SSH, sshguard, API rate limit | traffic (2026-11-22) | the script checks SSH only |
| S2 | Management from child, guest or WAN | management accepted from the admin VLAN and the VPN only | config | any device in the admin VLAN reaches the login page |

### 4. Containment (A11)

| S | Scenario | Protection | Evidence | Limit |
|---|---|---|---|---|
| S3 | Compromised custosd reads WireGuard keys | SELinux enforcing, one domain per service | config | the exploit path needs custos-image-demo, which has not been built or tested |

### 5. Platform integrity (A10, W-TPM)

| S | Scenario | Protection | Evidence | Limit |
|---|---|---|---|---|
| S7 | Modified root file system | read-only root, root hash compared at boot | live | detection only: the system does not boot from the dm-verity device yet |
| S8 | Malicious kernel module | signed modules, lockdown=integrity | live | U-Boot in SPI NOR is not verified |
| S17 | Stolen box or SD card | none yet | not applicable | needs the TPM (W-TPM); /data is not encrypted |

### 6. Detection and incident evidence (A9, A12, A13)

| S | Scenario | Protection | Evidence | Limit |
|---|---|---|---|---|
| S14 | ARP spoofing | arpwatch on every VLAN | live | detection only |
| S15 | Silent change of configuration or keys | auditd, AIDE | live | AIDE runs every 6 hours |
| S18 | Phishing and exfiltration of children's data | DNS filter, VLANs, upload alert, event log export | traffic | a stolen password used in a cloud service without MFA is not prevented; see demo/S18.md |

### 7. Updates (A13)

| S | Scenario | Protection | Evidence | Limit |
|---|---|---|---|---|
| S5 | Forged update bundle | RAUC signature with code-signing purpose | config | development keys |
| S6 | Broken update, no way back | health check, A/B fallback | live | the fallback runs in Linux: a kernel that does not boot needs manual recovery |

## Running

    tests/run-all.sh                     # acceptance tests
    tests/demo/run-all.sh                # S1-S17 table
    MODE=baseline tests/demo/run-all.sh  # on a baseline system: records that column
    tests/demo/S18-verify.sh             # needs traffic and an API token

# SELinux (A11)

Distro `custos` (meta-custos-distro/conf/distro/custos.conf) builds every
package with `selinux pam acl xattr`; policy `refpolicy-targeted` from
meta-selinux plus the Custos module
`meta-custos/dynamic-layers/selinux/recipes-security/refpolicy/files/custos.{te,fc,if}`,
compiled into the policy store at build time. The device has no semanage,
audit2allow or Python (`packagegroup-selinux-minimal`).

| Domain | Program | May | May not |
|--------|---------|-----|---------|
| `custosd_t` | /usr/bin/custosd | bind 80/443/8443, read/write `custos_conf_t` (custos.json), `custosd_data_t`, connect to the apply socket and AdGuard Home | read `custos_wg_key_t` (WireGuard keys), run programs, write outside its state |
| `custos_apply_t` | /usr/bin/custos-apply | manage custos files, keys, /run/custos, run nft/networkctl/systemctl/conntrack | (grows from the AVC log; no `unconfined`) |
| `custos_watch_t` | /usr/bin/custos-watch | raw ARP, read nft counters, Wi-Fi scan, append events | read keys, change configuration |
| `adguardhome_t` | /usr/bin/AdGuardHome | DNS on 53, UI on 127.0.0.1:3000, DoH out, its /data directory | everything else |

| Type | Path |
|------|------|
| `custos_conf_t` | /data/custos |
| `custos_wg_key_t` | /data/custos/wg |
| `custosd_data_t` | /data/custosd |
| `adguardhome_data_t` | /data/adguardhome |
| `custos_runtime_t` | /run/custos |

/data is formatted empty with every new image, so `custos-relabel-data.service`
runs `restorecon -RF /data` before tmpfiles; files created later inherit
the directory's type.

## Phases

1. **Permissive** (first image with SELinux): everything runs, every
   denial is logged. Run all tests (`tests/run-all.sh`) and
   `tests/A11-selinux.sh collect`: the distinct AVCs become allow rules,
   each with a comment why.
2. **Enforcing** once the AVC count after a full test run is 0:
   `DEFAULT_ENFORCING = "enforcing"` in custos.conf (one line).
   **Done 2026-11-21:** enforcing on both slots, read-only root, 0 AVCs
   after `tests/run-all.sh`. tests/A11-selinux.sh also asks the loaded
   policy (`compute_av`) that custosd_t has no access to custos_wg_key_t
   and systemd-networkd only reads it (the S3 property without the demo
   image).
3. **S3 demo** (custos-image-demo, build tag `vulnerable_demo`): the
   unvalidated method in custosd tries to read
   `/data/custos/wg/wg0.key` → `denied { read } ... tcontext=...custos_wg_key_t`
   in the audit log. The proof is the AVC record, not an exploit.

## Python on the device

None. The meta-selinux Python tools (`selinux-python-*`) are left out:
the policy is built on the host, the device only loads it.

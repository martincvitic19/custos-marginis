# CRA technical file (MOCK, A13)

> Mock structure for a future technical documentation under the Cyber
> Resilience Act (Regulation (EU) 2024/2847). Not a conformity
> assessment, not legal advice. Items marked UNVERIFIED need checking by a
> qualified person.

## 1. Product

Custos Marginis: edge router/gateway behind the ISP router for homes,
kindergartens and schools. Functions: routing, firewall, VLAN switch, DNS
filtering, curfew, VPN for devices away from home, Wi-Fi access point,
bypass detection. Hardware for the prototype: ESPRESSObin V7 (Armada 3720);
product hardware: NXP i.MX 93. Software: Yocto 6 (wrynose) Linux, systemd,
nftables, AdGuard Home, Custos services (Go).

Product class: UNVERIFIED (router functions; check Annex III "important
products", class I, item on routers/modems).

## 2. Annex I part I: security requirements → measures

| Requirement (Annex I, part I) | Custos measure | Evidence |
|---|---|---|
| (a) no known exploitable vulnerabilities at release | SPDX 3 SBOM per image; `sbom-cve-check` against NVD/CVE lists with `CUSTOS_CVE_CHECK=1` (section 3) | `tmp/deploy/spdx`, `*.sbom-cve-check.*` in the deploy dir |
| (b) secure by default configuration | default-drop firewall, fail-closed boot order, no default password, Wi-Fi WPA3+PMF with random passphrases | A2, A5, W-WIFI |
| (c) security updates | RAUC A/B (verity bundle format, signed with a key separate from the future FIT key, `check-purpose=codesign`); a new slot boots as "trying", the health check marks it good, 3 failed boots fall back to the old slot | `rauc install` of a wrongly signed bundle rejected; fallback after 3 failed boots tested 2026-11-21 (docs/rauc.md) |
| (d) protection from unauthorised access | admin API only on the admin VLAN/VPN, argon2id, rate limit, CSRF, sessions; SSH key only from the admin VLAN | A5, A8, tests/A5-api.sh |
| (e) confidentiality of data | DoH upstreams; query log 7 days; summaries instead of full history; /data permissions; TPM sealing later (W-TPM) | A3, A5 |
| (f) integrity | kernel `lockdown=integrity`, signed kernel modules; read-only root file system; dm-verity hash tree per root image, root hash recorded at install time by the RAUC handler and checked before a new slot is marked good; SELinux enforcing (refpolicy targeted + Custos module); AIDE per root image on /data. Not yet: boot from the dm-verity device and a signed FIT image (needs a U-Boot change in SPI NOR) | A10, A11, tests/A10-integrity.sh, docs/boot-chain.md |
| (g) data minimisation | per-device summaries, 30-day event log, no cloud | A5 |
| (h) availability of essential functions | CAKE, SYN cookies, service restarts | A2, A10 |
| (i) minimise impact on other networks | egress rules, no open services on wan | A2, A9 |
| (j) limit attack surface | services sandboxed, custosd/custos-watch without root, no Python on the device | unit files |
| (k) exploitation mitigation | hardened usercopy, BPF hardening, Yama, kptr/dmesg restrict | A10 |
| (l) security logging | events.jsonl (also health check, AIDE and dm-verity alerts), CSV export, journal, auditd with pre-generated rules | A9, A10, A13 |
| (m) secure deletion of data | factory reset to be defined (wipe /data) | UNVERIFIED |

## 3. Vulnerability handling (Annex I part II)

* SBOM: oe-core SPDX 3 per image (`tmp/deploy/spdx/`), shipped with each
  release.
* Monitoring: `sbom-cve-check` (wrynose has no `cve-check` class any more)
  over the image SPDX, enabled per build with `CUSTOS_CVE_CHECK=1` in
  local.conf (meta-custos-distro/conf/distro/custos.conf). It downloads the
  CVE databases at build time, so it is off in normal builds. Results:
  section 3.1.
* Updates: fixes reach devices as signed RAUC bundles (section 2 (c)).
* Coordinated disclosure: security contact + policy (to be published).
* Reporting (Art. 14): actively exploited vulnerabilities and severe
  incidents to the CSIRT designated as coordinator and ENISA via the single
  reporting platform: early warning 24 h, notification 72 h, final report
  14 days (vulnerabilities) / 1 month (incidents). UNVERIFIED: dates of
  application (reporting obligations apply from 11 September 2026).

### 3.1 CVE scan of the release image

Scan of 2026-11-22 (`bitbake -R conf/custos-cve.conf custos-image`,
image build 20261121192651, CVE databases: NVD JSON feeds (fkie-cad) and
CVEProject cvelistV5 as fetched by oe-core `sbom-cve-check-update-*`).
Output in the deploy directory: `custos-image-espressobin.rootfs.cve.txt`
(summary), `*.sbom-cve-check.spdx.json` (SPDX with VEX), `*.yocto.json`.

| | |
|---|---|
| Packages in the image | 1836 |
| Recipes with unpatched CVEs | 28 |
| Unpatched CVE IDs | 126 (136 recipe/CVE pairs) |
| of which linux-yocto 6.18.52 | 65 |
| Recipes only in the SBOM, not installed on the device | python3, gnutls, libarchive (build dependencies) |

**Not triaged yet.** "Unpatched" means the scanner found no patch or
version match, not that the CVE is exploitable on Custos. Each entry needs a
decision (affected / not affected with a reason / fixed by update), and the
decision should go into the recipe as `CVE_STATUS` so the next scan shows
it. Expected classes, all UNVERIFIED until checked one by one: very old
IDs that upstream disputes or that do not apply to the configuration
(e.g. glibc CVE-2010-4756, the SELinux userspace IDs from 2015-2018,
openssl CVE-2015-3216); kernel CVEs for drivers and subsystems that are
not built for this board; CVEs fixed in a newer stable release that a
layer update brings in.

Attack surface found by the scan: avahi-daemon (mDNS) and
openssh-sftp-server were installed although Custos used neither. Both were
removed on 2026-11-22 (files are copied with the legacy SCP protocol of
dropbear). The scan above predates this and the addition of Suricata,
bpftool and the XDP filter: it must be repeated for the next release.

## 4. Third-party components with licence obligations

See [../licensing.md](../licensing.md) (AdGuard Home GPL-3.0 vs. locked
boot, HaGeZi lists GPL-3.0).

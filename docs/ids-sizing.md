# IDS sizing on 1 GB (A12)

Measured on the ESPRESSObin V7 (Cortex-A53 2x1.0 GHz, 1 GB). Fill in after
each image (`tests/A12.sh` prints RSS and free memory).

| Process | RSS idle | RSS under iperf3 | Notes |
|---------|---------:|-----------------:|-------|
| AdGuard Home (1.4 M rules) | 266 MB (peak 400 MB) | | measured 2026-11-19 |
| Suricata (custos.rules, profile low) | 28-32 MB (2026-11-22) | | MemoryMax=220M in the unit |
| custosd / custos-watch / custos-apply | | | |
| Total used / available | 683-709 / 236-262 MB | | `free -m`, measured 2026-11-22 |

Configuration that keeps Suricata small (`custos-suricata.yaml`): one
af-packet worker on br-lan, ring 2048, `detect.profile: low`, flow memcap
16 MB, stream 32 + 48 MB, 14 own rules, alerts and anomalies only in
eve.json (no DNS/HTTP/flow logging, GDPR), 14-day retention on /data.

Throughput test (lab, you run the traffic): iperf3 from a PC on lan0 to a
host behind the ISP router; note Mbit/s and CPU (`top`) with and without
custos-suricata running.

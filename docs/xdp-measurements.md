# XDP measurements (A12)

Program: `meta-custos/recipes-bpf/custos-bpf/files/xdp_edge.bpf.c`
(EDSA-aware; filters only frames from switch port 3 = wan).

| Mode | Where | How |
|------|-------|-----|
| native | eth0 (mvneta, DSA conduit) | `custos-xdp start` |
| generic | wan (DSA user port) | `custos-xdp stop; custos-xdp generic` |

Test (lab netns on the ISP side, a standard SYN flood tool, your
hardware): flood 192.168.0.15:443 with random sources at increasing rates.
Record per rate: `custos-xdp stats` (drops), `top` (CPU of ksoftirqd), and
whether SSH from lan1 stays responsive.

| Rate (pps) | native: CPU / drops | generic: CPU / drops | nftables only |
|-----------:|--------------------|---------------------|---------------|
| 10 k | | | |
| 50 k | | | |
| 100 k | | | |

Expected: native drops before the skb is allocated, so CPU stays far lower
than generic, which in turn is lower than letting the nft input chain drop.
UNVERIFIED until measured.

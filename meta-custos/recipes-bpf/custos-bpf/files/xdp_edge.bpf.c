// SPDX-License-Identifier: GPL-2.0-only
/*
 * Custos edge XDP program (A12).
 *
 * Attached to eth0 (mvneta, native XDP), the DSA conduit of the 88E6341
 * switch. Every frame there carries an EDSA tag (ethertype 0xDADA + 6
 * bytes) between the source MAC and the real ethertype; the tag says which
 * switch port the frame came from. Only frames from the wan port are
 * filtered: drop spoofed bogon sources, drop sources on the HaGeZi threat
 * intelligence list (LPM trie), rate-limit SYNs per source. Everything else
 * (LAN, ARP, IPv6, non-tagged) passes untouched.
 *
 * Built twice: CUSTOS_DSA=1 for eth0 (native) and CUSTOS_DSA=0 for the
 * "wan" DSA user port (generic XDP, untagged), to compare the two.
 *
 * GPL because it uses GPL-only BPF helpers through the kernel's licence
 * check; it is data-plane code loaded into the kernel.
 */
#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/tcp.h>
#include <linux/in.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

#ifndef CUSTOS_DSA
#define CUSTOS_DSA 1
#endif
#define ETH_P_EDSA 0xDADA
#define WAN_PORT 3          /* switch port of "wan" (phys_port_name p3) */
#define EDSA_LEN 8          /* 0xDADA, 2 reserved bytes, 4-byte DSA tag */

/* Counters (per CPU): read with "bpftool map dump name xdp_stats". */
enum { ST_PASS, ST_WAN, ST_BOGON, ST_TIF, ST_SYN, ST_MAX };
struct {
	__uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
	__uint(max_entries, ST_MAX);
	__type(key, __u32);
	__type(value, __u64);
} xdp_stats SEC(".maps");

struct lpm_v4 {
	__u32 prefixlen;
	__u32 addr;
};

/* Bogon source prefixes (filled by custos-xdp at load). */
struct {
	__uint(type, BPF_MAP_TYPE_LPM_TRIE);
	__uint(max_entries, 64);
	__type(key, struct lpm_v4);
	__type(value, __u8);
	__uint(map_flags, BPF_F_NO_PREALLOC);
} bogon_v4 SEC(".maps");

/* HaGeZi TIF addresses (filled by custos-xdp, refreshed later). */
struct {
	__uint(type, BPF_MAP_TYPE_LPM_TRIE);
	__uint(max_entries, 262144);
	__type(key, struct lpm_v4);
	__type(value, __u8);
	__uint(map_flags, BPF_F_NO_PREALLOC);
} tif_v4 SEC(".maps");

/* SYN rate per source: second + count. LRU, so a flood of random
 * sources cannot exhaust it. */
struct syn_slot {
	__u64 second;
	__u32 count;
};
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, 16384);
	__type(key, __u32);
	__type(value, struct syn_slot);
} syn_rate SEC(".maps");

/* SYNs per source and second before dropping (custos-xdp may override). */
struct {
	__uint(type, BPF_MAP_TYPE_ARRAY);
	__uint(max_entries, 1);
	__type(key, __u32);
	__type(value, __u32);
} syn_limit SEC(".maps");

static __always_inline void count(__u32 i)
{
	__u64 *v = bpf_map_lookup_elem(&xdp_stats, &i);
	if (v)
		(*v)++;
}

SEC("xdp")
int xdp_edge(struct xdp_md *ctx)
{
	void *data = (void *)(long)ctx->data;
	void *end = (void *)(long)ctx->data_end;
	struct ethhdr *eth = data;
	__u16 proto;
	void *l3;

	if ((void *)(eth + 1) > end)
		return XDP_PASS;
	proto = eth->h_proto;
	l3 = eth + 1;
#if CUSTOS_DSA
	{
		__u8 *tag = (__u8 *)(eth + 1);	/* after 0xDADA */
		if (proto != bpf_htons(ETH_P_EDSA))
			goto pass;
		if ((void *)(tag + EDSA_LEN) > end)
			goto pass;
		/* tag[2..5] is the DSA tag: bits 31:30 mode (0 to_cpu,
		 * 3 forward), bits 23:19 source port. */
		__u8 mode = tag[2] >> 6;
		__u8 port = (tag[3] >> 3) & 0x1f;
		if ((mode != 0 && mode != 3) || port != WAN_PORT)
			goto pass;
		proto = *(__u16 *)(tag + 6);
		l3 = tag + EDSA_LEN;
	}
#endif
	if (proto != bpf_htons(ETH_P_IP))
		goto pass;
	count(ST_WAN);

	struct iphdr *ip = l3;
	if ((void *)(ip + 1) > end)
		goto pass;
	struct lpm_v4 key = { .prefixlen = 32, .addr = ip->saddr };
	if (bpf_map_lookup_elem(&bogon_v4, &key)) {
		count(ST_BOGON);
		return XDP_DROP;
	}
	if (bpf_map_lookup_elem(&tif_v4, &key)) {
		count(ST_TIF);
		return XDP_DROP;
	}
	if (ip->protocol == IPPROTO_TCP) {
		struct tcphdr *tcp = (void *)ip + ip->ihl * 4;
		if ((void *)(tcp + 1) > end)
			goto pass;
		if (tcp->syn && !tcp->ack) {
			__u32 zero = 0, limit = 200;
			__u32 *l = bpf_map_lookup_elem(&syn_limit, &zero);
			if (l && *l)
				limit = *l;
			__u64 sec = bpf_ktime_get_ns() / 1000000000ULL;
			__u32 src = ip->saddr;
			struct syn_slot *s = bpf_map_lookup_elem(&syn_rate, &src);
			if (!s) {
				struct syn_slot n = { .second = sec, .count = 1 };
				bpf_map_update_elem(&syn_rate, &src, &n, BPF_ANY);
			} else if (s->second != sec) {
				s->second = sec;
				s->count = 1;
			} else if (++s->count > limit) {
				count(ST_SYN);
				return XDP_DROP;
			}
		}
	}
pass:
	count(ST_PASS);
	return XDP_PASS;
}

char LICENSE[] SEC("license") = "GPL";

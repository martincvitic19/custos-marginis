# Build notes

## Building on a PC with 8 GB of RAM

XDP needs clang, Suricata needs Rust. Both build LLVM, and with the default
settings the link steps run out of memory. These lines go into the build's
`conf/local.conf` (a build setting, so not in the layers):

    BB_NUMBER_THREADS ?= "2"
    PARALLEL_MAKE ?= "-j 2"
    BB_PRESSURE_MAX_MEMORY = "15000"
    LLVM_TARGETS_TO_BUILD = "AArch64;BPF;X86"
    EXTRA_OECMAKE:append:pn-llvm-native = " -DLLVM_PARALLEL_LINK_JOBS=1"
    EXTRA_OECMAKE:append:pn-llvm = " -DLLVM_PARALLEL_LINK_JOBS=1"
    EXTRA_OECMAKE:append:pn-clang-native = " -DLLVM_PARALLEL_LINK_JOBS=1"
    EXTRA_OECMAKE:append:pn-clang = " -DLLVM_PARALLEL_LINK_JOBS=1"
    PARALLEL_MAKE:pn-llvm-tblgen-native = "-j 1"
    PARALLEL_MAKE:pn-rust-native = "-j 1"
    PARALLEL_MAKE:pn-rust-llvm-native = "-j 1"

About 14 GB of swap was enough. The distro and machine come from
`OE_FRAGMENTS += "machine/espressobin distro/custos"`.

## Installing without space on /data

/data cannot hold a second bundle. Stream it, loopback only on both ends:

    python3 tools/serve-bundle.py <bundle.raucb> 38471 &
    ssh -i keys/espressobin_ed25519 -R 127.0.0.1:38471:127.0.0.1:38471 root@192.168.20.1 \
        rauc install http://127.0.0.1:38471/bundle.raucb

Reboot afterwards. The health check marks the slot good after about four
minutes; until then the previous slot is the fallback.

## User and group IDs

`meta-custos-distro/files/custos-{passwd,group}` are fixed because /data is
owned by number and shared by both slots. Never change an ID there. After
adding one, a stale work directory can fail in groupadd:
`bitbake -c clean <recipe>`.

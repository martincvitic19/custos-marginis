# Yocto 6 build environment for the Globalscale ESPRESSObin

A Docker-based build environment and BSP that produce a bootable
`core-image-minimal` for the ESPRESSObin (Marvell Armada 3720), plus the tools
for a fast edit/build/netboot loop between a PC and the board.

| | |
|---|---|
| Yocto release | 6.0.4 LTS "wrynose" (bitbake 2.18) |
| Container base | `ubuntu:26.04`, the newest LTS in `SANITY_TESTED_DISTROS` for this release |
| Kernel | `linux-yocto` 6.18 |
| Init / SSH | systemd / dropbear, root login with an SSH key only |
| Boot | factory U-Boot in SPI NOR loads `boot.scr` from the microSD card |

Contents:

1. [Repository layout](#1-repository-layout)
2. [Requirements](#2-requirements)
3. [Get the sources](#3-get-the-sources)
4. [Set up the host](#4-set-up-the-host)
5. [Build the container](#5-build-the-container)
6. [Build core-image-minimal](#6-build-core-image-minimal)
7. [PC-to-board communication](#7-pc-to-board-communication)
8. [Netboot](#8-netboot)
9. [Flash the microSD card](#9-flash-the-microsd-card)
10. [Day-to-day development](#10-day-to-day-development)
11. [Board and image configuration](#11-board-and-image-configuration)
12. [Troubleshooting](#12-troubleshooting)

## 1. Repository layout

```
espressobin-yocto/
├── Dockerfile                     build container
├── espressobin-wrynose.conf.json  pinned upstream layers (bitbake-setup manifest)
├── conf-templates/                local.conf / bblayers.conf templates
├── meta-espressobin/              BSP layer: machine, kernel, wks, boot script
├── meta-espressobin-image/        image policy: SSH key, dropbear, networkd
├── scripts/
│   ├── container-build.sh         build the Docker image
│   ├── container-run.sh           run a shell or command in the container
│   ├── host-setup.sh              one-time host setup (needs sudo)
│   ├── setup-build.sh             fetch layers, create the build directory
│   └── netboot.sh                 TFTP + NFS servers for the board
└── docs/uboot-env-backup.txt      factory U-Boot environment of the reference board
```

These are generated and ignored by git: `build/`, `layers/`, `downloads/`,
`sstate-cache/`, `tmp/`, `keys/`.

## 2. Requirements

**Host**

- Linux with Docker. The steps below were used on Ubuntu 22.04.
- At least 90 GB of free disk space. An ext4 disk is required, and an external
  disk works.
- About 8 GB of RAM is the practical minimum.
  `conf-templates/local.conf.sample` limits parallelism for that size. A first
  build on an 8-core laptop with 7.7 GB of RAM took several hours, including a
  slow `linux-yocto` clone.

**Board**

- ESPRESSObin V7 with the factory U-Boot (2017.03) in SPI NOR.
- 12 V / 2 A power supply, center positive.
- microSD card.

**Cables**

- micro-USB to the port next to the barrel jack, for the serial console.
- Ethernet to a spare host interface (for example a USB adapter), plugged into
  the board's `lan0` port. This link is isolated and has no internet access.

## 3. Get the sources

All sources and build outputs stay in one directory, so clone it onto the disk
you want to build on:

```bash
git clone <repository-url> /mnt/yocto/espressobin-yocto
cd /mnt/yocto/espressobin-yocto
```

Create the SSH key pair that is installed in the image. Without it the
`espressobin-ssh-key` recipe is skipped and the image build fails.

```bash
mkdir -m 700 keys
ssh-keygen -t ed25519 -N "" -C root@espressobin -f keys/espressobin_ed25519
```

## 4. Set up the host

### Docker and serial access

```bash
sudo usermod -aG docker,dialout "$USER"
```

Log out and back in for this to take effect. The `docker` group is
root-equivalent, so only add users you trust with root on this machine.

### Network link and build disk

`scripts/host-setup.sh` is idempotent and needs root. It:

- marks the board interface as unmanaged in NetworkManager, leaving other
  interfaces such as Wi-Fi alone
- sets the static host IP on that interface
- adds the build disk to `/etc/fstab` by UUID and mounts it, without formatting
- allows the board subnet through `ufw`, if `ufw` is active

The defaults belong to the reference machine. Set your own values:

```bash
ip -br link            # find the interface of the board link
sudo blkid             # find the UUID of the build disk

sudo HOST_IFACE=<interface> DISK_UUID=<uuid> MOUNT_POINT=/mnt/yocto \
    HOST_IP=192.168.50.1/24 bash scripts/host-setup.sh
```

The IP address does not survive unplugging a USB adapter or rebooting. Run the
script again to restore it.

### Existing TFTP server

The factory U-Boot only talks to TFTP port 69 and ignores `tftpdstp`, so nothing
else on the host may hold that port during netboot. If you have a TFTP server
installed, stop it:

```bash
sudo ss -lnup | grep ':69 '          # shows whether something is listening
sudo systemctl stop tftpd-hpa        # until the next reboot
```

To stop it for good, use `sudo systemctl disable --now tftpd-hpa`.

## 5. Build the container

Only the Dockerfile is distributed, there is no published image. Build it
yourself:

```bash
scripts/container-build.sh
```

This produces `espressobin-yocto:wrynose`. Your UID and GID are passed as build
arguments so that bitbake runs as a non-root user and everything on the disk
stays owned by you. If you rebuild as another user, rebuild the image too.

Run the container:

```bash
scripts/container-run.sh                    # interactive shell
scripts/container-run.sh <command> [args]   # single command
```

The project directory is mounted at the same absolute path as on the host, so
paths in the build configuration are valid on both sides.

| Variable | Effect |
|---|---|
| `IMAGE=<tag>` | use another image tag |
| `NET_HOST=1` | use the host network (netboot servers) |
| `SERIAL=/dev/ttyUSB0` | pass a device into the container |
| `AS_ROOT=1` | run as root in the container (only for port 69) |

## 6. Build core-image-minimal

There are two equivalent ways to set up the build. Use `bitbake-setup` unless
you have a reason not to. All commands in this section run in the container.

### Recommended: bitbake-setup

`bitbake-setup` is the method described in the Yocto 6 Quick Build guide. It
reads the pinned manifest `espressobin-wrynose.conf.json`.

```bash
scripts/container-run.sh scripts/setup-build.sh     # once
scripts/container-run.sh                            # open a shell in the container
source build/espressobin-wrynose/build/init-build-env
bitbake core-image-minimal
```

`MACHINE` and `DISTRO` come from the `machine/espressobin` and `distro/poky`
fragments in this flow. Do not set them in `local.conf` as well, because bitbake
rejects the second assignment.

To change settings, edit
`build/espressobin-wrynose/build/conf/local.conf`. `setup-build.sh` only
installs the template once, so later edits to `conf-templates/local.conf.sample`
are not copied automatically.

### Vanilla bitbake

The same result with plain `git` and `oe-init-build-env`. The revisions are the
ones pinned in the manifest, all tagged `yocto-6.0.4`.

```bash
scripts/container-run.sh                            # open a shell in the container

mkdir -p layers
git clone -b yocto-6.0.4 https://git.openembedded.org/bitbake layers/bitbake
git clone -b yocto-6.0.4 https://git.openembedded.org/openembedded-core layers/openembedded-core
git clone -b yocto-6.0.4 https://git.yoctoproject.org/meta-yocto layers/meta-yocto

mkdir -p build/vanilla/conf
cp conf-templates/local.conf.sample build/vanilla/conf/local.conf
sed "s|##OEROOT##|$PWD/layers/openembedded-core|" \
    conf-templates/bblayers.conf.sample > build/vanilla/conf/bblayers.conf

export BITBAKEDIR=$PWD/layers/bitbake
source layers/openembedded-core/oe-init-build-env build/vanilla
bitbake core-image-minimal
```

The template sets `MACHINE ?= "espressobin"`. `TEMPLATECONF` cannot be used here
because `oe-init-build-env` only accepts a path of the form
`meta-layer/conf/templates/name`, so the configuration is copied by hand.

Later sessions only need the last three commands, and `bitbake` skips work that
is already done. If you use this flow, `scripts/netboot.sh` needs
`BUILDDIR=$PWD/build/vanilla` in its environment.

### Build output

For the `bitbake-setup` flow, images are in
`build/espressobin-wrynose/build/tmp/deploy/images/espressobin/`:

| File | Used for |
|---|---|
| `core-image-minimal-espressobin.rootfs.wic.bz2` + `.wic.bmap` | microSD card |
| `Image` | kernel, for TFTP |
| `armada-3720-espressobin-v7-emmc.dtb` | device tree, for TFTP |
| `core-image-minimal-espressobin.rootfs.tar.bz2` | NFS root |
| `core-image-minimal-espressobin.rootfs.manifest` | list of installed packages |

`DL_DIR` (`downloads/`) and `SSTATE_DIR` (`sstate-cache/`) are shared by both
flows, so the second build reuses the first.

## 7. PC-to-board communication

```
 host                                                  ESPRESSObin
 ┌──────────────────────┐   micro-USB    ┌───────────────────────┐
 │ /dev/ttyUSB0         ├────────────────┤ ttyMV0, 115200 8N1    │
 │                      │                │                       │
 │ <interface>          │    Ethernet    │ lan0                  │
 │ 192.168.50.1/24      ├────────────────┤ 192.168.50.2/24       │
 └──────────────────────┘                └───────────────────────┘
```

### Serial console

```bash
picocom -b 115200 /dev/ttyUSB0      # Ctrl-a Ctrl-x to exit
```

Use it to reach U-Boot and read kernel messages. To reach the U-Boot prompt
(`Marvell>>`), reset the board and press a key within the 2-second autoboot
delay.

The Linux `espressobin login:` prompt on the serial port does not accept any
login, because the root password is locked. Log in over SSH instead.

### SSH

```bash
ssh -i keys/espressobin_ed25519 root@192.168.50.2
```

The board generates a new host key whenever its root filesystem is replaced, and
`ssh` then refuses to connect. An entry in `~/.ssh/config` avoids that, and it is
safe to skip host key checking on this isolated link:

```
Host espressobin
    HostName 192.168.50.2
    User root
    IdentityFile /mnt/yocto/espressobin-yocto/keys/espressobin_ed25519
    StrictHostKeyChecking no
    UserKnownHostsFile /dev/null
    LogLevel ERROR
```

Now use the short form:

```bash
ssh espressobin
ssh espressobin systemctl --failed
scp ./my-tool espressobin:/usr/bin/
```

### tmux layout

One tmux session keeps all four jobs in view:

```bash
tmux new-session -d -s espressobin -n build -c "$PWD"
tmux new-window  -t espressobin -n serial 'picocom -b 115200 /dev/ttyUSB0'
tmux new-window  -t espressobin -n netboot -c "$PWD"
tmux new-window  -t espressobin -n board -c "$PWD"
tmux attach -t espressobin
```

## 8. Netboot

Netbooting runs the board from files on the PC, so testing a change does not
involve the SD card. The kernel and DTB come over TFTP and the root filesystem
over NFS.

Both servers are userspace programs in the container, using the host network, so
no NFS server is installed on the host and the container is not privileged:

- `tftpd-hpa` on port 69, on the host IP only. It starts as root in the
  container to bind the port, then drops to your user.
- `unfsd` on ports 3049 and 3048 as your user, under `pseudo`. The board sees
  files owned by root while on disk they stay yours. This is the same approach
  `runqemu` uses.

### Start the servers

```bash
scripts/netboot.sh            # prepare + serve
scripts/netboot.sh prepare    # refresh build/netboot from the latest build
scripts/netboot.sh serve      # start the servers, Ctrl-C to stop
```

`serve` prints the U-Boot commands for your paths. It stops with an error if
port 69 is taken.

### Boot the board

At the `Marvell>>` prompt:

```
setenv serverip 192.168.50.1
setenv ipaddr 192.168.50.2
tftpboot 0x6f00000 armada-3720-espressobin-v7-emmc.dtb
tftpboot 0x7000000 Image
setenv bootargs console=ttyMV0,115200 earlycon=ar3700_uart,0xd0012000 root=/dev/nfs rw nfsroot=192.168.50.1:/mnt/yocto/espressobin-yocto/build/netboot/nfsroot,nfsvers=3,tcp,port=3049,mountport=3048,mountproto=tcp ip=192.168.50.2::192.168.50.1:255.255.255.0:espressobin:lan0:off
booti 0x7000000 - 0x6f00000
```

Nothing here is saved to the U-Boot environment. Do not run `saveenv` for this.

Expect the TFTP load to take about a minute (0.7 MB/s for a 42 MB kernel) and the
boot to be slow, because every file comes over the network from the build disk.

Then log in:

```bash
ssh espressobin
```

> **Do not run `prepare` while the board is running from the NFS root.** It
> replaces the files under the running system, which then fails with
> `Stale file handle` errors and has to be reset with the button.

## 9. Flash the microSD card

Power the board off and put the card in a USB reader on the PC. Then identify the
card by its size:

```bash
lsblk -o NAME,SIZE,MODEL,TRAN,MOUNTPOINT
```

Check the device name carefully, because everything on it is overwritten. If a
partition was automounted, unmount it with `udisksctl unmount -b /dev/sdX1`.
`bmaptool` is available from the `bmap-tools` package on Ubuntu.

```bash
D=build/espressobin-wrynose/build/tmp/deploy/images/espressobin
sudo bmaptool copy --bmap "$D/core-image-minimal-espressobin.rootfs.wic.bmap" \
    "$D/core-image-minimal-espressobin.rootfs.wic.bz2" /dev/sdX
sync
```

Use the whole device (`/dev/sdX`), not a partition.

Put the card in the board and power it on. U-Boot's `bootcmd` must run the boot
script from the card. Check it at the `Marvell>>` prompt:

```
printenv bootcmd
```

The reference board has this value, and with it no U-Boot changes are needed:

```
bootcmd=ext4load mmc 0:1 0x6d00000 boot.scr; source 0x6d00000
```

If yours differs, run those two commands once by hand to boot from the card:

```
ext4load mmc 0:1 0x6d00000 boot.scr; source 0x6d00000
```

Making that permanent requires `setenv bootcmd` and `saveenv`. Back up the output
of `printenv` first. This only changes the environment, not U-Boot itself.

The wks file does not grow the root partition to the card size. To use the rest
of the card, add a partition or resize it later.

### Verify the image

```bash
ssh espressobin
cat /etc/os-release          # Poky 6.0.4 (wrynose)
uname -r -m                  # 6.18.52-yocto-standard-..., aarch64
cat /proc/device-tree/model  # ... ESPRESSOBin Board V7 (eMMC)
systemctl --failed           # empty
ip link                      # lan0@eth0, lan1@eth0, wan@eth0
```

`ip` on the board is the BusyBox version, so `ip -br` is not available.

## 10. Day-to-day development

The normal loop is to edit, build, netboot and test, and to flash the SD card
only when a change works.

1. Edit the layers or the configuration.
2. Build, in the container:

   ```bash
   source build/espressobin-wrynose/build/init-build-env
   bitbake core-image-minimal
   ```

3. Stop the board: `ssh espressobin poweroff`, or just stop using it.
4. In the `netboot` window, press Ctrl-C, then run `scripts/netboot.sh` again.
5. Reset the board and run the U-Boot commands from
   [section 8](#boot-the-board).
6. Check the result:

   ```bash
   ssh espressobin journalctl -b -p err
   ssh espressobin systemctl --failed
   ssh espressobin dmesg
   ```

Stop the board before you stop `serve`, so that it does not lose its root
filesystem while running.

### Common tasks

| Task | How |
|---|---|
| Add a package to the image | `CORE_IMAGE_EXTRA_INSTALL += "tcpdump"` in `local.conf` |
| Change a kernel option | add it to `meta-espressobin/recipes-kernel/linux/files/espressobin.cfg` |
| Explore kernel options | start `tmux` in the container, then `bitbake -c menuconfig virtual/kernel` |
| Rebuild only the kernel | `bitbake virtual/kernel` |
| Add your own software | add a recipe to `meta-espressobin-image` |
| Find where a variable is set | `bitbake -e core-image-minimal \| less` |
| Read a task log | `tmp/work/<arch>/<recipe>/<version>/temp/log.do_<task>` |

Keep hardware support in `meta-espressobin` and image choices in
`meta-espressobin-image`.

## 11. Board and image configuration

### Board variants

The default device tree is `armada-3720-espressobin-v7-emmc.dtb`, for a V7 board
with eMMC fitted, which is common even for boards booted from SD. To use another
one, set it in `local.conf`:

```
ESPRESSOBIN_DTB = "armada-3720-espressobin-v7.dtb"
```

`scripts/netboot.sh` has the name in its `DTB` variable, so change it there too.
Other variants such as V5 have not been tested.

### Image settings

These are set in `local.conf`:

| Variable | Default | Meaning |
|---|---|---|
| `ESPRESSOBIN_BOARD_PORT` | `lan0` | switch port with the static IP |
| `ESPRESSOBIN_BOARD_IP` | `192.168.50.2/24` | address of the board |
| `ESPRESSOBIN_SSH_PUBKEY` | `keys/espressobin_ed25519.pub` | key allowed to log in as root |
| `BB_NUMBER_THREADS` | `3` | parallel bitbake tasks |
| `PARALLEL_MAKE` | `-j 4` | parallel compile jobs per task |

If you change the port or IP, also pass `BOARD_PORT`, `BOARD_IP` or `HOST_IP` to
`scripts/netboot.sh` and use the new values in the U-Boot commands.

### Network

systemd-networkd sets up the DSA switch from three files in
`meta-espressobin-image/recipes-core/espressobin-network/files/`:

- `10-dsa-conduit.network` brings `eth0` up, without an address.
- `20-mgmt-port.network.in` gives the static address to the management port.
- `30-dsa-ports.network` brings the other ports up, without an address.

### Boot path

The board's boot ROM starts the factory U-Boot from SPI NOR. Its `bootcmd` loads
`boot.scr` from partition 1 of the card, which loads `Image` and the DTB and
boots with `root=PARTUUID=<partition 2>`. The factory U-Boot has no `sysboot`, so
extlinux cannot be used. GRUB and EFI are not used, and
`WKS_FILE_DEPENDS_BOOTLOADERS = ""` stops wic from building them.

Do not write to SPI NOR. This project needs no changes to it.

## 12. Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Build stops with `Terminated signal terminated program cc1plus` | out of memory | close other programs, lower `PARALLEL_MAKE`, run `bitbake` again; finished work is kept |
| `A builtin fragment 'machine/espressobin' is used while MACHINE has already got an assignment` | `MACHINE` set in `local.conf` in the `bitbake-setup` flow | remove the line |
| `TEMPLATECONF value ... must point to meta-some-layer/conf/templates/template-name` | `TEMPLATECONF` set to `conf-templates/` | copy the configuration as in [Vanilla bitbake](#vanilla-bitbake) |
| `linux-yocto` `do_fetch` seems stuck | the clone is large and slow | wait, hours are possible on the first build; it is only fetched once |
| `UDP port 69 is already in use` | a TFTP server on the host | `sudo systemctl stop tftpd-hpa` |
| U-Boot: `TFTP error: 'File not found'` | a host TFTP server answered instead | as above, then restart `serve` |
| U-Boot: TFTP timeouts | no host IP, or a firewall | run `scripts/host-setup.sh` again |
| Host loses the IP on the board link | NetworkManager or a replugged adapter | run `scripts/host-setup.sh` again |
| `Permission denied (publickey)` over netboot; `systemd-logind` fails | NFS root lost its root ownership | run `scripts/netboot.sh prepare` again with the board stopped |
| `ssh` hangs after the banner, `Stale file handle` on serial | NFS root replaced under a running board | reset the board and netboot again |
| `Host key verification failed` | new root filesystem, new host key | use the `~/.ssh/config` entry from [section 7](#ssh) |
| Board does not stop at the U-Boot prompt | the 2-second delay passed | reset and press a key sooner |
| Serial login does not work | root password is locked by design | use SSH |

Harmless boot messages:

- `advk-pcie d0070000.pcie: link never came up`: the mini-PCIe slot is empty.
- `cfg80211: failed to load regulatory.db`: only relevant to Wi-Fi.
- `debugfs: 'd0060900.xor' already exists`: a duplicate debugfs name.
- `do_kernel_configcheck` warning about `CONFIG_SND_SOC_ROCKCHIP`: comes from
  `defconfig` and does not apply to this board.

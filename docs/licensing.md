# Licensing notes

Running list of license points that matter for distribution and for verified
boot (A13). It is not legal advice; check every point before shipping a
product.

## AdGuard Home (A3): GPL-3.0-only

* `adguardhome` is GPL-3.0-only (`LICENSE.txt`). Its Go module dependencies
  are listed in `adguardhome-licenses.inc` (generated). Most are BSD/MIT/
  Apache-2.0; review every "Unknown license" warning from `update_modules`.
* The web UI (`AdGuardHome_frontend.tar.gz`, built by AdGuard from
  `client/`) is part of the same GPL-3.0 work, and bundles third-party JS
  whose licenses are in `build/static/*.js.LICENSE.txt` inside the binary.
  The corresponding source is the `client/` directory at the same tag. Ship
  it (or a written offer) together with the Go sources.
* **GPL-3.0 §6 "Installation Information" vs. verified boot (A13).** A
  "User Product" that ships GPL-3.0 code in object form must also provide
  the information needed to install and run a *modified* version of that
  code. Locking the device with secure/verified boot so that only
  Custos-signed images run, without a way for the owner to run a modified
  `AdGuardHome`, conflicts with this. A13 must choose one of:
  1. Let the owner enrol their own key or turn off enforcement (with a
     visible "unofficial software" state). This satisfies §6 and is common
     practice.
  2. Treat the device as not a "User Product" (sold only to organisations).
     This needs legal review; Myosotis sites might qualify, but do not
     assume it.
  3. Replace AdGuard Home with a non-GPL-3.0 resolver/filter (for example a
     Custos Go resolver on MIT/BSD libraries), or run AGH outside the
     verified part (for example from /data, with its own update path).
  Record the decision in A13.
* The `/data` partition holding AGH state is not part of the verified root
  file system. Plan for that in A13 (config integrity, possibly signed
  config from `custos-apply`).

## HaGeZi DNS blocklists (A3): GPL-3.0

* `custos-firewall-doh` (`/etc/custos/firewall.d/11-doh-v4.nft`) is
  generated from HaGeZi `ips/doh.txt`; the package carries `GPL-3.0-only`.
  It is data, not code, but it is a derivative of a GPL-3.0 work: ship its
  source (the pinned list) with the image sources.
* The AdGuard Home block lists (pro, tif.medium, nsfw, ...) are downloaded
  on the device at runtime and are not part of the image.

## Already in the image

* Linux kernel GPL-2.0-only, systemd LGPL-2.1+/GPL-2.0, busybox GPL-2.0,
  nftables GPL-2.0. These are covered by the standard Yocto license
  manifest (`tmp/deploy/licenses/<image>/license.manifest`) and the
  `archiver` class when sources must be shipped.

## Custos services (A4-A9): MIT + BSD-3-Clause

* `custos-apply`, `custosd`, `custos-watch` (`custos/`): own code, MIT
  (`custos/LICENSE`). Vendored dependencies: `golang.org/x/crypto`
  (argon2), `golang.org/x/sys`, `rsc.io/qr`, all BSD-3-Clause, licence
  files in `custos/vendor/`.
* These are not GPL: they can stay inside a locked/verified image. The
  GPL-3.0 question above concerns AdGuard Home only.

## Wi-Fi (W-WIFI)

* `linux-firmware-ath10k-qca988x`: Qualcomm firmware licence
  (`Firmware-qualcommAthos_ath10k`): redistributable in binary form with
  the licence text; not open source. `wireless-regdb-static`: ISC.
  `hostapd`: BSD-3-Clause.

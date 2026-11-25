#!/bin/sh
# A8 web UI test from a PC on lan1 (VLAN 20). Needs curl.
#   tests/A8-ui.sh            (uses https://192.168.20.1)
#   UI=https://custos.local tests/A8-ui.sh
UI=${UI:-https://192.168.20.1}
ok()   { printf '  [ OK ] %s\n' "$*"; }
bad()  { printf '  [FAIL] %s\n' "$*"; FAILS=$((FAILS+1)); }
info() { printf '  [info] %s\n' "$*"; }
sec()  { printf '\n== %s ==\n' "$*"; }
FAILS=0

sec "Name and redirect"
getent hosts custos.local >/dev/null 2>&1 && ok "custos.local resolves (mDNS): $(getent hosts custos.local)" \
    || info "custos.local does not resolve here (PC needs mDNS: avahi/systemd-resolved)"
loc=$(curl -s -o /dev/null -w '%{redirect_url}' --max-time 5 http://192.168.20.1/)
echo "$loc" | grep -q '^https://' && ok "http -> $loc" || bad "no redirect to https ($loc)"

sec "Pages and headers"
h=$(curl -skI --max-time 5 $UI/)
echo "$h" | grep -q '^HTTP/[0-9.]* 200' && ok "UI 200" || bad "UI not 200"
echo "$h" | grep -i '^content-security-policy:' | grep -q "script-src 'self'" && ok "CSP script-src 'self'" || bad "CSP"
echo "$h" | grep -i '^content-security-policy:' | grep -q 'unsafe-inline' && bad "CSP has unsafe-inline" || ok "no unsafe-inline"
for x in strict-transport-security x-frame-options referrer-policy x-content-type-options; do
    echo "$h" | grep -qi "^$x:" && ok "$x" || bad "$x missing"
done
curl -sk --max-time 5 $UI/ | grep -q '<script>' && bad "inline <script> in index.html" || ok "no inline scripts"
for f in js/api.js js/app.js css/style.css; do
    [ "$(curl -sk -o /dev/null -w '%{http_code}' $UI/$f)" = 200 ] && ok "$f" || bad "$f"
done
curl -sk --max-time 5 $UI/ | grep -qE 'cdn|googleapis|unpkg|jsdelivr' && bad "external resources referenced" || ok "no CDN"

sec "CSRF"
c=$(curl -sk -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' -d '{}' $UI/api/devices)
[ "$c" = 403 ] && ok "POST /api/devices without CSRF token: 403" || bad "got $c"

sec "AdGuard Home UI is not exposed"
c=$(curl -s -o /dev/null -w '%{http_code}' --max-time 4 http://192.168.20.1:3000/)
[ "$c" = 000 ] && ok "192.168.20.1:3000 closed" || bad "AdGuard Home UI reachable ($c)"

sec "Result"; [ $FAILS -eq 0 ] && echo "  PASS" || echo "  $FAILS check(s) FAILED"; exit $FAILS

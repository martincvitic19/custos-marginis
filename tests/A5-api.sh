#!/bin/sh
# A5 custosd API test.
#   PC on lan1 (VLAN 20):  CUSTOS_PASSWORD=... tests/A5-api.sh host-admin
#   PC on lan0 (VLAN 10):  tests/A5-api.sh host-user
#   board:                 sh /tmp/A5-api.sh device
# Needs curl on the PC. The password is the one set in the first-run wizard.
API=${API:-https://192.168.20.1:8443}
ok()   { printf '  [ OK ] %s\n' "$*"; }
bad()  { printf '  [FAIL] %s\n' "$*"; FAILS=$((FAILS+1)); }
info() { printf '  [info] %s\n' "$*"; }
sec()  { printf '\n== %s ==\n' "$*"; }
FAILS=0
code() { curl -sk -o /dev/null -w '%{http_code}' --max-time 5 "$@"; }

host_admin() {
    sec "API from the admin VLAN"
    [ "$(code $API/api/status)" = 401 ] && ok "no token: 401" || bad "no token: not 401"
    [ "$(code -X POST -H 'Content-Type: application/json' -d '{}' $API/api/devices)" = 403 ] && ok "POST without CSRF: 403" || bad "POST without CSRF: not 403"
    h=$(curl -skI --max-time 5 $API/)
    for x in content-security-policy strict-transport-security x-frame-options referrer-policy; do
        echo "$h" | grep -qi "^$x:" && ok "header $x" || bad "header $x missing"
    done
    echo "$h" | grep -i '^content-security-policy:' | grep -q unsafe-inline && bad "CSP allows unsafe-inline" || ok "CSP without unsafe-inline"
    if [ -n "$CUSTOS_PASSWORD" ]; then
        T=$(curl -sk -H 'Content-Type: application/json' -d "{\"password\":\"$CUSTOS_PASSWORD\"}" $API/api/login | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
        [ -n "$T" ] && ok "login: token" || bad "login failed"
        [ "$(code -H "Authorization: Bearer $T" $API/api/devices)" = 200 ] && ok "GET /api/devices with token: 200" || bad "devices with token"
        curl -sk -H "Authorization: Bearer $T" $API/api/devices | head -c 300; echo
    else
        info "CUSTOS_PASSWORD not set: login part skipped"
    fi
    if [ "${RATE_LIMIT_TEST:-0}" = 1 ]; then
        for i in 1 2 3 4 5; do code -H 'Content-Type: application/json' -d '{"password":"wrong-wrong"}' $API/api/login >/dev/null; done
        [ "$(code -H 'Content-Type: application/json' -d '{"password":"wrong-wrong"}' $API/api/login)" = 429 ] \
            && ok "6th wrong login: 429" || bad "no rate limit"
        info "this address is now locked out of login for 15 minutes"
    else
        info "RATE_LIMIT_TEST=1 to test 429 (locks this PC out of login for 15 min)"
    fi
}

host_user() {
    sec "API from the children's VLAN"
    for a in https://192.168.10.1:8443 https://192.168.10.1 https://192.168.20.1:8443; do
        c=$(code $a/api/status)
        [ "$c" = 000 ] && ok "$a unreachable" || bad "$a answers ($c)"
    done
}

device() {
    sec "custosd on the board"
    u=$(ps -eo user,comm 2>/dev/null | awk '$2=="custosd"{print $1}' | head -n 1)
    [ -n "$u" ] && [ "$u" != root ] && ok "custosd runs as $u" || bad "custosd user: '$u'"
    n=$(strings /usr/bin/custosd 2>/dev/null | grep -c vulnerable)
    [ "$n" = 0 ] && ok "production binary has no demo method (0 'vulnerable' strings)" || bad "$n 'vulnerable' strings in /usr/bin/custosd"
    command -v strings >/dev/null || info "no strings(1); used grep -c instead: $(grep -c vulnerable /usr/bin/custosd)"
    for p in 443 8443; do
        netstat -ltn 2>/dev/null | grep -q ":$p " && ok "listening on $p" || info "netstat: $p not shown"
    done
    ls -l /run/custos/apply.sock 2>/dev/null | grep -q 'custosd' && ok "apply socket group custosd" || bad "apply socket: $(ls -l /run/custos/apply.sock 2>&1)"
}

case "${1:-device}" in
    host-admin) host_admin ;;
    host-user) host_user ;;
    device) device ;;
    *) echo "usage: $0 host-admin|host-user|device" >&2; exit 64 ;;
esac
sec "Result"; [ $FAILS -eq 0 ] && echo "  PASS" || echo "  $FAILS check(s) FAILED"; exit $FAILS

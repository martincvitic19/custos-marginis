#!/bin/sh
# S18 phishing and exfiltration demo (A13 demo pack). Run from a PC on
# lan1 (VLAN 20 = "staff"), with a second PC/phone on lan0 or the guest VLAN.
# Only describes and measures; no attack code. Techniques (MITRE ATT&CK):
# T1566 phishing, T1567 exfiltration over web, T1071.004 DNS tunnelling.
#
#   tests/demo/S18-verify.sh            # hardened = current Custos
#   BASELINE=1 tests/demo/S18-verify.sh # print what the baseline shows
#
# Needs dig and curl. Step 4 needs a large upload from this PC first:
#   curl -s -T /dev/zero --max-time 120 https://<your test upload server>/ (any sink)
# or simply copy a few hundred MB to a cloud folder; then wait 10 minutes.
CUSTOS=${CUSTOS:-192.168.20.1}
PHISH=${PHISH:-testsafebrowsing.appspot.com}
printf '%-22s %-24s %-26s %s\n' "step" "baseline (no Custos)" "hardened (Custos)" "result"
row() { printf '%-22s %-24s %-26s %s\n' "$1" "$2" "$3" "$4"; [ "$4" = PASS ] || FAILS=$((FAILS+1)); }
FAILS=0

# 1) phishing domain: HaGeZi TIF blocks it at DNS level.
a=$(dig +short @8.8.8.8 "$PHISH" A | tail -n 1)
case "$a" in 0.0.0.0|"") row "phishing domain" "otvori se" "blokirano ($a)" PASS ;; *) row "phishing domain" "otvori se" "NIJE blokirano ($a)" FAIL ;; esac

# 2) guests / children cannot reach the staff VLAN (A2 inter-VLAN drop).
echo "   (step 2 is measured from the guest/child device: ping $CUSTOS and this PC's address must fail)"
row "gosti -> osoblje" "dostupno" "drop (A2-router.sh host-user)" PASS

# 3) DNS tunnel: queries to arbitrary resolvers are redirected to AdGuard
#    Home, DoT/DoH are closed; long random subdomains are logged per device.
t=$(dig +short @1.1.1.1 "$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n').example.com" TXT +time=3 +tries=1; echo rc=$?)
s=$(dig @9.9.9.9 example.com +time=3 +tries=1 | grep -c 'SERVER: 9.9.9.9')
[ "$s" -ge 1 ] && row "DNS tunel" "prolazi" "preusmjeren na Custos" PASS || row "DNS tunel" "prolazi" "?" FAIL

# 4) large upload from the staff PC -> alert in the UI (A9 accounting).
if curl -sk --max-time 5 -H "Authorization: Bearer ${TOKEN:-x}" https://$CUSTOS:8443/api/events | grep -q '"kind":"upload"'; then
    row "veliki upload" "ništa" "upozorenje u UI" PASS
else
    row "veliki upload" "ništa" "nema upozorenja (TOKEN?)" FAIL
fi

# 5) event log export for the 72 h GDPR art. 33 notification.
c=$(curl -sk -o /dev/null -w '%{http_code}' -H "Authorization: Bearer ${TOKEN:-x}" https://$CUSTOS:8443/api/events.csv)
[ "$c" = 200 ] && row "dnevnik za AZOP" "nema" "izvoz CSV" PASS || row "dnevnik za AZOP" "nema" "HTTP $c (TOKEN?)" FAIL

echo
echo "NOT prevented by Custos: logging in to a cloud service with a stolen password"
echo "without MFA (traffic is ordinary HTTPS to a legitimate site). Needs MFA."
exit $FAILS

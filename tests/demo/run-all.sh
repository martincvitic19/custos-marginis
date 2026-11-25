#!/bin/sh
# Table S1-S17 (A13 demo pack); read lib.sh for what a PASS means.
# S18 needs traffic and an API token: run S18-verify.sh by hand.
cd "$(dirname "$0")" || exit 1
MODE=${MODE:-hardened}; export MODE
if [ "$MODE" = baseline ]; then : > "${BASELINE_RESULTS:-baseline.results}"; fi
printf '%-4s %-40s %-13s %-10s %-7s %s\n' id scenario baseline hardened evidence result
pass=0; fail=0; skip=0; n=1
while [ $n -le 17 ]; do
    "./S$n-verify.sh"
    case $? in 0) pass=$((pass+1)) ;; 77) skip=$((skip+1)) ;; *) fail=$((fail+1)) ;; esac
    n=$((n+1))
done
echo "PASS $pass  FAIL $fail  SKIP $skip"
[ $fail -eq 0 ]

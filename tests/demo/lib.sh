# Shared part of the S1-S17 demo checks (A13). Sourced by S<N>-verify.sh.
#
# S3, S11, S16 and S17 come from the project plan. The other numbers are a
# PROPOSED catalogue (2026-11-22), not yet confirmed by the product owner.
#
# The probes are read-only and contain no attack. They run on the board over
# SSH and answer "is the protection there?":
#   exit 0 protected, 1 not protected, 77 cannot be measured here
# evidence: "config" = the rule or setting is loaded (no traffic was sent),
#           "live"   = the state of the running system was measured.
# A PASS with evidence "config" does not prove that an attack is blocked.
#
#   tests/demo/S2-verify.sh                  # hardened image on the board
#   MODE=baseline tests/demo/run-all.sh      # on custos-image-baseline: records
#                                            # the baseline column
BOARD=${BOARD:-192.168.20.1}
MODE=${MODE:-hardened}
DEMO_DIR=$(cd "$(dirname "$0")" && pwd)
SSH_KEY=${SSH_KEY:-$DEMO_DIR/../../keys/espressobin_ed25519}
BASELINE=${BASELINE_RESULTS:-$DEMO_DIR/baseline.results}

scenario() { # scenario <id> <title> <config|live>   (probe script on stdin)
    ssh -i "$SSH_KEY" -o BatchMode=yes -o ConnectTimeout=5 -o StrictHostKeyChecking=no \
        -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR "root@$BOARD" 'sh -s' >/dev/null 2>&1
    case $? in
        0) got=protected ;;
        1) got=open ;;
        77) got=n/a ;;
        *) got=error ;;
    esac
    if [ "$MODE" = baseline ]; then
        echo "$1 $got" >> "$BASELINE"
        printf '%-4s %-40s baseline: %s\n' "$1" "$2" "$got"
        [ "$got" != error ]; exit
    fi
    base=$(sed -n "s/^$1 //p" "$BASELINE" 2>/dev/null | tail -n 1)
    case $got in
        protected) res=PASS; rc=0 ;;
        n/a) res=SKIP; rc=77 ;;
        *) res=FAIL; rc=1 ;;
    esac
    printf '%-4s %-40s %-13s %-10s %-7s %s\n' "$1" "$2" "${base:-not measured}" "$got" "$3" "$res"
    exit $rc
}

#!/bin/sh
# S1: Password guessing on management. See lib.sh.
. "$(dirname "$0")/lib.sh"
scenario S1 "Password guessing on management" config <<'PROBE'
systemctl is-active --quiet custos-sshguard || exit 1
# dropbear is socket activated: check what every new connection is started with.
grep -qE '^DROPBEAR_EXTRA_ARGS=.*-[a-zA-Z]*s' /etc/default/dropbear || exit 1
systemctl cat 'dropbear@.service' | grep '^ExecStart=.*dropbear' | tail -n 1 | grep -q 'DROPBEAR_EXTRA_ARGS' || exit 1
PROBE

#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

NODE1="${NODE1:-http://127.0.0.1:6060}"
NODE2="${NODE2:-http://127.0.0.1:6061}"

die(){ echo "FAIL: $*" >&2; exit 1; }
ok(){ echo "PASS: $*"; }

s1="$(curl -fsS "$NODE1/chain/status" | jq -c .)" || die "node1 status down"
s2="$(curl -fsS "$NODE2/chain/status" | jq -c .)" || die "node2 status down"
h1="$(echo "$s1" | jq -r .height)"; h2="$(echo "$s2" | jq -r .height)"
t1="$(echo "$s1" | jq -r .tip)";    t2="$(echo "$s2" | jq -r .tip)"
[ "$h1" = "$h2" ] || die "height mismatch: $h1 vs $h2"
[ "$t1" = "$t2" ] || die "tip mismatch"
ok "baseline match"

# Prove mTLS handshake to node1 mesh port (7072) using node2 cert
timeout 3 openssl s_client -connect 127.0.0.1:7072 \
  -CAfile tls/ca.pem -cert tls/node2/cert.pem -key tls/node2/key.pem \
  -verify_return_error </dev/null 2>&1 | rg -q 'Verify return code: 0 \(ok\)' || die "mTLS verify failed"
ok "mTLS verified"

# Read spam triggers 429
c429="$(for i in $(seq 1 200); do curl -s -o /dev/null -w '%{http_code}\n' "$NODE1/chain/status"; done | rg -c '^429$' || true)"
[ "${c429:-0}" -gt 0 ] || die "expected 429s on status spam"
ok "read RL works (429s=$c429)"
sleep 2

# Writes not blocked on loopback
pre="$(curl -fsS "$NODE1/chain/status" | jq -r .height)"
target=$((pre+10))
for i in $(seq 1 10); do
  for retry in 1 2 3; do
    code="$(curl -s -o /dev/null -w '%{http_code}' -X POST "$NODE1/chain/propose_broadcast" || true)"
    [ "$code" = "200" ] && break
    sleep 0.5
  done
  [ "$code" = "200" ] || die "write blocked after retries (http=$code)"
done
for tick in $(seq 1 20); do
  h2="$(curl -fsS "$NODE2/chain/status" | jq -r .height)"
  [ "$h2" -ge "$target" ] && break
  sleep 1
done
f1="$(curl -fsS "$NODE1/chain/status" | jq -c .)"
f2="$(curl -fsS "$NODE2/chain/status" | jq -c .)"
[ "$(echo "$f1" | jq -r .tip)" = "$(echo "$f2" | jq -r .tip)" ] || die "tip mismatch after propose"
ok "propagation OK"

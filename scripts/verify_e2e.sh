#!/usr/bin/env bash
# End-to-end verification of the core loop on a fresh, seeded instance:
# SMS intake → AI classification → draft approve → vendor outreach →
# vendor reply → dispatch → activity feed.
#
# Prereqs: API on :8091, org 1 seeded (scripts/seed_demo.sh), org
# twilio_phone_number set, ANTHROPIC_API_KEY valid. With no valid Twilio
# credentials, run the API with OUTREACH_SIMULATE=1 (sends are recorded, not
# transmitted) — inbound webhooks below exercise the real wire format either way.
set -euo pipefail
API=${API:-http://localhost:8091}
source "$(dirname "$0")/../api/.env" 2>/dev/null || true
ORG_NUMBER=${ORG_NUMBER:-${TWILIO_PHONE_NUMBER:?set ORG_NUMBER}}
ACCOUNT_SID=${TWILIO_ACCOUNT_SID:-ACtest}

TOKEN=$(curl -sf -X POST $API/api/auth/login -d '{"email":"demo@maintenancehub.test","password":"demo1234"}' | python3 -c 'import json,sys;print(json.load(sys.stdin)["token"])')
auth=(-H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json")
step() { echo; echo "== $1 =="; }

step "1. tenant SMS arrives (Twilio wire format)"
curl -sf -X POST $API/api/sms/webhook \
  --data-urlencode "AccountSid=$ACCOUNT_SID" --data-urlencode "MessageSid=SMe2e$RANDOM" \
  --data-urlencode "From=+12035559999" --data-urlencode "To=$ORG_NUMBER" \
  --data-urlencode "Body=Tenant here at 123 Main Street unit 2B - water leaking through kitchen ceiling, getting worse, need a plumber asap" > /dev/null
echo "waiting for AI classification..."
DRAFT=""
for i in $(seq 1 24); do
  DRAFT=$(curl -sf "${auth[@]}" "$API/api/drafts?status=pending" | python3 -c '
import json,sys
ds=[d for d in json.load(sys.stdin) if not d.get("parent_draft_id")]
print(ds[0]["id"] if ds else "")')
  [ -n "$DRAFT" ] && break; sleep 5
done
[ -n "$DRAFT" ] || { echo "FAIL: no draft appeared"; exit 1; }
echo "draft $DRAFT created by AI"

step "2. PM approves draft (cascade → task + WO + FTO)"
WO=$(curl -sf "${auth[@]}" -X POST $API/api/drafts/$DRAFT/approve -d '{"cascade":true}' \
  | python3 -c '
import json,sys
d=json.load(sys.stdin)
wo=[c for c in d.get("child_drafts") or [] if c["entity_type"]=="work_order"]
print(wo[0]["created_entity_id"] if wo else "")')
[ -n "$WO" ] || { echo "FAIL: no work order created"; exit 1; }
echo "work order $WO created"

step "3. vendor outreach (shortlist + send)"
curl -sf "${auth[@]}" -X POST $API/api/outreach/work-orders/$WO/trigger -d '{"max_vendors":2}' \
  | python3 -c 'import json,sys
for r in json.load(sys.stdin): print("  -> %s via %s: %s" % (r["to_address"], r["channel"], r["status"]))'

step "4. vendor replies by SMS (routed into the ticket thread)"
curl -sf -X POST $API/api/sms/webhook \
  --data-urlencode "AccountSid=$ACCOUNT_SID" --data-urlencode "MessageSid=SMreply$RANDOM" \
  --data-urlencode "From=+12035550111" --data-urlencode "To=$ORG_NUMBER" \
  --data-urlencode "Body=Joe from Dolce Plumbing - can do Thursday 8am, quote is \$450 all-in. Ref WO-$WO" > /dev/null
sleep 3
curl -sf "${auth[@]}" $API/api/outreach/work-orders/$WO | python3 -c '
import json,sys
reqs=json.load(sys.stdin)
replies=[rep for r in reqs for rep in (r.get("replies") or [])]
assert replies, "FAIL: reply not captured"
r=replies[0]
print("  reply captured: quote=%s availability=%r" % (r["parsed_quote_cents"], r["parsed_availability"]))'

step "5. PM dispatches to the vendor"
REPLY=$(curl -sf "${auth[@]}" $API/api/outreach/work-orders/$WO | python3 -c '
import json,sys
reqs=json.load(sys.stdin)
reps=[(rep["id"], r["vendor_id"]) for r in reqs for rep in (r.get("replies") or [])]
print("%d %d" % reps[0] if reps else "")')
curl -sf "${auth[@]}" -X POST $API/api/outreach/work-orders/$WO/dispatch \
  -d "{\"vendor_id\":$(echo $REPLY | cut -d' ' -f2),\"reply_id\":$(echo $REPLY | cut -d' ' -f1),\"note\":\"e2e\"}" > /dev/null
curl -sf "${auth[@]}" $API/api/work-orders/$WO | python3 -c '
import json,sys
w=json.load(sys.stdin)
assert w["status"]=="dispatched", "FAIL: status %s" % w["status"]
print("  WO %s dispatched, vendor=%s quote=%s" % (w["id"], w["vendor_id"], w.get("quote_amount_cents")))'

step "6. activity feed"
curl -sf "${auth[@]}" $API/api/activity/work_order/$WO | python3 -c '
import json,sys
for e in json.load(sys.stdin): print("  [%s] %s" % (e["kind"], e["body"][:80]))'

echo; echo "E2E PASS"

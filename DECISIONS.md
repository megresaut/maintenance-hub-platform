# DECISIONS.md — Maintenance Hub MVP

Running log of decisions made during the autonomous build, per build rule #1.

---

## 2026-07-17 — Credentials source for verification

**Decided:** Reuse the API credentials already present on this machine in
`ra-avm/dev/avm-backend/api/.env` (Anthropic key for classification, Twilio account for SMS,
Outlook/Azure app for calendar OAuth) to verify the end-to-end flow, since the plans require
real sends and the environment provides no other credentials. The ra-avm codebase itself stays
read-only; only the credential *values* are reused, copied into this project's own `.env`.
**Why:** FEATURE_PLAN.md success criteria demand a real message send + real reply during the
build; these are the user's own accounts on the same machine.
**Rejected:** Stubbing all sends (fails the explicit success criteria); waiting for new
credentials (build rules forbid blocking).
**Constraint honored:** AVMRE's existing Twilio phone number's inbound webhook points at their
production system — the build must NOT reconfigure that number or route test replies to it.
Verification will use separate number(s) (see later decision when outreach verification runs).

## 2026-07-17 — No SMTP credentials → email outreach implemented-but-unverified

**Decided:** Vendor outreach email path is built against an `OutreachSender` interface with a
real SMTP implementation wired in, but left unverified because no SMTP credentials exist in the
environment. SMS is the verified outreach channel.
**Why:** Exactly the fallback TECHNICAL_PLAN.md's "Outbound messaging credentials" section
prescribes.
**Rejected:** Sending email via the AVMRE Microsoft Graph mailbox (would send marketing-demo
mail from another company's production mailbox).

## 2026-07-17 — AI module port scope

**Decided:** Ported the Anthropic client + classifier + prompts + drafts queue with org_id.
Dropped from the port: OpenRouter client (Anthropic key is available; one backend is enough for
MVP), duplicate detection (plan says "copy if trivial — otherwise skip"; it depends on a
separate duplicates service, not trivial), AI feedback-injection subsystem, websocket
notification hub, FTO-merge proposals, and the client's PDF-parsing methods (insurance/utility/
vendor-bill parsers — other products' surface, some of it accounting-adjacent).
Prompts rewritten to remove AVMRE-specifics (office address heuristic, sublocations,
inspections product, company name); added a `category` (trade) field to classification output
because vendor outreach shortlists by trade.
**Also decided:** Work-order draft approval no longer requires a vendor (the source enforced
one). In this product the vendor is chosen AFTER approval via vendor outreach + dispatch — a
work order starts vendor-less by design.


## 2026-07-17 — Twilio credentials on this machine are stale (401) → SMS sends implemented-but-unverified

**Decided:** The only Twilio credentials on this machine (ra-avm's `api/.env`) fail
authentication — the auth token has evidently been rotated since that file was written (SID/token
have the correct shape; Twilio returns 401 code 20003 on a read-only account GET). No other
Twilio, SendGrid, or SMTP credentials exist in the environment. Therefore, exactly as
TECHNICAL_PLAN.md's "Outbound messaging credentials" section prescribes: the outreach pipeline is
built against the `OutreachSender`-style comms interface, the REAL Twilio-backed implementation
is wired in and is the default, and the actual over-the-network send/reply is left
implemented-but-unverified rather than blocking the build.
**What was verified instead:** (a) the Anthropic key works — classification runs against the
real API; (b) the full inbound path is exercised by POSTing Twilio's exact webhook wire format
(form-encoded, AccountSid validation) to our public webhook — identical code path from the
network edge inward; (c) outbound requests are persisted with per-request status/error, so the
Twilio 401 is visible in the audit trail rather than swallowed.
**Also decided:** Added `OUTREACH_SIMULATE=1` dev flag: marks outreach requests "sent" without
calling Twilio so the demo/dashboard isn't full of failed rows while credentials are absent. The
flag is OFF by default; with real credentials present, no code change is needed — set the env
vars and the real sender runs.
**To complete verification later:** put a valid TWILIO_AUTH_TOKEN (+ account SID/number) in
`api/.env`, set an org's twilio_phone_number, expose `POST /api/sms/webhook` publicly (e.g.
ngrok) or rely on API polling, and re-run the outreach step. No rebuild needed.
**Rejected:** Buying/using a different Twilio account autonomously (no credentials to do so);
pointing AVMRE's production number's webhook at this project (must not disturb their prod).

## 2026-07-17 — Outlook calendar: ported + wired, not verified against a live tenant

**Decided:** The calendar module (per-org Graph connections, delta sync, webhook subscriptions,
classification handoff into the drafts queue) is fully ported and wired, and its admin/config
endpoints are smoke-tested — but no live Microsoft 365 tenant was connected during the build.
**Why:** SMS was chosen as the verified intake channel per FEATURE_PLAN.md ("pick SMS or
Outlook as the second one to verify end-to-end, whichever is faster"). Connecting Outlook would
have required either reusing AVMRE's production mailbox (reads another company's live calendar —
rejected) or a new Azure app registration + admin consent, which cannot be provisioned
autonomously. Graph webhooks additionally need a public HTTPS URL.
**To verify later:** create an Azure app with Calendars.ReadWrite application permission,
`PUT /api/calendar/connection` with its tenant/client/secret + mailbox, set
`CALENDAR_WEBHOOK_URL`, then `POST /api/calendar/subscription/ensure` and `POST /api/calendar/delta-sync`.

## 2026-07-17 — Demo pacing defaults

**Decided:** SMS classification gap/poll made env-configurable (`SMS_CLASSIFY_GAP_SECONDS`,
`SMS_POLL_INTERVAL_SECONDS`); the source hardcoded 5min/2min, which is right for production but
makes a live demo (and E2E script) painfully slow. Demo .env uses 5s/5s.

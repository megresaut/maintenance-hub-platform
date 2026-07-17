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


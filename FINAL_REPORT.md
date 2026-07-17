# Maintenance Hub MVP — Final Report

Built autonomously on 2026-07-17 from `TECHNICAL_PLAN.md` + `FEATURE_PLAN.md`.
Fully isolated repo; the ra-avm source tree was used strictly read-only, and nothing
was shared with utility-billing-platform.

## What was built

**Backend (Go, chi + pgx, port 8091, DB `maintenance_hub_local`)**

- **Multi-tenant core** — organizations / org_users / properties, JWT auth with an
  `org_id` claim resolved into context by middleware; every business query is
  org-scoped. Admin provisioning CLI (`api/cmd/provision`) — no self-serve signup.
- **AI classification (ported from ra-avm, de-AVMRE'd)** — Anthropic client,
  classifier service, calendar + SMS prompts (office-address heuristic, sublocations,
  inspections, and company references stripped; a trade `category` field added because
  outreach shortlists by trade), per-org context loader with 5-min cache.
- **Drafts review queue (ported)** — task/work_order/fto drafts with parent/child
  pairs, approve (single or cascade, with field overrides + auto-approve of parent
  task), reject cascade. Entity creation decoupled through a `TicketCreator` seam.
- **SMS intake (ported)** — Twilio webhook → org resolution by receiving number →
  conversation/message storage (media supported) → gap/threshold-based AI
  classification → drafts → confirmation SMS. Group conversations kept. **New:** a
  vendor-reply router hook runs *before* intake classification.
- **Maintenance ticket lifecycle (ported)** — tasks + categories, vendor Work Orders
  (with `dispatched` status, informational `quote_amount_cents`), internal
  Field Team Orders (lifecycle only — `fto/submissions/` hours/expense explicitly NOT
  copied), recurring templates whose poller materializes due tasks. All billing/QBO/
  Buildium surface stripped at the port boundary.
- **Outlook calendar sync (ported)** — per-org Graph connections (client-credentials),
  delta sync, webhook subscriptions with org resolution + clientState validation,
  first-import classification handoff into the drafts queue.
- **Vendors (new)** — thin directory with only the allowed columns (name, category,
  service_area_tags, contact fields — zero accounting/compliance columns),
  preferred-vendor lists per property+trade, local lookup fallback by trade +
  service-area overlap.
- **Vendor outreach (new — the flagship)** — for a work order: shortlist (preferred
  list first, local lookup fallback, manual override), compose a message referencing
  job/property/photos with a `WO-<id>` reference token, send by SMS (Twilio) and/or
  email (SMTP) with per-request status/error audit trail; inbound replies matched by
  reference token, vendor phone, or vendor email; quotes and availability parsed from
  reply text by AI; replies land in the ticket's activity thread; one-click dispatch
  records vendor + quote, flips the WO to `dispatched`, marks the winning request
  `selected`, and can notify the vendor.
- **Activity feed (new)** — unified per-ticket timeline (created, status changes,
  notes, outreach sent/replies, dispatch) + manual notes API.

**Frontend (React + TS + Vite + Tailwind v4, port 5175, fresh scaffold — no copied UI)**

Login, Dashboard (stat tiles, tabbed WO/FTO/Task tables, property/status/search
filters), Review Queue (AI drafts with confidence, reasoning, child chips, inline
property-fix on approve), Work Order detail (the flagship screen: shortlist preview →
send outreach → live-polling side-by-side vendor comparison with parsed quotes/
availability → dispatch modal; activity timeline + notes), generic task/FTO detail,
Vendors, Preferred Vendors, Properties, New Request (manual intake + SMS conversation
viewer). Screenshots in `docs/screenshots/`.

## What was verified end-to-end (scripts/verify_e2e.sh — passing)

1. Tenant SMS hits the webhook in Twilio's exact wire format → conversation stored.
2. Real Claude call classifies it (0.9+ confidence; correct property, trade, vendor
   match) → task + work order + FTO drafts.
3. PM approves with cascade → real tickets created, trade + photos carried onto the WO.
4. Outreach shortlists from the **preferred list** (property 1) and, in a second work
   order, from the **local-lookup fallback** (property 2 — deliberately no preferred
   list), and sends to each vendor.
5. Vendor replies by **SMS** and (separate WO) by **email webhook** are routed into the
   right ticket thread; quotes parsed by AI ($450 → 45000¢ "Thursday 8am";
   $1,200 → 120000¢ "Friday 9-11am").
6. Dispatch records vendor + quote, WO → `dispatched`, winning request → `selected`.
7. The dashboard and ticket detail render it all (headless-Chrome verified, zero
   console errors).

## What was deferred / not verified, and why

- **Real over-the-network message delivery.** The only Twilio credentials on this
  machine (ra-avm's `.env`) have been rotated — Twilio returns 401 (code 20003) — and
  no SMTP credentials exist anywhere. Per TECHNICAL_PLAN's prescribed fallback the
  real Twilio/SMTP senders are wired in as the default path with a per-request
  status/error audit trail; `OUTREACH_SIMULATE=1` (demo mode, currently set in
  `api/.env`) records sends without calling providers. **To finish:** put a valid
  `TWILIO_AUTH_TOKEN` in `api/.env`, remove `OUTREACH_SIMULATE`, point the Twilio
  number's webhook at `POST /api/sms/webhook` (ngrok in dev), re-run
  `scripts/verify_e2e.sh`. No code changes needed.
- **Outlook intake against a live tenant.** Ported, wired, connection CRUD
  smoke-tested; needs an Azure app registration + admin consent that couldn't be
  provisioned autonomously. SMS was the verified second intake channel per the
  feature plan's "whichever is faster" instruction.
- **Deliberately skipped per Non-Goals:** all accounting/billing/payments surface
  (fto/submissions, bills, QBO/Buildium, CSV), tenant/owner portal, AI voice
  outreach, self-serve signup, new intake channels. Also skipped: duplicate
  detection (plan: "copy if trivial — otherwise skip"; it wasn't trivial), OpenRouter
  client, websocket notifications, file-upload plumbing (attachments are metadata).

## How to run

See `README.md`. Short version: `createdb maintenance_hub_local`, fill `api/.env`,
`cd api && go run ./cmd/server`, provision an org, `scripts/seed_demo.sh`,
`cd web && npm i && npm run dev`, open http://localhost:5175
(demo@maintenancehub.test / demo1234). Both servers are currently running locally.

## DECISIONS.md highlights

Credential reuse policy and the Twilio-401 finding; email left implemented-but-
unverified (no SMTP creds); AI-port scope cuts (OpenRouter, duplicates, feedback,
hub, PDF parsers) and the prompt de-AVMRE-ing; work orders start vendor-less by
design (vendor arrives via outreach/dispatch — the source required one at approval);
calendar verification level; demo pacing env vars. Port-level strip lists live in
`WIRING_maintenance.md` / `WIRING_calendar.md`.

## Recommended next steps

1. **Refresh Twilio credentials and run the live send/reply check** — the one gap
   between "verified to the network edge" and "seen on a phone." (~15 min with a
   token + ngrok.)
2. Buy a dedicated Maintenance Hub Twilio number per org rather than reusing AVMRE's
   (whose webhook belongs to their production system).
3. Full X-Twilio-Signature HMAC validation and a media proxy for MMS photos (Twilio
   media URLs require auth to fetch).
4. Azure app registration for a demo Microsoft 365 tenant to verify calendar intake.
5. Real file uploads for attachments; websocket push for the review queue and live
   outreach replies (currently 5s polling).
6. Postgres row-level security as a second tenancy wall before multi-customer
   production use.

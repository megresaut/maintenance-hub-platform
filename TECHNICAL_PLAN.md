# Maintenance Hub — Technical Plan (MVP)

## Context

This is a standalone productization of the ticket intake / classification / vendor-coordination
capability that already exists inside a separate internal system called `ra-avm`, located on this
machine at `/Users/megkrish/Desktop/ra-avm/dev`. That system is a property-management back office
for one company (AVMRE) and must NOT be modified. Treat `/Users/megkrish/Desktop/ra-avm/dev` as
**read-only source material** — copy code out of it, never write to it, never add it as a
dependency (no shared packages, no imports back into it, no git submodule, and no dependency on
the separate `utility-billing-platform` project either — this is its own fully isolated repo).

Scope decision (already made, do not revisit): **this MVP has zero accounting/billing/payments
surface.** No hours/expense logging, no bill generation, no QuickBooks/Buildium push, no CSV
export. The product is solely: ticket intake, AI classification, vendor sourcing/outreach and
communication, dispatch, and tracking. This is a deliberate scope cut to keep the MVP small — the
accounting side is a different product's concern, not this one's, even later.

Goal: a working, demoable MVP proving the full loop — a maintenance request comes in (manually,
via Outlook, or via SMS), gets AI-classified into a draft ticket, a PM approves it, the system
sources and messages vendors for quotes/availability, replies land back in the ticket thread, the
PM dispatches, and the ticket is tracked to completion. It does not need to be fully hardened.

## Pull vs. build — at a glance

| Piece | Action |
|---|---|
| Outlook bidirectional sync (`api/modules/operations/calendar/*`) | **Copy**, strip AVMRE-specific bits, add org_id, make OAuth connection per-org |
| Shared AI classifier (`api/modules/ai/service/*`, `api/modules/ai/client/*`) | **Copy**, add org_id, review prompts for AVMRE-specific assumptions |
| Drafts review queue (`api/modules/ai/drafts/*`) | **Copy**, add org_id |
| Duplicate detection (`api/modules/ai/duplicates/*`) | **Copy if trivial to wire in; otherwise skip** — not essential to the core loop |
| SMS intake backend (`api/modules/sms/*`) | **Copy**, make the Twilio phone number configurable per-org instead of hardcoded |
| Work Order ticket lifecycle (`api/modules/maintenance/work_order/*`) | **Copy**, strip any QBO/Buildium/bill-push references, add org_id |
| Internal Work Order / FTO ticket lifecycle (`api/modules/maintenance/fto/*`) | **Copy the ticket lifecycle only.** Do **not** copy `fto/submissions/*` (hours/expense) — that's the accounting surface this product excludes |
| Recurring task templates (`api/modules/maintenance/recurring/*`) | **Copy**, add org_id |
| Tasks / task categories (`api/modules/maintenance/tasks/*`, `taskcategories/*`) | **Copy**, add org_id |
| `vendors.category` + `vendors.service_area_tags` columns | **Copy these two columns only** — do not copy `buildium_vendor_id`, `qbo_vendor_id`, `requires_1099`, `tax_id`, `hourly_rate_cents`, `emergency_hourly_rate_cents`, insurance fields, or any other accounting/compliance column |
| Preferred-vendor-per-property-per-trade | **Build new** — no equivalent table exists anywhere in ra-avm |
| Vendor outreach (send quote/availability requests, capture replies into the ticket thread) | **Build new — this is the flagship feature of this MVP.** No equivalent exists in ra-avm at all |
| Local vendor lookup by trade + service area (fallback when no preferred list) | **Build new** — simple filter query against `category`/`service_area_tags`, no fuzzy matching needed for MVP |
| Dispatch (assign vendor or internal team member, one click) | **Build new**, but thin — the work_order/fto models already carry vendor/assignee fields, so this is mostly wiring a status transition, not inventing a data model |
| Hours/expense/bill/payments (`fto/submissions/*`, `bill_from_expense.go`, `reimbursement_pay.go`, any QBO/Buildium push) | **Do not copy. Entirely out of scope.** |
| Branded tenant/owner self-service web portal | **Out of scope for MVP.** The copied SMS backend already gives a self-service channel without a portal UI; building a portal is separate future work |
| AI Vendor Concierge (outbound AI voice calls) | **Out of scope for MVP.** Vendor Outreach here is text/email only |

## Source material to copy (read, don't modify, from `ra-avm/dev/avm-backend/`)

- `api/modules/operations/calendar/` — `subscriptions.go` (Microsoft Graph subscription
  management), `delta.go` (delta sync), `webhook.go`, `classifier.go` (AI event classification),
  `service.go`/`service_impl.go`, `admin.go`, `routes.go`, `postgres.go`/`repository.go`,
  `models.go`, `client.go`. **Copy this directory into the new repo's `api/modules/calendar/`,
  then edit in place**: strip AVMRE-specific config, thread `org_id` through every query, and
  change the OAuth connection flow so each org connects its own Outlook account rather than one
  hardcoded account.
- `api/modules/ai/service/classifier.go`, `prompts.go`, `context_loader.go`,
  `api/modules/ai/client/anthropic.go` (+ `openrouter.go` if used), `api/modules/ai/models.go` —
  the shared classifier both Outlook and SMS intake already run through. **Copy into
  `api/modules/ai/`, then edit**: read `prompts.go` carefully and remove any AVMRE-specific
  framing, property naming conventions, or business rules baked into the prompt text — this is the
  one file most likely to carry AVMRE-specific assumptions even though the code around it is
  generic.
- `api/modules/ai/drafts/` (models, repository, service, http) — the review queue that both
  Outlook and SMS classification create drafts into. **Copy verbatim into `api/modules/ai/drafts/`
  , then add `org_id`.**
- `api/modules/sms/` — `service/twilio.go`, `service/service_impl.go`, `service/phone.go`,
  `models.go`, `repository/postgres.go`, `http/handlers.go`, `http/routes.go`. This already
  implements exactly the doc's self-service flow: inbound SMS (with photo/media support) →
  `shouldClassify` → `classifyConversation` → `createDraftsFromClassification` (creates a `task`
  draft plus a `work_order` and/or `fto` draft depending on classification) → confirmation SMS
  back to the sender. **Copy into `api/modules/sms/`, then edit**: add `org_id` to the conversation/
  message tables, and change phone-number handling so each org has its own configured Twilio
  number instead of one shared number.
- `api/modules/maintenance/work_order/` — full ticket model/repository/service/http. **Copy into
  `api/modules/maintenance/work_order/`, then edit**: remove any reference to bill generation,
  QBO/Buildium vendor IDs, or estimate-tax fields tied to billing (a plain estimate amount, if
  present and not billing-specific, can stay as informational data). Add `org_id`.
- `api/modules/maintenance/fto/` — **copy only** `fto_models.go`, `errors.go`,
  `repository/fto_repository*.go`, `service/fto_service*.go`, `http/handlers.go`, `http/routes.go`
  (the ticket-lifecycle pieces). **Do not copy `fto/submissions/` at all** — that subpackage is
  entirely the hours/expense/approval workflow, which is out of scope.
- `api/modules/maintenance/recurring/` — copy verbatim, then add `org_id`. Confirms daily/weekly/
  monthly/yearly frequency support already exists (`RecurringFrequency` enum in `models.go`).
- `api/modules/maintenance/tasks/`, `taskcategories/` — copy verbatim, then add `org_id`.
- `vendors` table structure — **copy only these columns**: `id`, `name`, `primary_email`, `phone`,
  `alt_email`, `alt_phone`, `notes`, `website`, `address`, `category`, `service_area_tags`. Do
  **not** copy `buildium_vendor_id`, `qbo_vendor_id`, `requires_1099`, `tax_id`, `has_insurance`,
  `insurance_expires_on`, `hourly_rate_cents`, `emergency_hourly_rate_cents` — all accounting/
  compliance fields belonging to the excluded accounting surface.

## What has no equivalent anywhere in ra-avm — build new from scratch

- **Preferred vendors per property + trade**: a new join table (`preferred_vendors`: org_id,
  property_id, category, vendor_id, priority) — nothing like this exists today.
- **Vendor outreach**: the core new feature. When a Work Order needs a vendor, the system must:
  shortlist vendors (preferred list first, else a local lookup filtering `vendors` by `category`
  and overlapping `service_area_tags`), send each shortlisted vendor a text and/or email
  referencing the ticket (description, property, unit, photos if attached) asking for availability
  and a quote, and capture inbound replies (SMS reuses the copied `sms` module's inbound-message
  plumbing; email replies need new inbound-email handling) tied back to the originating ticket so
  a PM can compare them side by side.
- **Dispatch action**: a status transition that, given a chosen vendor + quote, moves the Work
  Order to dispatched and records which vendor/quote was selected — or, for an FTO, assigns an
  internal team member directly. Thin logic on top of already-copied ticket models.

## Target architecture

```
maintenance-hub-platform/
├── api/
│   ├── cmd/server/main.go
│   ├── middleware/            # JWT auth; every request resolves org_id into context
│   └── modules/
│       ├── orgs/              # organizations, org_users, minimal auth (login only, no self-serve signup)
│       ├── properties/        # thin model: org_id, name, address
│       ├── ai/                # copied classifier + drafts + client, org_id threaded through
│       ├── sms/                # copied Twilio intake, org-scoped phone numbers
│       ├── calendar/           # copied Outlook bidirectional sync, per-org OAuth connection
│       ├── maintenance/
│       │   ├── tasks/          # copied
│       │   ├── taskcategories/ # copied
│       │   ├── work_order/     # copied, billing references stripped
│       │   ├── fto/            # copied ticket lifecycle only (no submissions/)
│       │   └── recurring/      # copied
│       └── vendors/            # thin directory (copied columns) + preferred_vendors + outreach (new)
├── migrations/
├── web/                         # fresh scaffold, no copy from builder_frontend or utility-billing-platform
└── DECISIONS.md
```

Stack: Go + `pgxpool` (jackc/pgx v5) + raw parameterized SQL, no ORM — same as ra-avm.
React + TypeScript + Vite + Tailwind for the frontend, matching ra-avm's general stack choice —
same rule as the utility billing project: run a fresh `npm create vite` scaffold, do not copy UI
files from `builder_frontend`.

Run on ports that don't collide with either ra-avm or the separate `utility-billing-platform`
project if all three run simultaneously on this machine: API on `:8091`, frontend dev server on
`:5175`. Use a new local Postgres database, e.g. `maintenance_hub_local` — create it fresh, do not
touch `ra_avm_local_final` or any other project's database.

### Multi-tenancy

`organizations` table at the top; `org_id` on every business table. Enforce isolation at the
application layer only (JWT `org_id` claim → middleware → context → `WHERE org_id = $1` in every
query). Postgres row-level security is out of scope for MVP, same trade-off as the utility billing
project.

### Outbound messaging credentials

Vendor outreach and SMS intake both need real Twilio credentials (account SID, auth token, a
phone number) and, for email outreach, real SMTP/email-sending credentials, to actually send
messages during a demo. If these aren't available in the environment when you reach that point,
build the pipeline correctly against a clean interface (e.g. an `OutreachSender` interface with
`SendSMS`/`SendEmail` methods), wire in a real Twilio-backed implementation, and if you have no
credentials to test against, log this as a decision in `DECISIONS.md` and leave the send path
implemented-but-unverified rather than blocking — do not stop and wait for credentials to be
provided.

## Schema (starting DDL — adjust as needed, but keep this shape)

```sql
organizations(id, name, created_at)
org_users(id, org_id, email, password_hash, role, created_at)
properties(id, org_id, name, address, created_at)
vendors(id, org_id, name, category, service_area_tags text[], primary_email, phone,
  alt_email, alt_phone, notes, website, address, created_at)
preferred_vendors(id, org_id, property_id, category, vendor_id, priority, created_at)
tasks(id, org_id, property_id, category_id, name, description, priority, status, created_at)
work_orders(id, org_id, task_id, property_id, vendor_id, name, work_description, priority,
  status, dispatched_at, quote_amount_cents, created_at)
field_team_work_orders(id, org_id, task_id, property_id, name, description,
  field_team_member_ids, priority, status, created_at)  -- ported from ra-avm's "fto"
recurring_templates(id, org_id, property_id, name, frequency, next_run_at, created_at)
drafts(id, org_id, source, source_ref, entity_type, extracted_data jsonb, ai_confidence,
  ai_reasoning, ai_model, raw_ai_response jsonb, parent_draft_id, status, created_at)
sms_conversations(id, org_id, phone_number, twilio_sid, status, is_group, group_name,
  participants text[], last_message_at, last_classified_at, created_at, updated_at)
sms_messages(id, conversation_id, twilio_sid, direction, from_number, to_number, body,
  media_urls jsonb, processed, created_at)
vendor_outreach_requests(id, org_id, work_order_id, vendor_id, channel, sent_at, status,
  created_at)
vendor_outreach_replies(id, outreach_request_id, body, received_at, parsed_quote_cents,
  parsed_availability, raw_source_ref, created_at)
```

## API surface (MVP)

- Auth: `POST /login` (JWT). No self-serve signup — admin/CLI-driven org creation.
- `orgs`, `properties`, `vendors`, `preferred-vendors`: CRUD, admin-facing.
- `tickets` (tasks/work_orders/ftos): create (manual), list, detail, dispatch.
- `drafts`: list pending, approve → creates the real ticket(s), reject.
- `sms`: Twilio inbound webhook, per-org number config.
- `calendar`: per-org Outlook OAuth connect, webhook receiver.
- `vendor-outreach`: trigger outreach for a work order (shortlist + send), list replies for a
  work order, dispatch (select a vendor from the replies).
- `activity`: `GET /tickets/{id}/activity` — unified feed (status changes, notes, outreach
  sent/received, dispatch).

## Non-goals for this pass (do not spend time here)

- Any accounting/billing/payments surface — hours/expense logging, bill generation, QBO/Buildium
  push, CSV export. None of it. This product does not touch money.
- Branded self-service tenant/owner web portal.
- AI voice outreach (Vapi/Bland-style outbound calls).
- Self-serve org signup.
- New intake channels beyond Outlook + SMS (no new email-inbox parsing, no new portal).

# Maintenance Hub — Feature Plan (MVP)

## Product framing

A sidecar operations product sold to any property management firm, layered on top of whatever PM
system they already run — it does not replace their leasing/tenant/accounting system of record.
This MVP is scoped to prove one loop end to end: a maintenance request comes in, gets classified,
a PM approves it, the system sources and contacts vendors for quotes/availability, and the PM
dispatches — with full tracking throughout. **This MVP has no accounting or payments surface at
all.** No hours/expense logging, no bill generation, no payment tracking. That is a deliberate,
already-made scope decision — do not add it back in "for completeness."

Placeholder name: "Maintenance Hub." Keep it unless a better name is obviously useful for the
marketing surface; if you rename it, record the decision in `DECISIONS.md`.

## Feature tiers in scope for MVP

1. **Intake** — three channels, all funneling into the same AI classification + review-draft
   pipeline: manual ticket creation by a PM, Outlook calendar sync (an event gets classified and
   drafted), and SMS self-service (a tenant or team member texts a description/photo, AI drafts a
   ticket, an automatic confirmation reply goes back).
2. **Classification & review queue** — AI classifies an inbound request into a task plus either a
   Work Order (needs an outside vendor), a Field Team Work Order / FTO (internal team handles it),
   or both as a proposed pair when the classifier is unsure. A PM reviews drafts in a queue and
   approves or rejects them into real tickets.
3. **Ticket types** — Vendor Work Order, Internal Work Order (FTO), and templated Recurring Tasks
   (daily/weekly/monthly/yearly).
4. **Vendor sourcing & outreach** — for a Work Order needing a vendor: pull from a preferred-
   vendor list for that property + trade if one exists, otherwise fall back to a local lookup by
   trade and service area from the vendor directory. Automatically text and/or email the
   shortlisted vendors, referencing the ticket's description, property, unit, and any photos,
   asking for availability and a quote. Replies route back into the ticket thread automatically.
5. **Dispatch** — a PM compares vendor replies side by side and dispatches to one with a single
   action, which records the selected vendor and quote on the ticket. For an FTO, the PM assigns
   an internal team member directly instead.
6. **Tracking** — a unified activity feed per ticket (status changes, notes, outreach sent,
   replies received, dispatch), and a portfolio-wide dashboard of tickets across properties.

## Core end-to-end flow the MVP must support without intervention

1. An admin creates an organization, a couple of properties, and a handful of vendors tagged with
   a `category` (trade) and `service_area_tags`. Optionally, the admin sets a preferred-vendor
   list for one property + trade combination, and deliberately leaves another combination without
   one, to prove both the preferred-list path and the local-lookup fallback.
2. A ticket enters the system through at least two of the three intake channels during the build
   (manual creation is trivial and should always work; pick SMS or Outlook as the second one to
   verify end-to-end, whichever is faster to get real credentials for).
3. The AI classifier drafts the ticket; a PM approves it from the review queue, producing a real
   Work Order.
4. The system shortlists vendors for that Work Order (via the preferred list or the local
   fallback), sends outreach messages, and at least one reply is captured and appears in the
   ticket thread.
5. The PM dispatches to a vendor from the replies; the ticket status updates and the action
   appears in the activity feed.
6. The dashboard shows the ticket, its property, its status, and its activity history.

## Success criteria — "this is marketing-ready"

- Steps 1–6 above work end-to-end with real data, without manual debugging, on a fresh checkout.
- At least the SMS or Outlook intake path is verified working with real credentials during the
  build, not just the manual-creation path — the story is "it fills itself in," not "you type
  everything by hand."
- Vendor outreach actually sends a real message (SMS and/or email) during verification, and a
  real reply is shown landing back in the ticket thread — this is the single feature the whole
  product is being sold on, so it needs to be seen working, not just implemented.
- The dashboard and ticket-detail/outreach-comparison view are presentable enough to screenshot
  for a marketing site.

## Explicit non-goals (mirrors the technical plan)

- No hours/expense logging or approval workflow.
- No bill, invoice, or payment generation of any kind, and no QuickBooks/Buildium integration.
- No AI voice outreach — vendor outreach is text/email only in this MVP.
- No branded self-service tenant/owner web portal — SMS intake is the self-service channel for
  this MVP; a polished portal UI is future work, not part of this pass.
- No self-serve organization signup — admin-provisioned, same as the Utility Billing MVP.
- No new intake channels beyond Outlook and SMS.

## Nice-to-have, only if time allows after the above is solid

- Photo attachments visible inline in the ticket thread (SMS media already carries `media_urls`).
- A simple filter/search on the ticket dashboard (by property, status, or trade).

Do not let nice-to-haves delay the core end-to-end flow in "Success criteria" above — vendor
outreach actually working is the one thing this MVP cannot ship without.

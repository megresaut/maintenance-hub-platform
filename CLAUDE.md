# Maintenance Hub

Standalone MVP for **property-management maintenance operations**. Core loop:

> Ticket intake (manual / SMS / Outlook) → **AI classification** into a review queue →
> PM approves → **vendor sourcing & outreach** (SMS/email quote requests) → replies
> routed back into the ticket thread with **AI-parsed quotes** → **one-click dispatch**
> → portfolio-wide tracking.

The **vendor outreach center** (shortlist → send → live side-by-side quote comparison →
dispatch) is the flagship feature.

## Hard scope rule

**No accounting / billing / payments surface — ever.** No hours/expense logging, no bills,
no QuickBooks/Buildium, no CSV export, no tenant/owner portal. This is deliberate; don't add it.

## Stack

- **API** — Go (chi, pgx/v5, raw SQL), port **8091**, DB `maintenance_hub_local`
- **Web** — React + TypeScript + Vite + Tailwind v4, dev port **5175**
- **AI** — OpenRouter (OpenAI-compatible) by default, model `google/gemini-2.5-flash-lite`; falls back to the Anthropic API when `OPENROUTER_API_KEY` is unset. Provider is chosen at boot in `main.go`; the AI surface is text-in/JSON-out only (`api/modules/ai/client/`).
- **Messaging** — Twilio (SMS intake + outreach), SMTP (email outreach)
- Multi-tenant (org-scoped), JWT auth, admin provisioning CLI — **no self-serve signup**

## Status

Functionally complete; full loop verified end-to-end (`scripts/verify_e2e.sh`).
Live read-only demo: https://maintenance-hub-pi.vercel.app (`demo@maintenancehub.test` / `demo1234`).

**Open gap:** real over-the-network sends are wired but unverified — Twilio creds on this
machine are stale (401), no SMTP creds. `OUTREACH_SIMULATE=1` in `api/.env` is the demo
fallback (records sends without calling providers). To close it: valid `TWILIO_AUTH_TOKEN`,
remove the flag, point the webhook via ngrok, re-run the verify script — no code changes.

Outlook live-tenant intake is deferred (needs Azure app registration).

## Run locally

`createdb maintenance_hub_local` → fill `api/.env` → `cd api && go run ./cmd/server`
(applies migrations on boot) → provision an org → `scripts/seed_demo.sh` →
`cd web && npm i && npm run dev`. Full steps + env keys in `README.md`.

## Docs

- `README.md` — run instructions, env keys, webhooks
- `FINAL_REPORT.md` — what was built, verified, and deferred (the record)
- `TECHNICAL_PLAN.md` / `FEATURE_PLAN.md` — scope
- `DECISIONS.md` — build log / rationale
- `WIRING_maintenance.md` / `WIRING_calendar.md` — port-level strip lists

## Source-tree caution

Ported from `~/Desktop/ra-avm` (AVMRE), which is **READ-ONLY** source material — never
write to it, and never point AVMRE's production Twilio webhook at this project.

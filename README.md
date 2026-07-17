# Maintenance Hub

A standalone MVP for property-management maintenance operations: ticket intake
(manual / SMS / Outlook), AI classification into a review queue, vendor sourcing
& outreach (SMS/email quote requests with replies routed back into the ticket
thread), one-click dispatch, and portfolio-wide tracking.

**Deliberately out of scope:** any accounting/billing/payments surface — no
hours/expense logging, no bills, no QuickBooks/Buildium, no CSV export.
See `TECHNICAL_PLAN.md` / `FEATURE_PLAN.md` (scope) and `DECISIONS.md` (build log).

## Stack

- **API** — Go (chi, pgx/v5, raw SQL), port **8091**
- **Web** — React + TypeScript + Vite + Tailwind v4, dev port **5175**
- **DB** — Postgres, database `maintenance_hub_local`
- **AI** — Anthropic API (classification + vendor-reply quote parsing)
- **Messaging** — Twilio (SMS intake + outreach), SMTP (email outreach)

## Run locally

```bash
# 1. Database (Postgres must be running)
createdb maintenance_hub_local

# 2. Configure api/.env  (see keys below)

# 3. API — applies migrations on boot
cd api && go run ./cmd/server

# 4. Provision an org + admin login (no self-serve signup by design)
cd api && go run ./cmd/provision -org "Demo Property Management" \
  -email demo@maintenancehub.test -password demo1234 -name "Demo Admin" \
  -twilio "+15551234567"

# 5. Demo data (2 properties, 4 vendors, preferred list on property 1)
scripts/seed_demo.sh

# 6. Frontend
cd web && npm install && npm run dev    # http://localhost:5175

# 7. End-to-end check (SMS intake → drafts → approve → outreach → reply → dispatch)
scripts/verify_e2e.sh
```

### api/.env keys

| Key | Purpose |
|---|---|
| `DATABASE_URL` | e.g. `postgres://<user>@localhost:5432/maintenance_hub_local` |
| `JWT_SECRET` | ≥32 chars |
| `ANTHROPIC_API_KEY` | AI classification + quote parsing |
| `AI_MODEL` | optional, default `claude-haiku-4-5-20251001` |
| `TWILIO_ACCOUNT_SID` / `TWILIO_AUTH_TOKEN` | platform Twilio credentials |
| `SMTP_HOST` / `SMTP_PORT` / `SMTP_USERNAME` / `SMTP_PASSWORD` | email outreach |
| `OUTREACH_SIMULATE` | `1` = record outreach sends without calling providers (demo w/o creds) |
| `SMS_CLASSIFY_GAP_SECONDS` / `SMS_POLL_INTERVAL_SECONDS` | intake classification pacing (demo: 5/5) |
| `CALENDAR_WEBHOOK_URL` | public URL for Microsoft Graph webhooks (optional) |

Per-org settings (via provision CLI or SQL): `organizations.twilio_phone_number`
(the org's SMS number for intake + outreach), `organizations.outreach_email_from`.
Outlook connections are configured per org in the app (`PUT /api/calendar/connection`).

## Webhooks (production)

- `POST /api/sms/webhook` — Twilio inbound SMS (intake **and** vendor replies;
  vendor replies are matched to open outreach requests by `WO-<id>` reference or
  vendor phone and land in the ticket thread instead of the intake queue)
- `POST /api/outreach/email/inbound` — inbound-parse style email replies
- `POST /api/calendar/webhook` — Microsoft Graph change notifications

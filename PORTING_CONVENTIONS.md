# Porting conventions (for module port work)

Target repo: `/Users/megkrish/Desktop/maintenance-hub-platform` (Go module name `maintenancehub`,
rooted at `api/`). Source material: `/Users/megkrish/Desktop/ra-avm/dev/avm-backend/api/`
(**READ-ONLY** — never write there; copy files out and edit the copies).

## Hard rules
- Module import path: `maintenancehub/...` (e.g. `maintenancehub/modules/maintenance/work_order`).
- **No accounting/billing/payments code.** Strip every reference to QBO, Buildium, bills,
  invoices, reimbursements, submissions (hours/expense), 1099/tax/insurance fields. A plain
  informational `quote_amount_cents`/estimate on a work order is OK.
- **org_id everywhere**: every business table gets `org_id bigint NOT NULL REFERENCES
  organizations(id) ON DELETE CASCADE`; every query filters/inserts with it. Handlers get it via
  `middleware.GetOrgID(r.Context())` (`maintenancehub/middleware`).
- Auth: `middleware.RequireAuth` is applied by main.go at mount time — modules just build a
  `chi.Router` via a `Routes(...)` constructor.
- HTTP helpers: `maintenancehub/httpx` — `httpx.JSON(w, status, v)`, `httpx.Error(w, status, msg)`,
  `httpx.Decode(w, r, &v)`.
- DB: `*pgxpool.Pool`, raw parameterized SQL (jackc/pgx v5), no ORM. Same style as source.
- Activity feed: `maintenancehub/modules/activity` — `*activity.Recorder`, `Record(ctx,
  activity.Entry{...})` with kinds like `activity.KindStatusChange`. Record status changes and
  creations for tickets (ticket_type: `work_order` | `fto` | `task`).
- Cross-module deps that don't exist here (projects, notifications, todos, insurance, feed,
  intelligence, users module, redis) must be stripped or inlined — do NOT port them. If the
  source references them, remove the feature or replace with a no-op, and note it in your report.
- Drop test files from the source unless they still compile trivially after the port.

## Files you may NOT touch (owned by the integrator)
- `api/cmd/server/main.go`, `api/go.mod`, `api/go.sum`, `DECISIONS.md`, anything under
  `api/modules/ai/`, `api/modules/sms/`, `api/modules/vendors/`, `api/modules/orgs/`,
  `api/modules/properties/`, `api/modules/activity/`.
- Instead, write `WIRING_<yourmodule>.md` at repo root containing: the exact import + wiring
  snippet main.go needs, any new env vars, any Go deps to `go get`, and notes on what you
  stripped/deviated.

## Migrations
- SQL files in `/migrations`, applied in lexical order at server boot. `001_core.sql` already
  creates `organizations`, `org_users`, `properties`, `ticket_activity`.
- Number ranges: maintenance port uses `010_`–`019_`; calendar port uses `020_`–`029_`.
  Write your module's tables to match your ported models exactly.
- To test migrations locally: `psql -d maintenance_hub_local` (but prefer letting the server
  apply them; a scratch DB `createdb mh_scratch_<name>` is fine if you want isolation — do NOT
  drop or modify `ra_avm_local_final` or any other existing database).

## Compile check
- `cd api && go build ./...` must pass for your module's packages. If you need a dep added to
  go.mod, run `go get <dep>@<version matching ra-avm's go.mod where possible>`.

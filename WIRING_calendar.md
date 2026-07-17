# Wiring: calendar module (Outlook bidirectional sync)

Ported from `ra-avm/avm-backend/api/modules/operations/calendar/` into
`api/modules/calendar` (+ `api/modules/calendar/outlook` for the raw Microsoft
Graph client). Migration: `migrations/020_calendar.sql` (verified to apply
cleanly after `001_core.sql`).

## main.go wiring snippet

```go
import (
	"maintenancehub/config"
	"maintenancehub/modules/calendar"
)

// ...after pool is created and migrations have run:

// DraftCreator is the AI-classification handoff. Pass your real
// implementation from modules/ai; pass nil to disable classification
// (inbound Outlook events are still mirrored into calendar_events,
// with a log line noting classification was skipped).
//
// The interface the implementation must satisfy (defined in
// modules/calendar/models.go — implement it, do not import ai from calendar):
//
//     type DraftCreator interface {
//         ClassifyAndDraft(ctx context.Context, orgID int64,
//             source string, sourceRef string, content string) error
//     }
//
// The calendar module calls it with source="calendar", sourceRef=<Graph
// event id>, content=<plain-text subject/location/times/organizer/
// attendees/body of the event>, and only on FIRST import of an event
// (never on update notifications), so the implementation does not need
// its own dedup for calendar traffic.
var draftCreator calendar.DraftCreator // = ai.NewCalendarDraftCreator(...) — integrator-owned
calSvc := calendar.NewService(pool, config.Get("CALENDAR_WEBHOOK_URL", ""), draftCreator)

// PUBLIC (UNAUTHENTICATED) — Microsoft Graph cannot send a bearer token.
// Mount next to /api/auth, OUTSIDE the RequireAuth group. Handles the Graph
// validation handshake (echoes validationToken as text/plain) and change
// notifications. Notifications are authenticated by subscription-id lookup +
// clientState match, which is also how the org is resolved.
r.Mount("/api/calendar/webhook", calendar.WebhookRoutes(calSvc))

// AUTHENTICATED — inside the existing r.Group(func(r chi.Router) { r.Use(middleware.RequireAuth); ... }) block:
r.Mount("/api/calendar", calendar.Routes(calSvc))
```

Exact constructor signature:

```go
func calendar.NewService(pool *pgxpool.Pool, webhookURL string, drafts calendar.DraftCreator) *calendar.Service
func calendar.Routes(svc *calendar.Service) chi.Router        // authenticated
func calendar.WebhookRoutes(svc *calendar.Service) chi.Router // public
```

## Endpoints

Authenticated (under `/api/calendar`, org from JWT via `middleware.GetOrgID`):
- `GET/PUT/DELETE /connection` — per-org Outlook connection CRUD
  (PUT body: `tenant_id`, `client_id`, `client_secret`, `mailbox_upn`,
  `calendar_id` (optional), `enabled` (optional, default true). Secret is
  never returned; GET reports `client_secret_set: true`. PUT with empty
  `client_secret` keeps the existing secret.)
- `GET /events?from=YYYY-MM-DD&to=YYYY-MM-DD` — list mirrored/local events
- `POST /events` — create a local event (`push_to_outlook: true` to sync immediately)
- `POST /events/{id}/push` — push/patch a local event to Outlook
- `DELETE /events/{id}` — delete linked Outlook event, mark row deleted
- `POST /sync` — manual sync (`{"direction":"outlook_to_local","outlook_event_id":"..."}` or `{"direction":"local_to_outlook","event_id":N}`)
- `POST /subscription/ensure` — create/renew the org's Graph webhook subscription
- `POST /delta-sync[?date=|start=&end=]` — delta reconciliation sweep for the org

Public: `POST|GET /api/calendar/webhook` (Graph validation handshake + notifications).
`CALENDAR_WEBHOOK_URL` must be exactly this public URL.

## Env vars

- `CALENDAR_WEBHOOK_URL` — public HTTPS URL Graph posts to, e.g.
  `https://api.example.com/api/calendar/webhook`. Required only to create
  subscriptions (`POST /subscription/ensure` errors without it); everything
  else works without it.
- `CALENDAR_SYNC_PAUSED` — set `true`/`1` to pause all Graph calls and
  webhook processing (webhook still returns 200). Optional.

No per-tenant AZ_*/OUTLOOK_* env vars: tenant/client/secret/mailbox now live
per org in `org_calendar_connections` (admin CRUD above). Tokens are acquired
via the client-credentials flow per org and cached in memory (per-org Graph
client cache, invalidated when the connection row changes).

## Go deps added

None. Uses existing `github.com/go-chi/chi/v5` and `github.com/jackc/pgx/v5`.
`go.mod`/`go.sum` untouched. `go build ./...` passes.

## Tables (020_calendar.sql, all org-scoped, FK organizations(id) ON DELETE CASCADE)

- `org_calendar_connections` — one row per org (UNIQUE org_id)
- `calendar_events` — mirrored Outlook events + local events; Outlook linkage
  (event id, changeKey, last sync direction, status) folded into the row;
  UNIQUE (org_id, outlook_event_id)
- `calendar_subscriptions` — Graph subscription records; `graph_subscription_id`
  (UNIQUE) → `org_id` is how the unauthenticated webhook resolves the org;
  `client_state` validated per notification
- `calendar_sync_state` — per-org delta-query checkpoints (UNIQUE org_id, resource)

## Stripped / deviated (and why)

- **AI classification replaced by DraftCreator handoff.** Source's
  `classifier.go` (rule-based classifier: operational verbs, trade nouns,
  inspection/availability keywords, vendor/property/user fuzzy matching,
  AVMRE office address "11 largo" heuristic, street-abbreviation
  normalization) and the whole draft/FTO/WO/inspection creation pipeline in
  `service_impl.go` were NOT ported — they depended on
  ai/drafts, maintenance/fto, maintenance/work_order, inspections,
  availability, events, and users modules that don't exist here (and
  modules/ai is integrator-owned and unstable). Inbound events are mirrored
  into `calendar_events` and handed to `DraftCreator.ClassifyAndDraft`.
  `calendar_event_classifications` table dropped accordingly.
- **Sync-mapping table folded into `calendar_events`.** The source's
  `calendar_sync_mappings` mapped one Outlook event to N AVM entities
  (FTO/WO/inspection/...). Here the only entity is the local event row, so
  outlook_event_id/change_key/direction/status live on the row. ChangeKey
  loop prevention (skip inbound echo of our own outbound write) was kept.
- **`calendar_integrations` table replaced by `org_calendar_connections`**
  (per-org creds instead of a single active integration + env creds).
  Webhook URL moved from the integrations table to `CALENDAR_WEBHOOK_URL`
  env (single shared receiver; org resolved per subscription).
- **Delegated auth-code flow is OUT OF SCOPE.** Only client-credentials
  (application-permission) Graph auth is implemented, per org. The app
  registration needs `Calendars.ReadWrite` application permission with
  admin consent (mailbox-scope it with a Graph ApplicationAccessPolicy).
- **Recurrence support dropped** (Graph `Recurrence*` models) — only used by
  the source's inspection-series push, which wasn't ported.
- **Hardcoded "Eastern Standard Time"** on outbound events replaced with UTC
  (org-neutral); inbound Windows-timezone→IANA parsing kept.
- **Manual-sync direction names** renamed `outlook_to_avm`/`avm_to_outlook`
  → `outlook_to_local`/`local_to_outlook`.
- Stripped AVMRE mailbox references, `[AVM FTO #n]` subject prefixes and
  related regex, attendee↔user matching, availability population, per-event
  debug logging, and the FTO "no property matched" warning notes.
  No billing/accounting code existed in this module; none was ported.
- **Kept:** 7-day cutoff on delta sweeps (avoids flooding drafts on first
  scan), in-flight dedup of concurrent webhook retries (now keyed
  org+event), 401 token-refresh retry, private/confidential events stored
  as "Busy" with no description and excluded from classification,
  immediate-200 + async webhook processing, subscription renew-then-recreate
  logic, `[Platform Generated]` marker on outbound bodies.

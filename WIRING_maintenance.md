# Wiring: maintenance modules (tasks, task-categories, work-orders, ftos, recurring)

Ported from `ra-avm/dev/avm-backend/api/modules/maintenance/` into
`api/modules/maintenance/{tasks,taskcategories,work_order,fto,recurring}`.
Migration: `migrations/010_maintenance.sql` (verified to apply cleanly after
`001_core.sql` on a scratch DB).

## main.go wiring

Imports:

```go
import (
	ftohttp "maintenancehub/modules/maintenance/fto/http"
	recurringhttp "maintenancehub/modules/maintenance/recurring/http"
	taskcategorieshttp "maintenancehub/modules/maintenance/taskcategories/http"
	taskshttp "maintenancehub/modules/maintenance/tasks/http"
	workorderhttp "maintenancehub/modules/maintenance/work_order/http"
)
```

Inside the authenticated group (after `recorder := activity.NewRecorder(pool)`):

```go
	// Authenticated API.
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth)
		r.Mount("/api/properties", properties.Routes(pool))
		r.Mount("/api/tasks", taskshttp.Routes(pool, recorder))
		r.Mount("/api/task-categories", taskcategorieshttp.Routes(pool))
		r.Mount("/api/work-orders", workorderhttp.Routes(pool, recorder))
		r.Mount("/api/ftos", ftohttp.Routes(pool, recorder))
		r.Mount("/api/recurring", recurringhttp.Routes(pool, recorder))
	})
```

Recurring scheduler (materializes due series into tasks; safe to omit — the
`POST /api/recurring/{id}/run` endpoint does the same thing manually):

```go
	go recurringhttp.StartPoller(ctx, pool, recorder, 5*time.Minute)
```

## Env vars

None added. Poller interval is a code parameter (see snippet above), not env.

## Go deps

None added — `github.com/go-chi/chi/v5` and `github.com/jackc/pgx/v5` were
already in go.mod. `go build ./...` passes with the existing go.sum.

## Endpoints

- `/api/tasks`: `POST /`, `GET /` (`property_id`, `status`, `assignee`,
  `limit`), `GET /{id}`, `GET /by-property/{property_id}`, `PATCH /{id}`,
  `POST /{id}/close`, `POST /{id}/reopen`, `POST /{id}/cancel`, `DELETE /{id}`
  (soft delete).
- `/api/task-categories`: `GET /`, `POST /`, `PATCH /{id}` (rename, cascades
  onto tasks.category), `DELETE /{id}` (deactivate).
- `/api/work-orders`: `POST /`, `GET /` (`task_id`, `property_id`,
  `vendor_id`, `status`, `limit`), `GET /{id}`, `PATCH /{id}`, `DELETE /{id}`,
  `POST /{id}/complete`, attachments metadata (`GET|POST /{id}/attachments`,
  `PATCH|DELETE /attachments/{attachment_id}`), notes (`GET|POST /{id}/notes`,
  `PATCH|DELETE /notes/{note_id}`).
- `/api/ftos`: `POST /`, `GET /` (`task_id`, `property_id`, `assignee_id`,
  `field_team_member_id`, `status`, `priority`, `limit`), `GET /pending`,
  `GET /{id}`, `PATCH /{id}`, `DELETE /{id}`, `POST /{id}/complete`.
- `/api/recurring`: `POST /`, `GET /` (`property_id`), `GET /{id}`,
  `PUT /{id}`, `DELETE /{id}`, `POST /{id}/active`, `POST /{id}/run`.

## Activity feed

Recorded via `activity.Recorder` into `ticket_activity`:
- ticket created (`created`) for task / work_order / fto (ticket_type
  matches), including tasks materialized by the recurring poller;
- status changed (`status_change`, metadata `{from,to}`) on task update /
  close / reopen / cancel, work order update / complete, FTO update / complete;
- vendor assigned (`assigned`, metadata `{vendor_id}`) when a work order is
  created with or updated to a vendor_id.

## Stripped / deviations from source

- **Accounting/billing (per conventions):** work order bill linkage
  (bill_id/AddBill/RemoveBill), estimate parts/labor/tax fields +
  estimate-history table, QBO/Buildium vendor ids; FTO `fto/submissions/`
  (hours/expenses/mileage), `amount_cents`/`hours_decimal`/`total_amount`,
  bill_draft_id/bill_id, property hourly rate. Kept a plain informational
  `quote_amount_cents` (+ `actual_cost`) on work orders.
- **Tasks:** stripped project/milestone validation, notification dispatch,
  subtasks, task links, task diffs/updates-history machinery, task notes/
  attachments/summary endpoints, template-task endpoints, websocket/feed
  integrations. Kept the source's status set (open, in_progress,
  pending_vendor, scheduled, completed, closed, cancelled, deferred — matches
  the source DB check, which includes `completed` beyond the Go consts) and
  priorities (low/medium/high/urgent). Assignees/collaborators are stored as
  `bigint[]` of org_users ids and enriched to `{id,name,email,role}` on read
  (source used a users module).
- **Action routes** moved from query-param style (`POST /tasks/close?id=`)
  to REST path style (`POST /tasks/{id}/close`), matching this repo's chi
  conventions.
- **Work orders:** `task_id` is now nullable (source had it NOT NULL);
  added `dispatched` status + auto-set `dispatched_at` (source had the column
  but no status); `vendor_id` is a plain bigint with no FK because the vendors
  module/table is owned by another port and doesn't exist yet — add the FK
  when it lands if desired. Stripped vendor-name lookup, evaluations/ratings,
  assignee-user listing (depended on task assignee join semantics), and file
  upload plumbing — attachments are **metadata only** (client registers
  file_name/type/size/storage_path; no upload endpoint, note this to FE).
- **FTOs:** kept `field_team_member_ids`/`pm_assignee_ids` (org_users ids),
  approval_status/form_submission_status, resolution fields, dispatched_at,
  `GET /pending`. Stripped submissions/merge/duplicate-detection, Outlook
  sync, checklists, notifications. Source never declared an FTO status enum
  in code; this port uses new/scheduled/dispatched/in_progress/completed/
  cancelled (only completed/cancelled were observable in source SQL).
- **Recurring:** faithful port of models, CalculateNextRun and validation.
  Deviations: assignees/collaborators stored as `bigint[]` instead of the
  source's accidental `bigint[][]`; user-enrichment of assignees in list/get
  responses dropped (raw ids returned; source used the users module);
  `RunSeries` now **materializes a task** from the template before advancing
  next_run_at (source only advanced timestamps; task creation happened
  elsewhere), and the poller sweeps all orgs via an indexed `ListDue` query.
- **units/locations:** modules don't exist here; `unit_id`/`location_id`
  kept as plain nullable bigints (no FK, no name joins) on tasks/FTOs/
  recurring for shape fidelity.
- Source test files (notification-focused) were dropped; they depend on
  stripped notification machinery.

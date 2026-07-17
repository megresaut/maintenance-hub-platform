-- Maintenance ticket lifecycle: task categories, tasks, work orders (vendor
-- tickets), field team orders (internal tickets), and recurring task series.
-- Ported from ra-avm maintenance modules; every table is org-scoped and all
-- accounting/billing surface (bills, QBO/Buildium, estimate tax, submissions)
-- is stripped.

-- ---------------------------------------------------------------------------
-- Task categories (free-text options for tasks.category, per org)
-- ---------------------------------------------------------------------------
CREATE TABLE task_categories (
    id                 bigserial PRIMARY KEY,
    org_id             bigint NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name               text NOT NULL,
    is_active          boolean NOT NULL DEFAULT true,
    created_by_user_id bigint REFERENCES org_users(id) ON DELETE SET NULL,
    created_at         timestamptz NOT NULL DEFAULT now()
);
-- case-insensitive uniqueness per org so "Repair" and "repair" can't both exist
CREATE UNIQUE INDEX task_categories_org_name_lower_idx ON task_categories (org_id, lower(name));

-- ---------------------------------------------------------------------------
-- Tasks (core maintenance ticket)
-- ---------------------------------------------------------------------------
CREATE TABLE tasks (
    id                    bigserial PRIMARY KEY,
    org_id                bigint NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    property_id           bigint NOT NULL REFERENCES properties(id) ON DELETE CASCADE,
    name                  text NOT NULL,
    description           text NOT NULL DEFAULT '',
    status                text NOT NULL DEFAULT 'open' CHECK (status = ANY (ARRAY[
                              'open', 'in_progress', 'pending_vendor', 'scheduled',
                              'completed', 'closed', 'cancelled', 'deferred'])),
    priority              text NOT NULL DEFAULT 'medium' CHECK (priority = ANY (ARRAY[
                              'low', 'medium', 'high', 'urgent'])),
    category              text NOT NULL DEFAULT '',
    due_date              timestamptz,
    date_requested        timestamptz NOT NULL DEFAULT now(),
    last_updated          timestamptz NOT NULL DEFAULT now(),
    requested_by          bigint REFERENCES org_users(id) ON DELETE SET NULL,
    assignees             bigint[] NOT NULL DEFAULT '{}',  -- org_users ids
    collaborators         bigint[] NOT NULL DEFAULT '{}',  -- org_users ids
    tags                  text[] NOT NULL DEFAULT '{}',
    is_visible_to_client  boolean NOT NULL DEFAULT false,
    is_editable_by_client boolean NOT NULL DEFAULT false,
    is_template           boolean NOT NULL DEFAULT false,
    -- units/locations modules were not ported; kept as plain ids for shape fidelity
    unit_id               bigint,
    location_id           bigint,
    closed_by             bigint,
    closed_at             timestamptz,
    reopened_at           timestamptz,
    deleted_at            timestamptz  -- soft delete
);
CREATE INDEX idx_tasks_org ON tasks(org_id);
CREATE INDEX idx_tasks_org_property ON tasks(org_id, property_id);
CREATE INDEX idx_tasks_org_status ON tasks(org_id, status);

-- ---------------------------------------------------------------------------
-- Work orders (vendor-facing tickets)
-- ---------------------------------------------------------------------------
CREATE TABLE work_orders (
    id                 bigserial PRIMARY KEY,
    org_id             bigint NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    task_id            bigint REFERENCES tasks(id) ON DELETE SET NULL,  -- nullable linkage
    property_id        bigint NOT NULL REFERENCES properties(id) ON DELETE CASCADE,
    vendor_id          bigint,  -- vendors table owned by another port; plain id, no FK
    name               text NOT NULL,
    work_description   text NOT NULL DEFAULT '',
    vendor_notes       text,
    internal_notes     text,
    status             text NOT NULL DEFAULT 'new' CHECK (status = ANY (ARRAY[
                           'new', 'sent', 'dispatched', 'scheduled', 'in_progress',
                           'completed', 'cancelled', 'deferred', 'closed'])),
    priority           text NOT NULL DEFAULT 'medium' CHECK (priority = ANY (ARRAY[
                           'low', 'medium', 'high', 'urgent'])),
    due_date           timestamptz,
    entry_preference   text,
    -- plain informational quote/estimate; all estimate parts/labor/tax stripped
    quote_amount_cents bigint,
    actual_cost        double precision,
    event_start_at     timestamptz,
    event_end_at       timestamptz,
    dispatched_at      timestamptz,
    completed_at       timestamptz,
    completed_by       bigint,
    created_by         bigint,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_work_orders_org ON work_orders(org_id);
CREATE INDEX idx_work_orders_org_task ON work_orders(org_id, task_id);
CREATE INDEX idx_work_orders_org_property ON work_orders(org_id, property_id);
CREATE INDEX idx_work_orders_org_vendor ON work_orders(org_id, vendor_id);

-- Attachment metadata only (no upload plumbing was ported; storage_path is an
-- opaque key registered by the client).
CREATE TABLE work_order_attachments (
    id               bigserial PRIMARY KEY,
    org_id           bigint NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    work_order_id    bigint NOT NULL REFERENCES work_orders(id) ON DELETE CASCADE,
    file_name        text NOT NULL,
    file_type        text NOT NULL DEFAULT '',
    file_size        bigint NOT NULL DEFAULT 0,
    storage_path     text NOT NULL DEFAULT '',
    uploaded_by_type text NOT NULL DEFAULT 'user',
    uploaded_by_id   bigint NOT NULL DEFAULT 0,
    label            text,
    description      text,
    created_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_wo_attachments_wo ON work_order_attachments(org_id, work_order_id);

CREATE TABLE work_order_notes (
    id                   bigserial PRIMARY KEY,
    org_id               bigint NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    work_order_id        bigint NOT NULL REFERENCES work_orders(id) ON DELETE CASCADE,
    author_type          text NOT NULL DEFAULT 'user',
    author_id            bigint NOT NULL DEFAULT 0,
    body                 text NOT NULL,
    is_visible_to_client boolean NOT NULL DEFAULT false,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz
);
CREATE INDEX idx_wo_notes_wo ON work_order_notes(org_id, work_order_id);

-- ---------------------------------------------------------------------------
-- Field team orders (internal/field-team tickets)
-- ---------------------------------------------------------------------------
CREATE TABLE field_team_orders (
    id                     bigserial PRIMARY KEY,
    org_id                 bigint NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name                   text NOT NULL,
    description            text,
    property_id            bigint REFERENCES properties(id) ON DELETE SET NULL,
    task_id                bigint REFERENCES tasks(id) ON DELETE SET NULL,
    field_team_member_ids  bigint[] NOT NULL DEFAULT '{}',  -- org_users ids
    pm_assignee_ids        bigint[] NOT NULL DEFAULT '{}',  -- org_users ids
    status                 text NOT NULL DEFAULT 'new' CHECK (status = ANY (ARRAY[
                               'new', 'scheduled', 'dispatched', 'in_progress',
                               'completed', 'cancelled'])),
    priority               text NOT NULL DEFAULT 'medium' CHECK (priority = ANY (ARRAY[
                               'low', 'medium', 'high', 'urgent'])),
    due_date               timestamptz,
    event_start_at         timestamptz,
    event_end_at           timestamptz,
    pm_notes               text,
    resolution_summary     text,
    issue_resolved         boolean,
    approval_status        text NOT NULL DEFAULT 'pending',
    form_submission_status text NOT NULL DEFAULT 'none',  -- none | partial | complete
    dispatched_at          timestamptz,
    completed_at           timestamptz,
    completed_by           bigint,
    unit_id                bigint,
    location_id            bigint,
    is_visible_to_client   boolean NOT NULL DEFAULT false,
    created_by             bigint,
    source                 text NOT NULL DEFAULT 'manual',
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz
);
CREATE INDEX idx_ftos_org ON field_team_orders(org_id);
CREATE INDEX idx_ftos_org_task ON field_team_orders(org_id, task_id);
CREATE INDEX idx_ftos_org_property ON field_team_orders(org_id, property_id);

-- ---------------------------------------------------------------------------
-- Recurring task series (templates materialized into tasks on schedule)
-- ---------------------------------------------------------------------------
CREATE TABLE recurring_task_series (
    id               bigserial PRIMARY KEY,
    org_id           bigint NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    property_id      bigint NOT NULL REFERENCES properties(id) ON DELETE CASCADE,
    unit_id          bigint,
    location_id      bigint,
    template_task_id bigint REFERENCES tasks(id) ON DELETE SET NULL,
    name             text NOT NULL DEFAULT '',
    description      text NOT NULL DEFAULT '',
    assignees        bigint[] NOT NULL DEFAULT '{}',
    collaborators    bigint[] NOT NULL DEFAULT '{}',
    priority         text,
    category         text,
    frequency        text NOT NULL CHECK (frequency = ANY (ARRAY[
                         'daily', 'weekly', 'monthly', 'yearly'])),
    interval         int NOT NULL DEFAULT 1,
    by_day           text[] NOT NULL DEFAULT '{}',
    day_of_month     int,
    week_of_month    int,
    weekday_of_month text,
    start_date       timestamptz NOT NULL,
    end_date         timestamptz,
    due_after_days   int NOT NULL DEFAULT 0,
    active           boolean NOT NULL DEFAULT true,
    next_run_at      timestamptz NOT NULL,
    last_run_at      timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_recurring_org ON recurring_task_series(org_id);
CREATE INDEX idx_recurring_next_run ON recurring_task_series(next_run_at) WHERE active;

-- Calendar module: per-org Outlook (Microsoft Graph) bidirectional sync.
-- Tables: org connection config, mirrored/local events, Graph webhook
-- subscriptions (subscription id -> org resolution), and per-org delta
-- sync checkpoints. All org-scoped.

-- One Outlook connection per org (client-credentials Graph auth).
CREATE TABLE org_calendar_connections (
    id            bigserial PRIMARY KEY,
    org_id        bigint NOT NULL UNIQUE REFERENCES organizations(id) ON DELETE CASCADE,
    tenant_id     text NOT NULL,
    client_id     text NOT NULL,
    client_secret text NOT NULL,
    mailbox_upn   text NOT NULL,
    calendar_id   text NOT NULL DEFAULT '',  -- optional specific/shared calendar; '' = mailbox default
    enabled       boolean NOT NULL DEFAULT true,
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- Org-scoped calendar events: mirrors of Outlook events (source = 'outlook')
-- and locally created events (source = 'local'). Outlook linkage and sync
-- bookkeeping are folded into the row (one Outlook event maps to at most one
-- local row per org).
CREATE TABLE calendar_events (
    id                  bigserial PRIMARY KEY,
    org_id              bigint NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    title               text NOT NULL,
    description         text NOT NULL DEFAULT '',
    location            text NOT NULL DEFAULT '',
    start_at            timestamptz NOT NULL,
    end_at              timestamptz NOT NULL,
    all_day             boolean NOT NULL DEFAULT false,
    show_as             text NOT NULL DEFAULT 'busy',      -- busy | free | oof | tentative | ...
    is_private          boolean NOT NULL DEFAULT false,
    organizer_name      text NOT NULL DEFAULT '',
    source              text NOT NULL DEFAULT 'local',     -- local | outlook
    outlook_event_id    text,                              -- Graph event id when linked
    outlook_change_key  text,                              -- last seen/written Graph changeKey (loop prevention)
    last_sync_direction text,                              -- outlook_to_local | local_to_outlook
    sync_status         text NOT NULL DEFAULT 'active',    -- active | cancelled | deleted
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_calendar_events_org_outlook
    ON calendar_events(org_id, outlook_event_id)
    WHERE outlook_event_id IS NOT NULL;
CREATE INDEX idx_calendar_events_org_start ON calendar_events(org_id, start_at);

-- Graph webhook subscriptions. graph_subscription_id -> org_id is how the
-- unauthenticated webhook receiver resolves which org a notification is for;
-- client_state is the shared secret validated on each notification.
CREATE TABLE calendar_subscriptions (
    id                    bigserial PRIMARY KEY,
    org_id                bigint NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    graph_subscription_id text NOT NULL UNIQUE,
    resource              text NOT NULL,
    expiration            timestamptz NOT NULL,
    client_state          text NOT NULL,
    active                boolean NOT NULL DEFAULT true,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_calendar_subscriptions_org ON calendar_subscriptions(org_id);

-- Per-org Graph delta-query checkpoints (deltaLink per resource).
CREATE TABLE calendar_sync_state (
    id              bigserial PRIMARY KEY,
    org_id          bigint NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    resource        text NOT NULL,
    delta_link      text NOT NULL,
    last_checked_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, resource)
);

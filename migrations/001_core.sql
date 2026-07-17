-- Core multi-tenancy tables: organizations, users, properties, and the
-- unified per-ticket activity feed.

CREATE TABLE organizations (
    id           bigserial PRIMARY KEY,
    name         text NOT NULL,
    -- Per-org Twilio number used for SMS intake and vendor outreach.
    -- Platform-level Twilio credentials live in env; each org gets its own number.
    twilio_phone_number text,
    -- From-address for outbound email outreach (SMTP creds are platform-level env).
    outreach_email_from text,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE org_users (
    id            bigserial PRIMARY KEY,
    org_id        bigint NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email         text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    name          text NOT NULL DEFAULT '',
    role          text NOT NULL DEFAULT 'pm',  -- admin | pm | field
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE properties (
    id         bigserial PRIMARY KEY,
    org_id     bigint NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name       text NOT NULL,
    address    text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_properties_org ON properties(org_id);

-- Unified activity feed. ticket_type: work_order | fto | task.
CREATE TABLE ticket_activity (
    id          bigserial PRIMARY KEY,
    org_id      bigint NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    ticket_type text NOT NULL,
    ticket_id   bigint NOT NULL,
    kind        text NOT NULL,
    actor_type  text NOT NULL DEFAULT 'system',
    actor_id    bigint,
    actor_name  text,
    body        text NOT NULL DEFAULT '',
    metadata    jsonb NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_ticket_activity_ticket ON ticket_activity(org_id, ticket_type, ticket_id);

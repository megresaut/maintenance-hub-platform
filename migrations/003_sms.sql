-- SMS intake tables (ported from ra-avm, org-scoped).

CREATE TABLE sms_conversations (
    id                 bigserial PRIMARY KEY,
    org_id             bigint NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    phone_number       text NOT NULL,
    twilio_sid         text,
    status             text NOT NULL DEFAULT 'active',
    is_group           boolean NOT NULL DEFAULT false,
    group_name         text,
    participants       text[],
    last_message_at    timestamptz,
    last_classified_at timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, phone_number)
);

CREATE TABLE sms_messages (
    id              bigserial PRIMARY KEY,
    conversation_id bigint NOT NULL REFERENCES sms_conversations(id) ON DELETE CASCADE,
    twilio_sid      text,
    direction       text NOT NULL,  -- inbound | outbound
    from_number     text NOT NULL,
    to_number       text NOT NULL,
    body            text NOT NULL DEFAULT '',
    media_urls      jsonb NOT NULL DEFAULT '[]',
    processed       boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_sms_messages_conversation ON sms_messages(conversation_id);
CREATE INDEX idx_sms_messages_unprocessed ON sms_messages(conversation_id) WHERE processed = false;

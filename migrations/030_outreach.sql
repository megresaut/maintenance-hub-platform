-- Vendor outreach (FLAGSHIP — build-new, no equivalent in ra-avm):
-- outreach requests sent to shortlisted vendors and their captured replies.

CREATE TABLE vendor_outreach_requests (
    id            bigserial PRIMARY KEY,
    org_id        bigint NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    work_order_id bigint NOT NULL REFERENCES work_orders(id) ON DELETE CASCADE,
    vendor_id     bigint NOT NULL REFERENCES vendors(id) ON DELETE CASCADE,
    channel       text NOT NULL,                -- sms | email
    to_address    text NOT NULL,                -- phone or email actually used
    message_body  text NOT NULL,
    provider_ref  text,                         -- twilio message SID etc.
    status        text NOT NULL DEFAULT 'pending', -- pending | sent | failed | replied | selected
    error         text,
    sent_at       timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_outreach_requests_wo ON vendor_outreach_requests(org_id, work_order_id);
CREATE INDEX idx_outreach_requests_vendor ON vendor_outreach_requests(org_id, vendor_id);

CREATE TABLE vendor_outreach_replies (
    id                  bigserial PRIMARY KEY,
    outreach_request_id bigint NOT NULL REFERENCES vendor_outreach_requests(id) ON DELETE CASCADE,
    body                text NOT NULL,
    media_urls          jsonb NOT NULL DEFAULT '[]',
    parsed_quote_cents  bigint,
    parsed_availability text,
    raw_source_ref      text,                   -- inbound twilio SID / email message-id
    received_at         timestamptz NOT NULL DEFAULT now(),
    created_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_outreach_replies_request ON vendor_outreach_replies(outreach_request_id);

-- Trade + photos on work orders: populated from AI-classified drafts so
-- outreach can shortlist by trade and reference photos. (Informational,
-- non-accounting columns.)
ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS category text NOT NULL DEFAULT '';
ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS media_urls jsonb NOT NULL DEFAULT '[]';

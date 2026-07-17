-- AI classification drafts review queue (ported from ra-avm ai_drafts,
-- org-scoped, minus the duplicate-detection column).

CREATE TABLE ai_drafts (
    id                bigserial PRIMARY KEY,
    org_id            bigint NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    source            text NOT NULL,               -- sms | calendar | manual
    source_ref        text,
    entity_type       text NOT NULL,               -- task | work_order | fto
    extracted_data    jsonb NOT NULL DEFAULT '{}',
    ai_confidence     double precision,
    ai_reasoning      text,
    ai_model          text,
    raw_ai_response   jsonb,
    status            text NOT NULL DEFAULT 'pending',  -- pending | approved | rejected | expired
    parent_draft_id   bigint REFERENCES ai_drafts(id) ON DELETE SET NULL,
    reviewed_by       bigint REFERENCES org_users(id),
    reviewed_at       timestamptz,
    created_entity_id bigint,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_ai_drafts_org_status ON ai_drafts(org_id, status);
CREATE INDEX idx_ai_drafts_parent ON ai_drafts(parent_draft_id);

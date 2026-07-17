-- Vendor directory. Column set deliberately limited per the plan: only the
-- contact/directory columns are ported from ra-avm's vendors table — no
-- accounting/compliance fields (no QBO/Buildium IDs, 1099/tax, insurance,
-- hourly rates).

CREATE TABLE vendors (
    id                bigserial PRIMARY KEY,
    org_id            bigint NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name              text NOT NULL,
    category          text NOT NULL DEFAULT '',          -- trade: plumbing/electrical/hvac/...
    service_area_tags text[] NOT NULL DEFAULT '{}',      -- e.g. {stamford, greenwich}
    primary_email     text,
    phone             text,
    alt_email         text,
    alt_phone         text,
    notes             text,
    website           text,
    address           text,
    created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_vendors_org ON vendors(org_id);
CREATE INDEX idx_vendors_org_category ON vendors(org_id, category);

-- Preferred vendor list per property + trade. Build-new: no equivalent
-- exists anywhere in ra-avm.
CREATE TABLE preferred_vendors (
    id          bigserial PRIMARY KEY,
    org_id      bigint NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    property_id bigint NOT NULL REFERENCES properties(id) ON DELETE CASCADE,
    category    text NOT NULL,
    vendor_id   bigint NOT NULL REFERENCES vendors(id) ON DELETE CASCADE,
    priority    int NOT NULL DEFAULT 1,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (org_id, property_id, category, vendor_id)
);
CREATE INDEX idx_preferred_vendors_lookup ON preferred_vendors(org_id, property_id, category);

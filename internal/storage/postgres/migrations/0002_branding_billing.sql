-- Panel-wide settings live in one row so they can be read in a single query and
-- edited without a migration every time a knob is added.
CREATE TABLE panel_settings (
    id                  BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
    brand_name          TEXT        NOT NULL DEFAULT 'AmneziaX',
    brand_tagline       TEXT        NOT NULL DEFAULT '',
    brand_logo          TEXT        NOT NULL DEFAULT '',
    brand_accent        TEXT        NOT NULL DEFAULT '',
    subscription_title  TEXT        NOT NULL DEFAULT '',
    support_url         TEXT        NOT NULL DEFAULT '',
    currency            TEXT        NOT NULL DEFAULT 'USD',
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO panel_settings (id) VALUES (TRUE) ON CONFLICT DO NOTHING;

-- Infrastructure billing: what a node costs and when it has to be paid for.
ALTER TABLE nodes
    ADD COLUMN provider          TEXT   NOT NULL DEFAULT '',
    ADD COLUMN provider_url      TEXT   NOT NULL DEFAULT '',
    ADD COLUMN cost_amount       NUMERIC(12, 2) NOT NULL DEFAULT 0,
    ADD COLUMN cost_currency     TEXT   NOT NULL DEFAULT '',
    ADD COLUMN billing_cycle     TEXT   NOT NULL DEFAULT 'NONE',
    ADD COLUMN next_payment_at   TIMESTAMPTZ,
    ADD COLUMN billing_notes     TEXT   NOT NULL DEFAULT '',
    ADD COLUMN tags              TEXT[] NOT NULL DEFAULT '{}';

CREATE INDEX nodes_next_payment_idx ON nodes (next_payment_at)
    WHERE next_payment_at IS NOT NULL;

-- Devices seen on a subscription, so hwid_device_limit can actually be enforced.
CREATE TABLE user_devices (
    user_uuid   UUID        NOT NULL REFERENCES users (uuid) ON DELETE CASCADE,
    hwid        TEXT        NOT NULL,
    user_agent  TEXT        NOT NULL DEFAULT '',
    platform    TEXT        NOT NULL DEFAULT '',
    first_seen  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_uuid, hwid)
);

CREATE INDEX user_devices_last_seen_idx ON user_devices (last_seen DESC);

-- Tokens for external integrations (bots, billing systems, provisioning).
CREATE TABLE api_tokens (
    uuid        UUID PRIMARY KEY,
    name        TEXT        NOT NULL,
    token_hash  TEXT        NOT NULL UNIQUE,
    token_preview TEXT      NOT NULL DEFAULT '',
    created_by  TEXT        NOT NULL DEFAULT '',
    last_used_at TIMESTAMPTZ,
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

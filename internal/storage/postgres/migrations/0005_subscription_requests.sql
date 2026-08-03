-- Every fetch of a subscription, not just the most recent one.
--
-- The users table already carried sub_last_opened_at and sub_last_user_agent,
-- which answers "did they ever open it" and nothing else. The questions that
-- actually come up — which client did they use, did it get the format we think,
-- is something pulling this link from an address the subscriber has never been
-- near — all need the history.
CREATE TABLE subscription_requests (
    id          BIGSERIAL   PRIMARY KEY,
    user_id     UUID        REFERENCES users (uuid) ON DELETE CASCADE,
    -- The token as presented. A request that resolved to nobody is the most
    -- interesting kind — a revoked link still being polled, or a guess — so it
    -- is recorded with a null user rather than dropped.
    token       TEXT        NOT NULL DEFAULT '',
    ip          TEXT        NOT NULL DEFAULT '',
    user_agent  TEXT        NOT NULL DEFAULT '',
    format      TEXT        NOT NULL DEFAULT '',
    status      INT         NOT NULL DEFAULT 200,
    hwid        TEXT        NOT NULL DEFAULT '',
    at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX subscription_requests_at_idx   ON subscription_requests (at DESC);
CREATE INDEX subscription_requests_user_idx ON subscription_requests (user_id, at DESC);

-- Rules that decide what a subscription request is answered with, matched on
-- the client's User-Agent.
--
-- The panel already had two ends of this: a built-in list of clients it
-- recognises, and one global default for everything else. Neither helps an
-- operator whose users run a client the panel has never heard of — they could
-- only change the default for everybody at once.
CREATE TABLE response_rules (
    uuid       UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT        NOT NULL DEFAULT '',
    -- Matched case-insensitively as a substring of the User-Agent. A substring
    -- rather than a regex on purpose: operators write these, and a bad regex
    -- fails at request time on a path that must not fail.
    match_ua   TEXT        NOT NULL,
    format     TEXT        NOT NULL,
    is_enabled BOOLEAN     NOT NULL DEFAULT TRUE,
    -- Lower runs first, so a narrow rule can be placed ahead of a broad one.
    priority   INT         NOT NULL DEFAULT 100,
    hits       BIGINT      NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX response_rules_order_idx ON response_rules (is_enabled, priority, created_at);

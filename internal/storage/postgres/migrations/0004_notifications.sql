-- Outbound notification channels. One table rather than a table per transport:
-- a channel is "somewhere an event gets delivered", and the transport-specific
-- parts (a URL and signing secret, a bot token and chat id) differ only in
-- shape, so they live in a JSONB payload the dispatcher understands.
CREATE TABLE notification_channels (
    uuid        UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL,
    kind        TEXT        NOT NULL,
    config      JSONB       NOT NULL DEFAULT '{}'::jsonb,
    -- Empty means every event. Storing the subscription on the channel keeps
    -- routing a property of the destination instead of a global switchboard.
    events      TEXT[]      NOT NULL DEFAULT '{}',
    is_enabled  BOOLEAN     NOT NULL DEFAULT TRUE,

    -- Last outcome, denormalised so the list can show health without joining
    -- the delivery log for every row.
    last_ok     BOOLEAN,
    last_detail TEXT        NOT NULL DEFAULT '',
    last_sent_at TIMESTAMPTZ,

    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX notification_channels_enabled_idx ON notification_channels (is_enabled);

-- A delivery log, because "the webhook did not arrive" is otherwise
-- unanswerable: nobody can tell a channel that was never tried from one that
-- was tried and refused.
CREATE TABLE notification_deliveries (
    id          BIGSERIAL   PRIMARY KEY,
    channel_id  UUID        NOT NULL REFERENCES notification_channels (uuid) ON DELETE CASCADE,
    event_kind  TEXT        NOT NULL,
    ok          BOOLEAN     NOT NULL,
    detail      TEXT        NOT NULL DEFAULT '',
    attempts    INT         NOT NULL DEFAULT 1,
    duration_ms INT         NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX notification_deliveries_channel_idx
    ON notification_deliveries (channel_id, created_at DESC);

-- Samples from node heartbeats. The nodes table only ever held the latest
-- reading, so an operator could see that a node is at 90% CPU but not whether
-- it has been there for an hour or for a second.
CREATE TABLE node_metrics (
    node_id      UUID        NOT NULL REFERENCES nodes (uuid) ON DELETE CASCADE,
    at           TIMESTAMPTZ NOT NULL,
    cpu_percent  DOUBLE PRECISION NOT NULL DEFAULT 0,
    used_ram_bytes  BIGINT   NOT NULL DEFAULT 0,
    total_ram_bytes BIGINT   NOT NULL DEFAULT 0,
    load_avg1    DOUBLE PRECISION NOT NULL DEFAULT 0,
    online_users INT         NOT NULL DEFAULT 0,
    PRIMARY KEY (node_id, at)
);

CREATE INDEX node_metrics_at_idx ON node_metrics (at DESC);

-- A notice shown to subscribers on their subscription page. Scheduling it means
-- an operator can queue maintenance windows ahead of time instead of
-- remembering to publish and then remove a message by hand.
CREATE TABLE announcements (
    uuid       UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    title      TEXT        NOT NULL DEFAULT '',
    body       TEXT        NOT NULL,
    level      TEXT        NOT NULL DEFAULT 'INFO',
    is_enabled BOOLEAN     NOT NULL DEFAULT TRUE,
    starts_at  TIMESTAMPTZ,
    ends_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

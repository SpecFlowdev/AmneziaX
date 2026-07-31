CREATE TABLE admins (
    uuid           UUID PRIMARY KEY,
    username       TEXT        NOT NULL UNIQUE,
    password_hash  TEXT        NOT NULL,
    role           TEXT        NOT NULL DEFAULT 'ADMIN',
    is_disabled    BOOLEAN     NOT NULL DEFAULT FALSE,
    last_login_at  TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE config_profiles (
    uuid       UUID PRIMARY KEY,
    name       TEXT        NOT NULL UNIQUE,
    config     JSONB       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE config_profile_inbounds (
    uuid               UUID PRIMARY KEY,
    config_profile_uuid UUID NOT NULL REFERENCES config_profiles (uuid) ON DELETE CASCADE,
    tag                TEXT NOT NULL,
    type               TEXT NOT NULL DEFAULT '',
    network            TEXT NOT NULL DEFAULT '',
    security           TEXT NOT NULL DEFAULT '',
    port               INTEGER NOT NULL DEFAULT 0,
    UNIQUE (config_profile_uuid, tag)
);

CREATE TABLE nodes (
    uuid                  UUID PRIMARY KEY,
    name                  TEXT        NOT NULL UNIQUE,
    address               TEXT        NOT NULL DEFAULT '',
    country_code          TEXT        NOT NULL DEFAULT 'XX',
    description           TEXT        NOT NULL DEFAULT '',
    token_hash            TEXT        NOT NULL,
    token_preview         TEXT        NOT NULL DEFAULT '',
    is_disabled           BOOLEAN     NOT NULL DEFAULT FALSE,
    health                TEXT        NOT NULL DEFAULT 'UNKNOWN',
    config_profile_uuid   UUID REFERENCES config_profiles (uuid) ON DELETE SET NULL,
    active_inbound_tags   TEXT[]      NOT NULL DEFAULT '{}',
    consumption_multiplier DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    traffic_limit_bytes   BIGINT      NOT NULL DEFAULT 0,
    traffic_used_bytes    BIGINT      NOT NULL DEFAULT 0,
    traffic_reset_strategy TEXT       NOT NULL DEFAULT 'NO_RESET',
    notify_percent        INTEGER     NOT NULL DEFAULT 0,
    last_traffic_reset_at TIMESTAMPTZ,
    agent_version         TEXT        NOT NULL DEFAULT '',
    xray_version          TEXT        NOT NULL DEFAULT '',
    xray_running          BOOLEAN     NOT NULL DEFAULT FALSE,
    xray_started_at       TIMESTAMPTZ,
    config_hash           TEXT        NOT NULL DEFAULT '',
    hostname              TEXT        NOT NULL DEFAULT '',
    os                    TEXT        NOT NULL DEFAULT '',
    arch                  TEXT        NOT NULL DEFAULT '',
    kernel                TEXT        NOT NULL DEFAULT '',
    cpu_count             INTEGER     NOT NULL DEFAULT 0,
    cpu_model             TEXT        NOT NULL DEFAULT '',
    cpu_usage             DOUBLE PRECISION NOT NULL DEFAULT 0,
    total_ram_bytes       BIGINT      NOT NULL DEFAULT 0,
    used_ram_bytes        BIGINT      NOT NULL DEFAULT 0,
    load_avg_1            DOUBLE PRECISION NOT NULL DEFAULT 0,
    online_users          INTEGER     NOT NULL DEFAULT 0,
    status_message        TEXT        NOT NULL DEFAULT '',
    last_status_at        TIMESTAMPTZ,
    last_connected_at     TIMESTAMPTZ,
    view_position         INTEGER     NOT NULL DEFAULT 0,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE hosts (
    uuid           UUID PRIMARY KEY,
    inbound_uuid   UUID        NOT NULL REFERENCES config_profile_inbounds (uuid) ON DELETE CASCADE,
    remark         TEXT        NOT NULL DEFAULT '',
    address        TEXT        NOT NULL DEFAULT '',
    port           INTEGER     NOT NULL DEFAULT 443,
    path           TEXT        NOT NULL DEFAULT '',
    sni            TEXT        NOT NULL DEFAULT '',
    host_header    TEXT        NOT NULL DEFAULT '',
    alpn           TEXT        NOT NULL DEFAULT '',
    fingerprint    TEXT        NOT NULL DEFAULT '',
    public_key     TEXT        NOT NULL DEFAULT '',
    short_id       TEXT        NOT NULL DEFAULT '',
    spider_x       TEXT        NOT NULL DEFAULT '',
    flow           TEXT        NOT NULL DEFAULT '',
    security       TEXT        NOT NULL DEFAULT '',
    allow_insecure BOOLEAN     NOT NULL DEFAULT FALSE,
    tags           TEXT[]      NOT NULL DEFAULT '{}',
    is_disabled    BOOLEAN     NOT NULL DEFAULT FALSE,
    view_position  INTEGER     NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX hosts_inbound_idx ON hosts (inbound_uuid);

CREATE TABLE squads (
    uuid       UUID PRIMARY KEY,
    name       TEXT        NOT NULL UNIQUE,
    info       TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE squad_inbounds (
    squad_uuid   UUID NOT NULL REFERENCES squads (uuid) ON DELETE CASCADE,
    inbound_uuid UUID NOT NULL REFERENCES config_profile_inbounds (uuid) ON DELETE CASCADE,
    PRIMARY KEY (squad_uuid, inbound_uuid)
);

CREATE TABLE users (
    uuid                    UUID PRIMARY KEY,
    short_uuid              TEXT        NOT NULL UNIQUE,
    username                TEXT        NOT NULL UNIQUE,
    subscription_uuid       UUID        NOT NULL UNIQUE,
    vless_uuid              UUID        NOT NULL,
    trojan_password         TEXT        NOT NULL DEFAULT '',
    ss_password             TEXT        NOT NULL DEFAULT '',
    status                  TEXT        NOT NULL DEFAULT 'ACTIVE',
    traffic_limit_bytes     BIGINT      NOT NULL DEFAULT 0,
    used_traffic_bytes      BIGINT      NOT NULL DEFAULT 0,
    lifetime_traffic_bytes  BIGINT      NOT NULL DEFAULT 0,
    traffic_reset_strategy  TEXT        NOT NULL DEFAULT 'NO_RESET',
    last_traffic_reset_at   TIMESTAMPTZ,
    expire_at               TIMESTAMPTZ,
    online_at               TIMESTAMPTZ,
    description             TEXT        NOT NULL DEFAULT '',
    tag                     TEXT        NOT NULL DEFAULT '',
    email                   TEXT        NOT NULL DEFAULT '',
    telegram_id             BIGINT,
    hwid_device_limit       INTEGER     NOT NULL DEFAULT 0,
    sub_last_user_agent     TEXT        NOT NULL DEFAULT '',
    sub_last_opened_at      TIMESTAMPTZ,
    sub_revoked_at          TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX users_status_idx ON users (status);
CREATE INDEX users_expire_idx ON users (expire_at);

CREATE TABLE user_squads (
    user_uuid  UUID NOT NULL REFERENCES users (uuid) ON DELETE CASCADE,
    squad_uuid UUID NOT NULL REFERENCES squads (uuid) ON DELETE CASCADE,
    PRIMARY KEY (user_uuid, squad_uuid)
);

CREATE TABLE node_usage (
    node_uuid UUID        NOT NULL REFERENCES nodes (uuid) ON DELETE CASCADE,
    bucket    TIMESTAMPTZ NOT NULL,
    bytes     BIGINT      NOT NULL DEFAULT 0,
    PRIMARY KEY (node_uuid, bucket)
);

CREATE TABLE user_usage (
    user_uuid UUID        NOT NULL REFERENCES users (uuid) ON DELETE CASCADE,
    node_uuid UUID        NOT NULL REFERENCES nodes (uuid) ON DELETE CASCADE,
    bucket    TIMESTAMPTZ NOT NULL,
    bytes     BIGINT      NOT NULL DEFAULT 0,
    PRIMARY KEY (user_uuid, node_uuid, bucket)
);

CREATE INDEX user_usage_bucket_idx ON user_usage (bucket);

CREATE TABLE events (
    id         BIGSERIAL PRIMARY KEY,
    kind       TEXT        NOT NULL,
    actor      TEXT        NOT NULL DEFAULT '',
    subject    TEXT        NOT NULL DEFAULT '',
    message    TEXT        NOT NULL DEFAULT '',
    meta       JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX events_created_idx ON events (created_at DESC);

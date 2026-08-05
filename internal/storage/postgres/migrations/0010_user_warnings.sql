-- Warn before a subscriber is cut off rather than after.
--
-- Both markers record which cycle a warning was already sent for, so the
-- maintenance loop — which runs every minute — emits each warning once instead
-- of once a minute until the thing it warns about happens.
ALTER TABLE users
    -- The expiry the warning was sent about. A changed expiry date is a
    -- different deadline and earns a fresh warning.
    ADD COLUMN expiry_warned_for   timestamptz,
    -- The used-traffic figure the quota warning was sent at. A traffic reset
    -- moves usage backwards, which clears the warning by comparison.
    ADD COLUMN quota_warned_at     bigint NOT NULL DEFAULT 0;

-- Thresholds live in settings so an operator can tune them without a redeploy.
-- Zero disables either warning, which is the behaviour every existing install
-- had before this migration.
ALTER TABLE panel_settings
    ADD COLUMN warn_expiry_days    INTEGER NOT NULL DEFAULT 3,
    ADD COLUMN warn_quota_percent  INTEGER NOT NULL DEFAULT 90;

-- Two-factor authentication for administrators.
--
-- The secret is stored in the clear on purpose: TOTP verification needs the
-- original key, so there is no hash that would still let the panel check a
-- code. It is exactly as sensitive as the rest of this database, which already
-- holds every node token and subscription link.
ALTER TABLE admins
    ADD COLUMN totp_secret          text,
    ADD COLUMN totp_enabled         boolean     NOT NULL DEFAULT false,
    ADD COLUMN totp_confirmed_at    timestamptz,
    -- The last accepted time step, so a code cannot be replayed inside the
    -- thirty seconds it stays valid.
    ADD COLUMN totp_last_step       bigint      NOT NULL DEFAULT 0,
    -- Digests only; the codes themselves are shown once and never again.
    ADD COLUMN recovery_code_hashes text[]      NOT NULL DEFAULT '{}';

-- When set, an administrator without two-factor cannot sign in — they are sent
-- to enrolment instead. Kept in settings rather than per-account so an owner
-- can raise the bar for everyone at once.
ALTER TABLE panel_settings
    ADD COLUMN require_totp boolean NOT NULL DEFAULT false;

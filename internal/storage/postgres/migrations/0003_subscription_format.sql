-- The format a subscription is served in when the client does not identify
-- itself. Empty means keep the built-in fallback (base64), which every client
-- understands; an operator whose users are all on JSON-capable apps can pin
-- something richer.
ALTER TABLE panel_settings
    ADD COLUMN IF NOT EXISTS subscription_format TEXT NOT NULL DEFAULT '';

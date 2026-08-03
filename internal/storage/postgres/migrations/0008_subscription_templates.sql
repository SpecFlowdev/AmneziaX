-- Operator-supplied Clash and sing-box documents.
--
-- The panel renders both formats from a fixed template baked into the binary.
-- That is right for most deployments and wrong for any operator who needs
-- their own rule set, DNS or proxy groups — and they had no way to change it
-- short of forking. An empty column keeps the built-in document, so this is
-- invisible until someone opts in.
ALTER TABLE panel_settings
    ADD COLUMN IF NOT EXISTS clash_template   TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS singbox_template TEXT NOT NULL DEFAULT '';

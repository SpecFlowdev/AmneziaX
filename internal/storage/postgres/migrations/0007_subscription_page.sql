-- What the subscriber's own page shows.
--
-- The page was fixed: QR, link, format buttons and the raw connection links,
-- for everyone. Handing a non-technical subscriber a wall of vless:// strings
-- invites them to paste the wrong one, and an operator who supports such users
-- has no way to hide it.
ALTER TABLE panel_settings
    ADD COLUMN IF NOT EXISTS sub_page_show_links   BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS sub_page_show_formats BOOLEAN NOT NULL DEFAULT TRUE;

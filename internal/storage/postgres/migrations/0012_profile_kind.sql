-- Which engine a profile's document is for.
--
-- Everything that exists today is an xray document, which is why the default
-- makes every existing row correct without a data migration. A profile of a
-- different kind is validated by that engine's rules and rendered by its
-- renderer — the two share the fact that a profile is one JSON document and
-- nothing else.
ALTER TABLE config_profiles
    ADD COLUMN kind text NOT NULL DEFAULT 'xray';

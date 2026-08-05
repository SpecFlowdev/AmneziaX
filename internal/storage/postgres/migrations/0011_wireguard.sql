-- WireGuard identifies a peer by a Curve25519 key pair, so every subscriber
-- needs one of their own — the shared password model the other protocols use
-- has no equivalent here.
--
-- The private key is stored because the panel is what hands the subscriber
-- their .conf; the server side only ever needs the public half. That is the
-- same trade already made for the trojan and shadowsocks passwords sitting in
-- this table.
ALTER TABLE users
    ADD COLUMN wg_private_key text NOT NULL DEFAULT '',
    ADD COLUMN wg_public_key  text NOT NULL DEFAULT '';

-- A stable per-user number, which is what the address inside the tunnel is
-- derived from. BIGSERIAL numbers the rows that already exist and keeps
-- allocating for new ones, so an address never moves under a subscriber who
-- already imported their config.
ALTER TABLE users
    ADD COLUMN wg_index BIGSERIAL;

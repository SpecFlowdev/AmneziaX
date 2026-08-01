# Security policy

## Reporting a vulnerability

Please report privately through
[GitHub Security Advisories](https://github.com/SpecFlowdev/AmneziaX/security/advisories/new)
rather than opening a public issue. A panel like this holds every credential of
every subscriber, so a public report puts other people's deployments at risk
before a fix exists.

Include what you need to demonstrate the problem: the version, the endpoint or
component, and the steps. A proof of concept helps, but a clear description is
enough to start.

## Supported versions

Fixes go onto the latest release. There is no long-term support branch yet.

## What the panel protects, and how

| | |
|---|---|
| Admin passwords | bcrypt |
| Admin sessions | JWT signed with `JWT_SECRET` |
| Node enrolment tokens | SHA-256 digest, compared in constant time, shown once |
| API tokens | same treatment as node tokens |
| Subscription links | unguessable UUID — **this is the subscriber's only credential** |
| Everything in transit | TLS terminated by Caddy, including the node control stream |

The panel binds no host port of its own. Caddy is the only process listening
publicly: 80 and 443 for the web UI, and the node port (9999 by default), which
answers nothing except the gRPC control service.

## Running it safely

- **Back up `/opt/amneziax/.env` and your Postgres.** They hold every secret.
- **Treat subscription URLs as passwords.** Anyone with the link has the access.
  Use *Revoke & reissue* to roll a user's credentials.
- **Rotate a node's token** if the server is ever compromised; the old one stops
  working immediately.
- **Do not expose the panel over plain HTTP.** The installer will not set that
  up, and neither should you.
- **Keep the node port closed to everything but your nodes** if you can — it is
  not a secret, but it does not need to be internet-wide.

## Out of scope

Findings that require an attacker to already be root on the panel server, or to
already hold an owner account, are not vulnerabilities in this project.

# Deploying Relay (maintenance-hub) — co-hosted on the Metered OVH box

Relay runs as a **self-contained stack** (its own Postgres + Go API + internal
Caddy serving the SPA) on the **same** OVH VPS as Metered. Metered's Caddy is the
only thing bound to :80/:443; it terminates TLS for `relay.rentatrix.com` and
reverse-proxies the hostname to Relay's `relay-web` over a shared `edge` network.

```
internet ─HTTPS─▶ Metered Caddy (:443) ──edge──▶ relay-web:80 ──relaynet──▶ relay-app:8091
                                                    └ serves web/dist          └ Postgres (relay-db)
```

Relay has **no RLS** and **no prod-config gate**, so a normal DB role +
`sslmode=disable` on the in-host hop is fine (simpler than Metered).

## Prereqs
- The Metered stack already deployed on the box (provides the edge Caddy + TLS).
- DNS: `relay.rentatrix.com` A-record → the box IP (`15.204.81.222`).

## Deploy
```bash
# on the box, from this repo synced to ~/relay
cd ~/relay/deploy/ovh
cp .env.production.example .env.production   # fill DOMAIN + openssl secrets + OpenRouter key
sudo docker network create edge 2>/dev/null || true
sudo ./deploy.sh          # builds frontend + images, starts relay-db/app/web
```

## Wire the edge Caddy (once)
In the **Metered** repo's `deploy/ovh/`:
- `docker-compose.yml` — Caddy joins the external `edge` network.
- `Caddyfile` — adds a `relay.rentatrix.com { reverse_proxy relay-web:80 }` block.

Then recreate the Metered Caddy so it picks up the new route + issues the cert:
```bash
cd ~/metered/deploy/ovh && sudo docker compose --env-file .env.production up -d
```
(Do this **after** DNS resolves, so ACME succeeds on the first try.)

## First org/user
```bash
docker compose --env-file .env.production exec relay-app \
  /app/provision -org "Acme PM" -email you@acme.com -password <temp> -role admin
```

## Notes
- **Outreach stays simulated** (`OUTREACH_SIMULATE=1`) until real Twilio/SMTP
  creds are added to `.env.production` and the flag flipped to `0`.
- The Dockerfile builds both `server` and `provision`.
- Backups: add Relay's DB to the box's backup routine (it currently backs up
  Metered's DB only) — see issue tracker.

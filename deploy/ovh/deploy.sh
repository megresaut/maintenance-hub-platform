#!/usr/bin/env bash
# Build + (re)deploy Relay's co-hosted stack on the shared OVH box. Run after
# filling .env.production. The Metered edge Caddy must already be wired to proxy
# relay.rentatrix.com → relay-web (see README "Wire the edge Caddy").
set -euo pipefail
cd "$(dirname "$0")"

[ -f .env.production ] || { echo "missing .env.production (copy .env.production.example)"; exit 1; }
set -a; . ./.env.production; set +a

echo "==> Building the frontend (relative /api → same-origin behind Caddy)"
( cd ../../web && npm ci && VITE_API_URL= VITE_DEMO= npm run build )

echo "==> Ensuring shared edge network exists"
docker network create edge 2>/dev/null || true

echo "==> Building images + starting Relay"
docker compose --env-file .env.production up -d --build

echo "==> Waiting for API health"
for i in $(seq 1 40); do
  if docker compose --env-file .env.production exec -T relay-app wget -qO- http://localhost:8091/healthz >/dev/null 2>&1; then
    echo "    Relay API healthy."; break
  fi
  sleep 2
done
docker compose --env-file .env.production ps
echo "==> Relay up. Create first org/user:"
echo "    docker compose --env-file .env.production exec relay-app /app/provision -org \"Acme PM\" -email you@acme.com -password <temp> -role admin"

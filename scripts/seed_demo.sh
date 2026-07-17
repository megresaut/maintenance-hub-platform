#!/usr/bin/env bash
# Seeds the demo org with properties, vendors, and a preferred-vendor list.
# Run with the API up on :8091. Idempotent-ish (re-running duplicates vendors).
set -euo pipefail
API=${API:-http://localhost:8091}
EMAIL=${EMAIL:-demo@maintenancehub.test}
PASSWORD=${PASSWORD:-demo1234}

TOKEN=$(curl -sf -X POST $API/api/auth/login -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}" | python3 -c 'import json,sys;print(json.load(sys.stdin)["token"])')
auth=(-H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json")

j() { python3 -c 'import json,sys;print(json.load(sys.stdin)["id"])'; }

P1=$(curl -sf "${auth[@]}" -X POST $API/api/properties -d '{"name":"Maple Court Apartments","address":"123 Main Street, Stamford, CT 06901"}' | j)
P2=$(curl -sf "${auth[@]}" -X POST $API/api/properties -d '{"name":"Oak Ridge Townhomes","address":"544 Oak Ridge Road, Greenwich, CT 06830"}' | j)

V1=$(curl -sf "${auth[@]}" -X POST $API/api/vendors -d '{"name":"Dolce Plumbing","category":"plumbing","service_area_tags":["stamford","greenwich"],"phone":"+12035550111","primary_email":"dispatch@dolceplumbing.test"}' | j)
V2=$(curl -sf "${auth[@]}" -X POST $API/api/vendors -d '{"name":"Harbor Point Plumbing & Heating","category":"plumbing","service_area_tags":["stamford"],"phone":"+12035550122","primary_email":"office@harborpointph.test"}' | j)
V3=$(curl -sf "${auth[@]}" -X POST $API/api/vendors -d '{"name":"Coastal HVAC Services","category":"hvac","service_area_tags":["stamford","norwalk"],"phone":"+12035550133","primary_email":"service@coastalhvac.test"}' | j)
V4=$(curl -sf "${auth[@]}" -X POST $API/api/vendors -d '{"name":"Nutmeg Electric","category":"electrical","service_area_tags":["greenwich"],"phone":"+12035550144","primary_email":"jobs@nutmegelectric.test"}' | j)

# Preferred list ONLY for Maple Court + plumbing (proves the preferred path);
# Oak Ridge deliberately has none (proves the local-lookup fallback).
curl -sf "${auth[@]}" -X POST $API/api/preferred-vendors -d "{\"property_id\":$P1,\"category\":\"plumbing\",\"vendor_id\":$V1,\"priority\":1}" > /dev/null
curl -sf "${auth[@]}" -X POST $API/api/preferred-vendors -d "{\"property_id\":$P1,\"category\":\"plumbing\",\"vendor_id\":$V2,\"priority\":2}" > /dev/null

echo "seeded: properties P1=$P1 P2=$P2; vendors $V1,$V2,$V3,$V4; preferred plumbing list on P1"

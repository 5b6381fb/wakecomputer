#!/bin/sh
set -e
cd /opt/wakecomputer
grep -v '^WC_PUBLIC_URL=' .env > .env.tmp || true
URL="${1:?usage: set-public-url.sh https://wake.example.com}"
echo "WC_PUBLIC_URL=$URL" >> .env.tmp
mv .env.tmp .env
grep '^WC_PUBLIC_URL=' .env
docker compose up -d 2>&1 | tail -5
sleep 2
docker compose exec -T wakecomputer /app/wakecomputer healthcheck && echo HEALTH_OK

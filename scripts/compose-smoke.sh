#!/usr/bin/env bash
set -euo pipefail

project="chatops-deploy-smoke"
cleanup() {
  docker compose -p "$project" logs app || true
  docker compose -p "$project" down --volumes
}
trap cleanup EXIT

docker compose -p "$project" up --build -d
for attempt in $(seq 1 90); do
  if curl --fail --silent http://localhost:8080/readyz >/dev/null; then
    break
  fi
  if [ "$attempt" -eq 90 ]; then
    echo "application did not become ready" >&2
    exit 1
  fi
  sleep 2
done
curl --fail --silent http://localhost:8080/healthz >/dev/null
curl --fail --silent http://localhost:8080/ >/dev/null
curl --fail --silent http://localhost:8080/auth/capabilities | grep -q '"dev_login":true'

#!/usr/bin/env bash
set -euo pipefail

CONTAINER=lazymongo-test-mongo
docker run --rm -d --name "$CONTAINER" -p 27018:27017 mongo:7 >/dev/null

cleanup() {
  docker stop "$CONTAINER" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "Esperando a que MongoDB esté listo..."
for _ in $(seq 1 30); do
  if docker exec "$CONTAINER" mongosh --quiet --eval 'db.runCommand({ping:1})' >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

LAZYMONGO_TEST_URI="mongodb://localhost:27018" go test -tags=integration ./... -v

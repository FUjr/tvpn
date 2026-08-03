#!/bin/sh
set -eu
test -z "$(gofmt -l cmd internal sdk/go)"
node --check internal/proxy/runtime.js
(cd web && npm run lint && npm test && npm run build)
(cd sdk/typescript && npm ci --ignore-scripts && npm test)
python3 -m compileall -q sdk/python/tvpn
PYTHONPATH=sdk/python python3 -m unittest discover -s sdk/python/tests
test -z "$(gofmt -l cmd internal web sdk/go)"
go test ./cmd/... ./internal/... ./scripts/... ./sdk/go/...
docker compose config --quiet
docker compose --env-file .env.example config --quiet
docker build -t tvpn:ci .

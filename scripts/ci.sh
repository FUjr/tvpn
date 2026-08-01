#!/bin/sh
set -eu
test -z "$(gofmt -l cmd internal)"
node --check internal/proxy/runtime.js
(cd web && npm run lint && npm test && npm run build)
test -z "$(gofmt -l cmd internal web)"
go test ./cmd/... ./internal/... ./scripts/...
docker build -t tvpn:ci .

#!/bin/sh
set -eu
test -z "$(gofmt -l cmd internal)"
go test ./cmd/... ./internal/...
(cd web && npm run lint && npm test && npm run build)
docker build -t tvpn:ci .

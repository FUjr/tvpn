#!/bin/sh
set -eu
go test ./cmd/... ./internal/... ./scripts/...
node --check internal/proxy/runtime.js
(cd web && npm test && npm run build)

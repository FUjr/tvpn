#!/bin/sh
set -eu
node --check internal/proxy/runtime.js
(cd web && npm test && npm run build)
go test ./cmd/... ./internal/... ./scripts/...

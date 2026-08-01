#!/bin/sh
set -eu
go test ./cmd/... ./internal/...
(cd web && npm test && npm run build)

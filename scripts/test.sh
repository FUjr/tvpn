#!/bin/sh
set -eu
node --check internal/proxy/runtime.js
(cd web && npm test && npm run build)
(cd sdk/typescript && npm ci --ignore-scripts && npm test)
python3 -m compileall -q sdk/python/tvpn
PYTHONPATH=sdk/python python3 -m unittest discover -s sdk/python/tests
go test ./cmd/... ./internal/... ./scripts/... ./sdk/go/...

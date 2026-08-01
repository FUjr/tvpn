#!/bin/sh
set -eu
./scripts/init-secrets.sh
TVPN_CONTAINER_UID="$(id -u)" TVPN_CONTAINER_GID="$(id -g)" docker compose up --build

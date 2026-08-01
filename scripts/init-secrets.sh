#!/bin/sh
set -eu

mkdir -p secrets
if [ ! -f secrets/tvpn_master_key ]; then
  umask 077
  openssl rand 32 > secrets/tvpn_master_key
fi

if [ ! -f secrets/tvpn_bootstrap_admin_password ]; then
  umask 077
  : > secrets/tvpn_bootstrap_admin_password
fi

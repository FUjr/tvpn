#!/bin/sh
set -eu

build_flag=--no-build
case "${1:-}" in
  "") ;;
  --build)
    build_flag=--build
    shift
    ;;
  -h|--help)
    echo "用法: $0 [--build]"
    echo "默认使用现有镜像；本地首次启动可用 --build 构建应用镜像。"
    exit 0
    ;;
  *)
    echo "用法: $0 [--build]" >&2
    exit 2
    ;;
esac
if [ "$#" -ne 0 ]; then
  echo "用法: $0 [--build]" >&2
  exit 2
fi

./scripts/init-secrets.sh
secret=secrets/tvpn_bootstrap_admin_password
export TVPN_CONTAINER_UID="$(id -u)"
export TVPN_CONTAINER_GID="$(id -g)"
cleanup() {
  stty echo 2>/dev/null || true
  : > "$secret"
}
trap cleanup EXIT HUP INT TERM

printf '管理员用户名 [admin]: '
IFS= read -r username
username=${username:-admin}
printf '管理员密码（至少 12 个字符）: '
stty -echo
IFS= read -r password
stty echo
printf '\n再次输入密码: '
stty -echo
IFS= read -r confirmation
stty echo
printf '\n'

if [ "$password" != "$confirmation" ]; then
  echo '两次输入的密码不一致' >&2
  exit 1
fi
if [ "${#password}" -lt 12 ]; then
  echo '密码至少需要 12 个字符' >&2
  exit 1
fi

umask 077
printf '%s' "$password" > "$secret"
unset password confirmation

TVPN_BOOTSTRAP_ADMIN_USERNAME="$username" docker compose up -d "$build_flag" --force-recreate app

attempt=0
until docker compose exec -T postgres psql -U tvpn -d tvpn -Atc "SELECT normalized_username FROM users WHERE is_admin=true" | grep -Fxiqx -- "$username"; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 20 ]; then
    echo '管理员初始化失败，请检查 docker compose logs app' >&2
    exit 1
  fi
  sleep 1
done
echo "管理员 $username 已就绪"

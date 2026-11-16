#!/bin/sh
set -eu
umask 077

if [ -n "${FB_REDIS_PASSWORD_FILE:-}" ]; then
  redis_password=$(cat "$FB_REDIS_PASSWORD_FILE")
  case "$redis_password" in ''|*[!0-9a-fA-F]*) echo 'redis_password must be a random hex secret' >&2; exit 1;; esac
  [ "${#redis_password}" -ge 32 ] || { echo 'redis_password must contain at least 32 hex characters' >&2; exit 1; }
  export FB_REDIS_CACHE_URL="redis://filebrowser:$redis_password@redis:6379"
fi

if [ ! -f /config/settings.json ]; then
  printf '%s\n' '{"address":"0.0.0.0","port":8080,"database":"/database/filebrowser.db","root":"/srv","disableExec":true}' > /config/settings.json
fi
if [ ! -f /database/filebrowser.db ]; then
  password_hash=$(cat /run/secrets/filebrowser_password_hash)
  [ -n "$password_hash" ] || { echo 'initial password hash is required' >&2; exit 1; }
  set -- --password "$password_hash" "$@"
fi
exec /usr/local/bin/filebrowser --config /config/settings.json "$@"

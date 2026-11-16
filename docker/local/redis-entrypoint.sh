#!/bin/sh
set -eu
umask 077
password=$(cat /run/secrets/redis_password)
case "$password" in ''|*[!0-9a-fA-F]*) echo 'redis_password must be a random hex secret' >&2; exit 1;; esac
[ "${#password}" -ge 32 ] || { echo 'redis_password must contain at least 32 hex characters' >&2; exit 1; }
hash=$(printf '%s' "$password" | sha256sum)
hash=${hash%% *}
printf '%s\n' 'user default off' "user filebrowser on #$hash ~filebrowser:upload:v2:* +ping +get +set +del +pexpire +eval +hello +client|setinfo" > /tmp/users.acl
exec redis-server --bind 0.0.0.0 --protected-mode yes --aclfile /tmp/users.acl --appendonly yes --dir /data --maxmemory 128mb --maxmemory-policy noeviction

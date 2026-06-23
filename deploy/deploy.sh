#!/usr/bin/env bash
# Deploy ShortLink to the shared VPS as an isolated compose project.
# Safe by design: separate dir (/opt/shortlink), separate compose project,
# no published ports, joins the game's network read-only-style (external).
# Does NOT touch /opt/mgho or any game container.
set -euo pipefail

VPS="${VPS:-mth-game}"          # ssh alias
REMOTE="/opt/shortlink"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

echo "==> packaging source from $REPO_ROOT"
# COPYFILE_DISABLE=1: stop macOS from leaking ._* AppleDouble files into the tar.
COPYFILE_DISABLE=1 tar czf /tmp/shortlink.tgz \
  --exclude='._*' --exclude='.git' --exclude='*.db' --exclude='deploy/.env' \
  -C "$REPO_ROOT" .

echo "==> ensuring $REMOTE exists on $VPS"
ssh "$VPS" "mkdir -p $REMOTE"

echo "==> uploading"
scp /tmp/shortlink.tgz "$VPS:$REMOTE/"
rm -f /tmp/shortlink.tgz

echo "==> building & starting (compose project: shortlink)"
ssh "$VPS" "cd $REMOTE \
  && tar xzf shortlink.tgz && rm -f shortlink.tgz \
  && find . -name '._*' -delete \
  && test -f deploy/.env || { echo 'MISSING deploy/.env on VPS — create it first'; exit 1; } \
  && docker compose -p shortlink -f deploy/docker-compose.yml build \
  && docker compose -p shortlink -f deploy/docker-compose.yml up -d \
  && docker builder prune -f --filter 'until=24h' || true"

echo "==> done. Container 'shortlink' is up on network deploy_default (no host port)."

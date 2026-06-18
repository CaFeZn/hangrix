#!/usr/bin/env bash
set -euo pipefail

project="${HANGRIX_DEPLOY_PROJECT:-hangrix-main}"
service="${HANGRIX_DEPLOY_SERVICE:-hangrix}"
runtime_app="${HANGRIX_DEPLOY_RUNTIME_APP:-/run/desktop/mnt/host/d/Codes/hangrix-main-docker/hangrix-main/.runtime/app}"
override_file="$(mktemp -t hangrix-local-deploy.XXXXXX.yml)"

cleanup() {
  rm -f "$override_file"
}
trap cleanup EXIT

if ! command -v docker >/dev/null 2>&1; then
  echo "docker CLI is not available in the workflow container" >&2
  exit 1
fi

if [ ! -S /var/run/docker.sock ]; then
  echo "/var/run/docker.sock is not mounted; local deployment cannot reach the host Docker daemon" >&2
  exit 1
fi

if [ ! -f Dockerfile.hangrix ] || [ ! -f docker-compose.deploy.yml ]; then
  echo "run this script from the repository root" >&2
  exit 1
fi

cat > "$override_file" <<YAML
services:
  ${service}:
    build:
      context: "$(pwd)"
      dockerfile: Dockerfile.hangrix
    volumes:
      - "${runtime_app}:/data"
YAML

echo "Docker:"
docker version --format 'client={{.Client.Version}} server={{.Server.Version}}'
docker compose version

echo "Deploying ${service} into compose project ${project}"
docker compose \
  -p "$project" \
  -f docker-compose.deploy.yml \
  -f "$override_file" \
  up -d --no-deps --build "$service"

echo "Container status:"
docker compose \
  -p "$project" \
  -f docker-compose.deploy.yml \
  -f "$override_file" \
  ps "$service"

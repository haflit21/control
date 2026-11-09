#!/bin/sh
# Construit et pousse l'image $REGISTRY/$APP:<AAAAMMJJ-HHMM>-<sha7> (heure de Paris).
# En CI, le tag est exporté pour les étapes suivantes (GITHUB_ENV).
set -eu
cd "$(dirname "$0")/../.."
. .cloud/forgejo/app.env
: "${REGISTRY:?REGISTRY manquant (variable de l'organisation)}"
SHA=${GITHUB_SHA:-$(git rev-parse HEAD)}
TAG="$(TZ=Europe/Paris date +%Y%m%d-%H%M)-$(printf %s "$SHA" | cut -c1-7)"
docker build -t "$REGISTRY/$APP:$TAG" .
docker push "$REGISTRY/$APP:$TAG"
[ -n "${GITHUB_ENV:-}" ] && echo "TAG=$TAG" >> "$GITHUB_ENV"
echo "image : $REGISTRY/$APP:$TAG"

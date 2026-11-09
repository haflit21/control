#!/bin/sh
# Tests du projet (CI et `make test`) : serveur Go, puis types de l'interface.
set -eu
cd "$(dirname "$0")/../.."
# l'interface embarquée (go:embed ui/dist) n'est construite qu'avec l'image
[ -d server/internal/server/ui/dist ] || { mkdir -p server/internal/server/ui/dist && : > server/internal/server/ui/dist/.gitkeep; }
(cd server && go vet ./... && go test ./...)
command -v pnpm >/dev/null 2>&1 || corepack enable
pnpm install --frozen-lockfile
pnpm -r typecheck

.PHONY: dev dev-ha demo build build-ui build-server test check deploy

# Interface Vite (5173, proxy /api) + serveur Go en mode démo (8080)
dev:
	@(cd server && MAISON_DEMO=1 MAISON_TOKEN=dev MAISON_CONFIG=../maison.yaml go run ./cmd/maison serve &) ; pnpm dev

# Interface Vite (5173) + serveur Go branché sur le vrai Home Assistant (.env : HA_URL, HA_TOKEN)
dev-ha:
	@(cd server && MAISON_TOKEN=dev MAISON_CONFIG=../maison.yaml go run ./cmd/maison serve &) ; pnpm dev

# Serveur Go seul en mode démo, interface embarquée (après make build)
demo: build
	MAISON_DEMO=1 MAISON_TOKEN=dev ./server/bin/maison serve

build: build-ui build-server

build-ui:
	pnpm install --frozen-lockfile
	pnpm build

build-server:
	cd server && go build -trimpath -ldflags "-s -w" -o bin/maison ./cmd/maison

test:
	cd server && go vet ./... && go test ./...
	pnpm -r typecheck

# Vérifie que chaque entité de maison.yaml existe dans HA (.env requis)
check:
	cd server && MAISON_CONFIG=../maison.yaml go run ./cmd/maison check

# Image pour le NAS (linux/amd64), registre local, puis redéploiement Portainer
deploy:
	docker build --platform linux/amd64 -t 192.168.10.157:5011/control:latest .
	docker push 192.168.10.157:5011/control:latest
	@echo "→ Portainer : stack « control », Pull and redeploy"

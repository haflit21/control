.PHONY: dev dev-ha demo build build-ui build-server test check secrets-init secret-from-env secrets-keys secrets secret-random secret-set secret-get spotify-login

# Interface Vite (5173, proxy /api) + serveur Go en mode démo (8080)
dev:
	@(cd server && CONTROL_DEMO=1 CONTROL_TOKEN=dev CONTROL_CONFIG=../control.yaml go run ./cmd/control serve &) ; pnpm dev

# Interface Vite (5173) + serveur Go branché sur le vrai Home Assistant (secrets déchiffrés avec la clé du Mac)
dev-ha:
	@(cd server && CONTROL_TOKEN=dev CONTROL_CONFIG=../control.yaml go run ./cmd/control serve &) ; pnpm dev

# Serveur Go seul en mode démo, interface embarquée (après make build)
demo: build
	CONTROL_DEMO=1 CONTROL_TOKEN=dev ./server/bin/control serve

build: build-ui build-server

build-ui:
	pnpm install --frozen-lockfile
	pnpm build

build-server:
	cd server && go build -trimpath -ldflags "-s -w" -o bin/control ./cmd/control

test:
	.cloud/forgejo/test.sh

# Vérifie que chaque entité de control.yaml existe dans HA (secrets déchiffrés avec la clé du Mac)
check:
	cd server && CONTROL_CONFIG=../control.yaml go run ./cmd/control check

# Déploiement : chaque push sur master (CI Forgejo, .cloud/forgejo/). La première fois, Sentinel
# crée la stack « control » et le nom control.home.arpa.

# ---- secrets (SOPS + age) : jamais affichés, jamais dans l'historique du shell ----
SECRETS := .cloud/secrets/secrets.enc.yaml
SOPS := sops --config .cloud/secrets/.sops.yaml

# Crée le fichier de secrets (chiffré pour les clés de .cloud/secrets/.sops.yaml)
secrets-init:
	@test ! -f $(SECRETS) || { echo "$(SECRETS) existe déjà"; exit 1; }
	@! grep -q CLE_PUBLIQUE .cloud/secrets/.sops.yaml || { echo "renseigne d'abord la clé publique du NAS dans .sops.yaml"; exit 1; }
	@printf 'HA_URL: "http://192.168.10.242:8123"\n' > $(SECRETS) && $(SOPS) encrypt -i $(SECRETS) && echo "$(SECRETS) créé"

# Reprend une valeur du .env sans l'afficher : make secret-from-env NAME=HA_TOKEN
secret-from-env:
	@test -n "$(NAME)" || { echo "NAME=… manquant"; exit 1; }
	@v=$$(grep "^$(NAME)=" .env | head -1 | cut -d= -f2- | sed -e 's/^["'"'"']//' -e 's/["'"'"']$$//'); test -n "$$v" || { echo "$(NAME) absent du .env"; exit 1; }; \
	 $(SOPS) set $(SECRETS) '["$(NAME)"]' "\"$$v\"" && echo "$(NAME) chiffré dans $(SECRETS)"

# Après un changement de destinataires dans .sops.yaml
secrets-keys:
	$(SOPS) updatekeys -y $(SECRETS)

# Ouvre les secrets déchiffrés dans l'éditeur, rechiffre à la fermeture
secrets:
	$(SOPS) $(SECRETS)

# Jeton aléatoire de 32 octets : make secret-random NAME=CONTROL_TOKEN
secret-random:
	@test -n "$(NAME)" || { echo "NAME=… manquant"; exit 1; }
	@$(SOPS) set $(SECRETS) '["$(NAME)"]' "\"$$(openssl rand -hex 32)\"" && echo "$(NAME) : nouveau jeton chiffré dans $(SECRETS)"

# Valeur saisie sans écho : make secret-set NAME=HA_TOKEN
secret-set:
	@test -n "$(NAME)" || { echo "NAME=… manquant"; exit 1; }
	@printf "valeur de $(NAME) (masquée) : "; stty -echo; read v; stty echo; echo; \
	 $(SOPS) set $(SECRETS) '["$(NAME)"]' "\"$$v\"" && echo "$(NAME) chiffré dans $(SECRETS)"

# Copie une valeur dans le presse-papiers du Mac, sans l'afficher : make secret-get NAME=CONTROL_TOKEN
secret-get:
	@$(SOPS) decrypt --extract '["$(NAME)"]' $(SECRETS) | tr -d '\n' | pbcopy && echo "$(NAME) copié dans le presse-papiers"

# Spotify (une fois) : autorise Control sur le compte Premium et chiffre le jeton de renouvellement,
# sans jamais l'afficher. Avant : une appli sur developer.spotify.com (URI de redirection
# http://127.0.0.1:8888/callback, ton compte dans « Users »), puis
# make secret-set NAME=SPOTIFY_CLIENT_ID et make secret-set NAME=SPOTIFY_CLIENT_SECRET.
spotify-login:
	@v=$$(cd server && CONTROL_CONFIG=../control.yaml go run ./cmd/control spotify-login) && test -n "$$v" && \
	 $(SOPS) set $(SECRETS) '["SPOTIFY_REFRESH_TOKEN"]' "\"$$v\"" && echo "SPOTIFY_REFRESH_TOKEN chiffré dans $(SECRETS)"

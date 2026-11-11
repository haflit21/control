# Control

L'interface du quotidien de la maison, au-dessus de Home Assistant : chauffage, lumières, volets, prises, clim, caméras, alarme, météo, par pièce, sur iPhone d'abord. Home Assistant reste le moteur (intégrations, automatisations) ; Control est un serveur Go intermédiaire et une PWA React. Brief et décisions : projet « Homelab » (`maison/brief.md`).

## Arborescence

- `docs/design-system/` — le design system (System · Mobile · Dashboard · Météo) : `tokens.css`, `widgets.css`, `weather.css`, `dashboard.css` sont importés tels quels par l'app ; `DESIGN.md` tient les décisions.
- `packages/model` — types TypeScript du modèle (miroir de `server/internal/model`).
- `packages/ui` — composants React, fins, sur les classes du design system ; `format.ts` pour le français (Intl).
- `apps/web` — l'app : accueil, pièces, sécurité, météo (+ qualité de l'air), agenda, mode mur `/mur` (tablette fixée : horloge, caméras en rotation, lumières, agenda, sans navigation) ; TanStack Query + SSE ; PWA.
- `server/` — module Go `ahomelab.io/control` : client WebSocket HA, normalisation (`internal/build`), API + SSE + session (`internal/server`), mode démo.
- `control.yaml` — pièces, appareils, entités HA. Sans secret.
- `media/` — photos des pièces (`rooms/`) et des appareils (`devices/`), servies sous `/media/` ; libres de droits (Pexels, voir `media/CREDITS.md`) en attendant les photos de la maison : remplacer les fichiers sous le même nom, c'est tout.

## Démarrer

```sh
# secrets (HA_URL, HA_TOKEN, CLAUDE_GATEWAY_TOKEN…) : .cloud/secrets/secrets.enc.yaml, déchiffré
# avec la clé age du Mac (make secrets) ; un .env (copie de .env.example) ne fait que compléter
pnpm install
make dev                        # serveur Go en démo sur :8080 + Vite sur :5173 (proxy /api)
make dev-ha                     # pareil, branché sur le vrai Home Assistant (secrets déchiffrés)
```

Production locale : `make build && ./server/bin/control serve` (lit `.env` et `control.yaml` dans le dossier courant). Le serveur affiche au démarrage `control : http://…/login?token=…` : ouvrir ce lien une fois par appareil pose le cookie de session (30 jours). Sur iPhone, ajouter ensuite à l'écran d'accueil.

`make check` liste les entités de `control.yaml` que Home Assistant ne connaît pas, les services HA dont l'app a besoin qui manqueraient, et l'état de la saisie rapide (clé Claude présente, service stt joignable). `control.yaml` et `.env` sont trouvés depuis la racine comme depuis `server/`.

## État (étapes 1 et 2 du brief)

Lecture complète en temps réel : `GET /api/home`, `GET /api/events` (SSE : `home`, `device`, `summary`, `status`), vignettes caméras (`/api/cameras/{id}/snapshot`, `/stream`), météo (actuelle, horaire, 7 jours via `weather/subscribe_forecast`), alertes (fenêtre ouverte en chauffe, pile faible, appareils injoignables, fumée/CO, connexion perdue), présence. Actions (étape 2) : `POST /api/devices/{id}/actions` `{action, params}` (lumières : `power`, `set_brightness`, `set_color_temp`, `set_color` ; volets : `open`, `close`, `stop`, `set_position` ; prises : `power` ; chauffage : `set_target`, `set_mode` planning / manuel (+ durée) / arrêt, Tado via `tado.set_climate_timer` ; clim : `power`, `set_mode`, `set_target`, `set_eco`, `set_boost` ; médias : `play`, `pause`, `set_volume` ; caméras : `set_privacy`) — une carte lampe allumée ouvre un panneau intensité / blanc / teinte / pastilles —, `POST /api/home/heating {mode}`, `/api/home/lights/off`, `/api/security/alarm {armed}`, `/api/security/alarm/ack`. Chaque action = un ou plusieurs appels de service HA ; l'interface se met à jour par les événements qui suivent (optimiste, estompé tant que HA n'a pas confirmé, retour arrière + toast sinon). Les gestes à conséquence (couper tout le chauffage, désarmer, tout éteindre, boost) passent par une feuille de confirmation. En démo, les services sont simulés sur l'état.

Qualité de l'air : Open-Meteo (sans clé) aux coordonnées de `globals.air` ou de `zone.home` ; indice européen rendu sur l'échelle ATMO.

Agenda : tenu par l'app (`state/agenda/events.json`, écrit de façon atomique), calendriers `agenda.calendars` de `control.yaml` (couleur, et un indice pour le modèle : « poubelles, tri, verre »). Événements ponctuels ou répétés (chaque semaine, toutes les 2 semaines, chaque mois, chaque année, avec date de fin), en heure locale : un rendez-vous hebdomadaire à 9 h 30 reste à 9 h 30 au changement d'heure. API : `GET/POST /api/agenda/events`, `GET/PUT/DELETE /api/agenda/events/{id}` (une série se modifie en entier), journalisé ; les écrans reçoivent la semaine courante + 5 semaines par SSE. Onglet Agenda : « Ajouter », micro, toucher un événement pour le modifier.

Saisie rapide : une phrase tapée (`POST /api/agenda/parse {text}`) ou dictée (`POST /api/agenda/voice`, corps = l'enregistrement) ouvre le formulaire pré-rempli ; rien n'est enregistré sans « Enregistrer ». Deux étapes indépendantes, configurées dans `agenda.voice` :
- **parole → texte** : le service stt du homelab (dépôt `ahomelab/stt`, NVIDIA Parakeet TDT 0.6B v3 sur le CPU du NAS, ~1,5 s par phrase, API compatible OpenAI `/v1/audio/transcriptions`, jeton `STT_API_KEY` = `CONTROL_TOKEN` des secrets de stt). L'audio n'est jamais stocké. La consigne de Claude prévient que la phrase dictée peut contenir des erreurs de reconnaissance.
- **texte → événement** : `provider: gateway` (claude-gateway, l'abonnement Claude du homelab, profil `calendar` : Haiku ; jeton `CLAUDE_GATEWAY_TOKEN`, réponse structurée par le schéma), ou `provider: anthropic` (clé `ANTHROPIC_API_KEY`), ou `provider: openai` + `url` + `model` pour n'importe quel modèle local compatible OpenAI (Ollama, llama.cpp…) : changer de fournisseur = ces trois lignes. Même consigne et même validation pour tous (`server/internal/voice`).
Le micro du navigateur exige HTTPS (ou `localhost`) ; sans clé ou sans service stt, le bouton correspondant n'apparaît pas et le formulaire reste. `make check` dit ce qui est configuré et si stt répond.

Photos : chaque pièce a `image:` dans `control.yaml` (relatif à `serve.media_dir`, défaut `media/`, ou `CONTROL_MEDIA`) ; l'image par type d'appareil est choisie par l'interface, `image:` sur un appareil la remplace. Les pièces avec caméra montrent la caméra et retombent sur la photo quand elle est privée ou injoignable ; en démo, la photo tient lieu de caméra.

Notifications hors app (Web Push) : dans la cloche, « Recevoir les alertes ici » abonne l'appareil (permission du navigateur, puis `POST /api/push/subscribe` ; `GET /api/push/key` donne la clé VAPID publique, `/api/push/unsubscribe`, `/api/push/test`). Le serveur chiffre lui-même (RFC 8291, bibliothèque standard) et pousse à chaque **nouvelle** alerte `warn`/`crit` hors piles, au déclenchement de l'alarme et à la perte de HA ; les abonnements morts (404/410) sont oubliés. Clé VAPID et abonnements dans `state_dir/push/` (perdre ce dossier = chaque appareil doit se réabonner). Il faut HTTPS (ou l'app installée) : sur iPhone, l'app doit être ajoutée à l'écran d'accueil (iOS 16.4+).

Journal : chaque action (qui, quoi, quand, depuis quel appareil, acceptée ou refusée) est ajoutée à `state_dir/journal/AAAA-MM.jsonl` ; `GET /api/journal?limit=50` (les 500 dernières en mémoire), carte « Journal » en bas de Sécurité. Un seul utilisateur en v1 : la première personne de `globals.presence`.

Valeurs grisées quand le serveur a perdu Home Assistant ou quand l'app a perdu le serveur depuis plus de 2 min.

## Déploiement

Chaque push sur `master` : CI Forgejo (`.forgejo/workflows/deploy.yml`, qui n'appelle que `.cloud/forgejo/`) : tests, image `control:<AAAAMMJJ-HHMM>-<sha7>` (`Dockerfile`, multi-étapes, distroless, utilisateur non-root, port 8080), déploiement vérifié par le moteur de Sentinel. La première fois, Sentinel crée la stack `control` (`.cloud/portainer-stack.yml` : macvlan `lan_macvlan`, IP 192.168.10.6, volume `control_state`, aucune variable secrète) et le nom `control.home.arpa`. Secrets : `.cloud/secrets/secrets.enc.yaml` (SOPS + age : clé du Mac et clé du service `/volume1/docker/secrets/control/age.key`), copié dans l'image et déchiffré au démarrage ; changer un secret = `make secret-set NAME=…`, commit, push. Image, stack et conteneur portent le nom du dépôt (`ahomelab/control`) pour que Sentinel rapproche déploiements et commits . Jamais publié via NPM : réseau de la maison et VPN seulement (`serve.allow_from`).

## Conventions

Go bibliothèque standard (une dépendance : `goccy/go-yaml`), `go build ./... && go vet ./... && go test ./...` et `pnpm -r typecheck` doivent passer. Tout ce qui vient de HA est une donnée non fiable, normalisée côté serveur et échappée à l'affichage. Commentaires en anglais, interface en français.

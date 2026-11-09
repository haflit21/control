# Maison

L'interface du quotidien de la maison, au-dessus de Home Assistant : chauffage, lumières, volets, prises, clim, caméras, alarme, météo, par pièce, sur iPhone d'abord. Home Assistant reste le moteur (intégrations, automatisations) ; Maison est un serveur Go intermédiaire et une PWA React. Brief et décisions : projet « Homelab » (`maison/brief.md`).

## Arborescence

- `docs/design-system/` — le design system (System · Mobile · Dashboard · Météo) : `tokens.css`, `widgets.css`, `weather.css`, `dashboard.css` sont importés tels quels par l'app ; `DESIGN.md` tient les décisions.
- `packages/model` — types TypeScript du modèle (miroir de `server/internal/model`).
- `packages/ui` — composants React, fins, sur les classes du design system ; `format.ts` pour le français (Intl).
- `apps/web` — l'app : accueil, pièces, sécurité, météo (+ qualité de l'air), agenda, mode mur `/mur` (tablette fixée : horloge, caméras en rotation, lumières, agenda, sans navigation) ; TanStack Query + SSE ; PWA.
- `server/` — module Go `ahomelab.io/maison` : client WebSocket HA, normalisation (`internal/build`), API + SSE + session (`internal/server`), mode démo.
- `maison.yaml` — pièces, appareils, entités HA. Sans secret.
- `media/` — photos des pièces (`rooms/`) et des appareils (`devices/`), servies sous `/media/` ; libres de droits (Pexels, voir `media/CREDITS.md`) en attendant les photos de la maison : remplacer les fichiers sous le même nom, c'est tout.

## Démarrer

```sh
cp .env.example .env            # HA_URL, HA_TOKEN (utilisateur HA « maison-app »), ou MAISON_DEMO=1
pnpm install
make dev                        # serveur Go en démo sur :8080 + Vite sur :5173 (proxy /api)
```

Production locale : `make build && ./server/bin/maison serve` (lit `.env` et `maison.yaml` dans le dossier courant). Le serveur affiche au démarrage `maison : http://…/login?token=…` : ouvrir ce lien une fois par appareil pose le cookie de session (30 jours). Sur iPhone, ajouter ensuite à l'écran d'accueil.

`make check` liste les entités de `maison.yaml` que Home Assistant ne connaît pas.

## État (étapes 1 et 2 du brief)

Lecture complète en temps réel : `GET /api/home`, `GET /api/events` (SSE : `home`, `device`, `summary`, `status`), vignettes caméras (`/api/cameras/{id}/snapshot`, `/stream`), météo (actuelle, horaire, 7 jours via `weather/subscribe_forecast`), alertes (fenêtre ouverte en chauffe, pile faible, appareils injoignables, fumée/CO, connexion perdue), présence. Actions (étape 2) : `POST /api/devices/{id}/actions` `{action, params}` (lumières : `power`, `set_brightness`, `set_color_temp`, `set_color` ; volets : `open`, `close`, `stop`, `set_position` ; prises : `power` ; chauffage : `set_target`, `set_mode` planning / manuel (+ durée) / arrêt, Tado via `tado.set_climate_timer` ; clim : `power`, `set_mode`, `set_target`, `set_eco`, `set_boost` ; médias : `play`, `pause`, `set_volume` ; caméras : `set_privacy`) — une carte lampe allumée ouvre un panneau intensité / blanc / teinte / pastilles —, `POST /api/home/heating {mode}`, `/api/home/lights/off`, `/api/security/alarm {armed}`, `/api/security/alarm/ack`. Chaque action = un ou plusieurs appels de service HA ; l'interface se met à jour par les événements qui suivent (optimiste, estompé tant que HA n'a pas confirmé, retour arrière + toast sinon). Les gestes à conséquence (couper tout le chauffage, désarmer, tout éteindre, boost) passent par une feuille de confirmation. En démo, les services sont simulés sur l'état.

Qualité de l'air : Open-Meteo (sans clé) aux coordonnées de `globals.air` ou de `zone.home` ; indice européen rendu sur l'échelle ATMO. Agenda : entités `calendar.*` listées dans `globals.calendars` (REST `/api/calendars/…` de HA), cinq semaines chargées, rafraîchies toutes les 10 min, onglet Agenda + coup d'œil sur l'accueil.

Photos : chaque pièce a `image:` dans `maison.yaml` (relatif à `serve.media_dir`, défaut `media/`, ou `MAISON_MEDIA`) ; l'image par type d'appareil est choisie par l'interface, `image:` sur un appareil la remplace. Les pièces avec caméra montrent la caméra et retombent sur la photo quand elle est privée ou injoignable ; en démo, la photo tient lieu de caméra.

Notifications hors app (Web Push) : dans la cloche, « Recevoir les alertes ici » abonne l'appareil (permission du navigateur, puis `POST /api/push/subscribe` ; `GET /api/push/key` donne la clé VAPID publique, `/api/push/unsubscribe`, `/api/push/test`). Le serveur chiffre lui-même (RFC 8291, bibliothèque standard) et pousse à chaque **nouvelle** alerte `warn`/`crit` hors piles, au déclenchement de l'alarme et à la perte de HA ; les abonnements morts (404/410) sont oubliés. Clé VAPID et abonnements dans `state_dir/push/` (perdre ce dossier = chaque appareil doit se réabonner). Il faut HTTPS (ou l'app installée) : sur iPhone, l'app doit être ajoutée à l'écran d'accueil (iOS 16.4+).

Journal : chaque action (qui, quoi, quand, depuis quel appareil, acceptée ou refusée) est ajoutée à `state_dir/journal/AAAA-MM.jsonl` ; `GET /api/journal?limit=50` (les 500 dernières en mémoire), carte « Journal » en bas de Sécurité. Un seul utilisateur en v1 : la première personne de `globals.presence`.

Valeurs grisées quand le serveur a perdu Home Assistant ou quand l'app a perdu le serveur depuis plus de 2 min.

## Déploiement

`make deploy` construit l'image (`Dockerfile`, multi-étapes, distroless) et la pousse sur le registre du NAS ; stack Portainer dans `deploy/portainer-stack.yml` (macvlan, IP à réserver, `.env` dans les variables de la stack). Pi-hole : `maison.home.arpa → 192.168.10.6`. Jamais publié via NPM : réseau de la maison et VPN seulement (`serve.allow_from`).

## Conventions

Go bibliothèque standard (une dépendance : `goccy/go-yaml`), `go build ./... && go vet ./... && go test ./...` et `pnpm -r typecheck` doivent passer. Tout ce qui vient de HA est une donnée non fiable, normalisée côté serveur et échappée à l'affichage. Commentaires en anglais, interface en français.

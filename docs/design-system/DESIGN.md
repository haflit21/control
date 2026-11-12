# Design system de Control — v2 « ambiance »

Quatre pages, un menu commun en haut (System · Mobile · Dashboard · Météo) :
- `index.html` — **System** : les fondations, chaque widget dans tous ses états, la table données → widget (`?theme=dark|light`, `?phone=1`).
- `mobile.html` — **Mobile** : les deux écrans iPhone composés (accueil, page Salon).
- `dashboard.html` — **Dashboard** : le grand écran (iPad paysage, Mac), pleine page, responsive (3 colonnes, 2 sous 1280 px, 1 sous 860 px).
- `meteo.html` — **Météo** : les 15 glyphes de condition (jour / nuit, 5 tailles), les icônes trait, et tous les widgets météo : en-tête, pilules, grille de détails, boussole, UV, pression, pluie dans l'heure, horaire avec courbe, journalier et jour ouvert, soleil, vigilance Météo-France, qualité de l'air (indice ATMO, polluants, pollens), dedans / dehors, tuiles, états ; table données HA → widget.

Fichiers destinés à l'app, à importer tels quels : `tokens.css`, `widgets.css`, `weather.css`, `widgets.js` (points, arcs, glyphes météo et courbes dessinés depuis les attributs `data-*`), `dashboard.css` (la mise en page large). Fichiers propres au catalogue : `catalogue.css`, `catalogue.js`. Les noms de classes sont le contrat des futurs composants React.

## 1. Lecture de design

> Une app de maison pour une personne, iPhone d'abord, regardée vingt fois par jour pendant deux secondes. Ambiance : une toile de nuit sur laquelle la lumière chaude glow. Photos d'ambiance sous du verre dépoli, grands chiffres fins, pilules de mesures, un seul accent lumineux (terracotta) pour ce qui est allumé, sélectionné ou qui chauffe. Densité moyenne (44 px), variance basse, mouvement minimal, couleur sémantique par-dessus les deux teintes d'identité.

Références fournies par Abdelhak (2026-11-08) : six tableaux de bord « smart home » (sync, Rumah, Realist Home, Smart House, zoya, Sellution). Ce qu'ils ont en commun et qu'on reprend : fond charbon/nuit profond, imagerie des pièces en hero, surfaces translucides à grand rayon, pilules de mesures posées sur la photo, un accent vif avec lueur sur les bascules et l'onglet actif, chiffres géants en graisse fine, cartes d'appareils avec image + étiquette de verre + bouton marche, barre de pièces en segments, lecteur média, météo illustrée, tuiles pastel pour l'énergie, jauges à points ou à arc.

## 2. Ce qu'on reprend de Sentinel, ce qu'on ne reprend pas

Repris (principes de `sentinel-taste`) : résumé avant détail ; état porté par la forme et le libellé, jamais par la couleur seule ; chiffres tabulaires ; seuls les états transitoires s'animent ; états vide / chargement / erreur dessinés ; honnêteté (inconnu en pointillé, périmé daté, « — » pour une valeur absente) ; budget de mouvement et ses tokens ; règle du 2× ; formats `Intl` fr-FR ; copie en phrases, sans « avec succès » ni point d'exclamation ; tout ce qui vient de HA est échappé.

Pas repris : la charte de Sentinel (teal, neutres froids, densité de console, monospace, bords fins sans ombre), ni sa règle « accent ≤ 5 % de l'écran » : ici l'accent est plus présent (bascules, picto allumé, segment actif, bouton primaire) parce que c'est la lumière de la maison. La contrepartie : un seul type de lueur à l'écran, jamais deux accents.

## 3. Fondations

### Couleurs
- **Toile** `--bg #0d1017` (nuit), avec deux lueurs d'ambiance peintes par `body::before` (chaude en haut à gauche, bleu nuit en bas à droite).
- **Verre** : `--surface` 6 % de blanc, `-2` 10 %, `-3` 15 %, flou 18 px, bord 8 %. Repli opaque `--surface-solid #161a23` pour les cartes posées directement sur la toile quand le flou coûte trop.
- **Accent** `--accent #ec9b74` (terracotta lumineuse) + `--accent-glow` : interactif, sélectionné, allumé. `--heat-*` partage la même teinte : la flamme et la consigne active sont de la même famille que « allumé », volontairement (ce qui est allumé éclaire) ; l'icône distingue.
- **Nuit** `--night #a6bbe6` : tuiles d'information, alerte « info ».
- États : `--cool` bleu d'eau (froid, clim), `--ok` vert, `--warn` ambre, `--crit` rouge, `--neutral` gris. Teintes ≥ 15° entre familles (accent/chaleur 20°, attention 42°, ok 140°, froid 192°, nuit 225°, alerte 358°).
- **Tuiles pastel** (`--tile-peach/-blue/-mint/-sand`) à encre sombre, pour quelques chiffres clés.
- **Clair** : dérivé, ivoire chaud `#f1eee8`, verre blanc translucide, accent assombri `#c2562c`. Dessiné en second.
- Contrastes vérifiés dans le catalogue (verre composé sur la toile) : texte ≥ 4,5:1, méta et bords ≥ 3:1, dans les deux thèmes.

### Typographie
Deux faces OFL auto-hébergées : **Outfit** (titres, chiffres ; 300 sur les grands nombres) et **Inter** (texte). Échelle 12 / 13 / 15 / 17 / 22 / 30 / 56 / 72. Libellés secondaires en capitales espacées (12 px, +6 %). Dans le catalogue les polices viennent de Google Fonts ; dans l'app elles sont embarquées (PWA hors ligne).

### Forme et mouvement
Rayons 14 / 22 / 30 / pilule. Ombre portée douce sous les cartes qui portent une photo, aucune sous le verre plat. Lignes 48 et 56 px, cibles 44 px, grille 4 px. Mouvement : tokens Sentinel inchangés ; le glow ne s'anime pas.

### Photos
Les cartes héros et les cartes de pièce sont faites pour recevoir les captures des caméras (Salon, Bureau) et des photos des autres pièces. En attendant, `.ph` dessine une ambiance en CSS (mur à panneaux, lampe, sol) avec des variantes par pièce.

## 4. Widgets d'affichage (v2)

| Widget | Classe | Variantes | États |
|---|---|---|---|
| Valeur | `.val`, `.stat` | `-giant` 72, `-hero` 56, `-std` 30, `-s` 22, `-meta` | `is-none`, `is-stale`, `is-heat`, `is-cool` |
| Pilule de mesure | `.chip`, `.chip-row` | `-s`, `-l`, `-on-photo` | `is-heat/-cool/-ok/-warn/-crit`, `is-live` |
| Paire actuelle → consigne | `.pair` | compacte, `-wide` | `is-heating`, `is-cooling`, `is-off` |
| Point d'état + libellé | `.dot`, `.status` | `-m`, `-l`, `.status.-s` | `is-ok/-warn/-crit/-heat/-cool/-accent`, `is-unknown`, `is-live` |
| Picto d'état | `.pict`, `.pictv`, `.pict-n` | `-s` 32, 40, `-l` 48, `-xl` 56 | éteint, `is-lit`, `is-on` (accent + glow), `is-heat/-cool/-warn/-crit/-ok/-night`, `is-unknown` |
| Étiquette | `.tag` | `-live` | neutre, `is-accent`, `is-heat/-cool/-ok/-warn/-crit`, `is-unknown` |
| Jauges | `.gauge`, `.dots`, `.arc` | barre (`-heat/-cool/-ok/-warn/-crit`, `-thick`, `-hatched`), points (`-heat/-cool/-l`), arc (`-heat`) | `is-none` |
| Bascule, bouton marche | `.toggle`, `.pwr` | `-s` | `is-on`, `is-busy` |
| Temps relatif | `.time` | — | `is-old`, `is-live` |
| Compteur-phrase | `.phrase` | `-l` | `is-zero`, `is-good`, `is-heat`, `is-on`, `is-warn` |
| Carte héros | `.hero` | 4:3, `-wide` 16:9, `-tall` | direct, `is-private`, `is-offline`, `is-loading` |
| Carte appareil | `.dev`, `.obj` | `.obj -speaker / -shade` (dessins par défaut) | `is-off` |
| Tuile | `.tile` | pêche, `-blue`, `-mint`, `-sand`, `-glass` | — |
| Liste | `.list`, `.row` | — | — |
| Lecteur | `.player` | — | `is-idle` |
| Barre de pièces, navigation | `.seg`, `.nav` | — | `is-active` |
| Météo | `.wx`, `.wx-glyph`, `.wx-hours`, `.wx-days` | glyphe soleil / `-moon` / nuage (`.wx-cloud`), `-night` | `is-now` |
| Présence | `.who`, `.avatar`, `.who-pill` | `-s`, `.who-stack` | `is-home`, `is-away` |
| Alerte | `.alert` | — | neutre, `is-info`, `is-warn`, `is-crit` |
| Surface, carte de pièce | `.card`, `.room`, `.sec`, `.greet` | `-solid`, `-tight`, `-hero` | `is-stale`, `is-unavailable`, `is-crit` |

Règles transversales inchangées : valeur absente = « — » + raison dans `title` ; périmée = grisée, jamais cachée ; injoignable = pointillé et daté ; point ou picto isolé = `role="img"` + `aria-label` ; une seule lueur par zone ; jamais de sens porté par la couleur seule.

## 5. Décisions prises en dessinant

- **v1 abandonnée** (2026-11-08, même jour) : la version « console sobre » (ivoire, bleu nuit en accent, bords fins) a été jugée trop simple et pas assez belle par Abdelhak, références à l'appui. v2 inverse le rôle des deux couleurs : le bleu nuit devient la toile, la terracotta devient la lumière.
- **Allumé = chauffe en couleur**, distingués par l'icône. Assumé, parce que les deux sont « de la lumière chaude dans la maison ».
- **Une seule lueur par zone** : la bascule allumée, le picto `is-on`, le segment actif de la navigation. Les pilules de mesures et les étiquettes ne glowent pas.
- **Carte de pièce** sur photo, deux par ligne, 176 px de haut : picto d'état en haut à gauche, mesure courte en haut à droite, nom et température en bas sur un dégradé sombre.
- **Carte appareil** : bouton marche en haut à droite (pas dans l'étiquette de verre, trop étroite à 170 px).
- **Fenêtre ouverte** reste ambre ; **mode privé** et **injoignable** sur une caméra ne sont pas rouges (image estompée, icône centrée).

## 6. À faire ensuite

1. Regarder le catalogue sur un vrai iPhone (Outfit chargée, glow, verre) et ajuster les tailles.
2. Remplacer les ambiances CSS par de vraies images : captures des caméras Salon et Bureau, photos des autres pièces (cadrées 4:3 et 16:9), photos ou dessins des appareils.
3. Widgets de **commande** : bascule et bouton marche actifs, segment de modes (Planning / Manuel / Arrêt), stepper − / + de consigne, curseur (volet, intensité, volume), confirmation inline, toast, bandeau connexion perdue, pastille de couleur Hue.
4. Compositions restantes : page Sécurité ; variantes du dashboard (pièce sélectionnée, tablette murale).
5. Portage React (`packages/ui`) + Storybook, classes et tokens inchangés ; polices embarquées.

## 6b. Météo — décisions
- Les glyphes sont dessinés (CSS + SVG inline par `widgets.js`), jamais des bitmaps : un disque (soleil / lune), un ou deux nuages, un effet sous le nuage. `data-night` = lune + nuages assombris ; fortes pluies, orages, grêle et exceptionnel ont un nuage sombre.
- Les 15 conditions sont celles de Home Assistant (`weather.*` state) ; libellés français fixés dans `widgets.js` (`labels`, `labelsNight`).
- Pluie dans l'heure (produit Météo-France) : la phrase d'abord, la barre à six tranches la justifie ; intensité 0–3 en `lv0`–`lv3`.
- UV : échelle officielle à cinq niveaux (`--uv-1`…`--uv-5`) ; vigilance : échelle officielle vert / jaune / orange / rouge (`--vig-*`), distincte des couleurs d'état de l'app (l'orange de vigilance n'est pas la terracotta).
- Le vent se lit à la boussole (flèche = d'où vient le vent), la pression à sa tendance, le soleil à son arc ; une donnée absente est « — » (l'UV la nuit, le vent calme).
- Qualité de l'air : indice ATMO à six niveaux avec l'échelle de couleurs officielle (`--atmo-1`…`--atmo-6`), polluant dominant en premier, sous-indices par polluant, pollens par espèce (0–6) ; alerte à partir de « mauvais » (4). Source à brancher : Atmo AURA (ou WAQI / Open-Meteo) ; pas de capteur intérieur en v1.
- La météo est rafraîchie toutes les quelques minutes par HA : la fraîcheur s'affiche toujours, ambre au-delà de 30 min.

## 6c. Portage dans l'app (étape 1) — décisions
- Les quatre feuilles `tokens.css`, `widgets.css`, `weather.css`, `dashboard.css` sont la **source unique** : `packages/ui/src/styles.css` les importe telles quelles, l'app n'a qu'un `app.css` de mise en page (coquille, rail, barre basse, grille d'accueil). Toute correction visuelle se fait ici, puis se voit dans l'app.
- Les composants React (`packages/ui`) sont des enveloppes fines sur les classes du catalogue, mêmes noms (`Hero`, `DevCard`, `Tile`, `Pill`, `Arc`, `WxHero`…) ; pas de style dans le TSX. Le sprite d'icônes du catalogue est extrait tel quel dans `sprite.ts`.
- Les polices sont embarquées (`@fontsource/outfit`, `@fontsource/inter`) : rien ne sort vers Google Fonts depuis la maison.
- La page Pièce prend le nom de la pièce en titre du héros ; la grille d'accueil passe en `display: contents` sous 860 px (zones nommées annulées) pour empiler proprement.
- Les bascules, boutons marche et segments sont rendus mais **inertes** en étape 1 ; le serveur répond 501 aux actions. Pas de variante visuelle « désactivé » ajoutée : on garde l'apparence finale, l'interaction arrive en étape 2.
- Correctifs remontés depuis l'app vers le catalogue : `.room > *:not(.ph):not(.hero-img) { position: relative }` (les cartes de pièce étaient grises, la photo passait au-dessus), `button.hero/.room` sans bordure ni padding natifs, modificateurs `.glass.-two/-three` (identifiants `-2/-3` invalides).

## 6d. Photos — décisions
- Les ambiances CSS (`.ph`) étaient trop vides : les cartes de pièce, le héros et les cartes appareil reçoivent des **photos**. Source provisoire : Pexels (libres de droits, `media/CREDITS.md`), choisies sombres, une lampe chaude, pour rester dans la nuit + terracotta. Les vraies photos de la maison prendront leur place sous le même nom.
- Une photo se pose toujours en `<img class="hero-img">` (cover) ; le conteneur prend `has-img`, qui ajoute un voile dégradé bas pour la lisibilité du verre et du texte. Éteint = photo désaturée et assombrie ; injoignable = gris foncé ; caméra privée / hors ligne = photo de la pièce grisée.
- Appareils : image par type (ampoule allumée / éteinte, store, prise, ouvrant, enceinte, radiateur, caméra, clim), carrée ; volets et prises deviennent des cartes appareil comme les lumières ; radiateur, clim et ouvrants gardent une ligne mais avec une vignette ronde (`.thumb`) à la place du picto.
- `.ph` reste pour l'état « chargement » et comme repli quand aucune image n'est configurée.

## 6e. Agenda, qualité de l'air, mise en page — décisions
- **Accueil** : plus de section Pièces (l'onglet Pièces devient une page de blocs photo) ; la météo est une pastille d'en-tête à côté des personnes ; trois colonnes pleines (chauffage · caméra + tuiles + agenda · allumées + ouvrants). Tout tient dans un écran de Mac.
- **Page de pièce** en deux colonnes ≥ 1100 px : caméra, chauffage, clim, capteurs à gauche ; lumières, volets, prises, médias à droite.
- **Sécurité** : alarme (avec détecteurs et présence) · caméras deux par ligne · ouvrants sur deux colonnes · mouvement.
- **Couleur des lampes** : `--tint` (teinte HS de l'ampoule, ou blanc froid / très chaud) colore la photo (`mix-blend-mode: color` + lueur `screen`) et le bouton marche. Blanc chaud = rien à teinter.
- **Qualité de l'air** : les widgets v2.5 branchés sur Open-Meteo (indice européen → six niveaux ATMO, sous-indices, pollens en grains/m³ → 0–6). Alerte à partir de « mauvais ». Pastille « Air moyen » dans l'en-tête de l'accueil.
- **Agenda** (`agenda.css`) : semaine en sept colonnes, liste par jour sur téléphone, coup d'œil sur l'accueil. Couleur par calendrier sur une barre fine ; journée entière en aplat teinté ; passé estompé ; aujourd'hui en terracotta. Deux semaines affichées sur grand écran.
- Icônes ajoutées : `calendar`, `chevron-left`, `chevron-right`, `pin`.

## 6f. Commande (étape 2) — décisions
- Les bascules et boutons marche deviennent de vrais `<button>` (`role=switch` / `aria-pressed`), même apparence, état pressé (scale .94), état `is-pending` (opacité .7) tant que Home Assistant n'a pas renvoyé l'état.
- **Optimiste mais honnête** : le contrôle prend la valeur demandée tout de suite ; si rien ne revient en 8 s il reprend l'ancienne et un toast d'erreur le dit ; une erreur HTTP remet l'état précédent immédiatement.
- Pas à pas (`.stepper`) pour la consigne : demi-degré, envoi 600 ms après le dernier appui ; variante `-heat` et `-l` dans la carte chauffage. Curseur (`.slider`) pour volet, intensité, volume : envoi à la relâche (clavier aussi), variante `-cool` pour le volume.
- Modes de chauffage : segment Planning / Manuel / Arrêt ; en manuel, pilules de durée (prochain bloc, 1 h, 3 h, sans fin) ; Tado reçoit `tado_hijack.set_mode` (overlay next_block / manual + durée).
- Feuille de confirmation (`.scrim` + `.sheet`, portail sur `body`) pour couper le chauffage d'une pièce ou de la maison, booster, désarmer, tout éteindre ; un seul geste par feuille, bouton danger quand c'est un arrêt.
- Toasts (`.toasts`) au-dessus de la barre du bas : succès 3,5 s, erreur 8 s avec bouton OK ; un toast par action nommée (« Salon · consigne 21° »), silencieux pour les gestes évidents (allumer une lampe).
- Barre du bas en colonnes automatiques (quatre onglets avec Agenda).

## 6g. Panneau lumière — décisions
- Une carte lampe allumée s'ouvre (clic sur la carte, pas sur le bouton) en un panneau flottant sur la même feuille de verre que la confirmation : intensité, blanc (2 000 → 6 500 K), teinte, saturation, huit pastilles (trois blancs, cinq couleurs). Les curseurs n'apparaissent que si l'ampoule sait le faire (`set_color_temp`, `set_color`).
- La piste du curseur porte la couleur (dégradé blanc chaud → froid, arc-en-ciel, blanc → teinte), la poignée reste blanche cerclée de sombre. L'échantillon rond en tête et la photo de la carte prennent la couleur courante.
- Bouger la teinte ou choisir une pastille couleur passe l'ampoule en mode couleur ; bouger le blanc ou une pastille blanche la ramène en blanc (c'est le comportement de Home Assistant, on ne l'habille pas).

## 6h. Mode mur (`wall.css`) — décisions
- `/mur` : l'écran entier est le tableau, sans rail ni barre ; grande horloge fine (Outfit 300, 72 px) et date, alertes en colonne au centre, météo + personnes + bouton « ouvrir l'app » à droite. Grille chauffage · caméra + lumières · agenda + ouvrants.
- Les caméras disponibles tournent toutes les 20 s (points de rotation à côté de LIVE) ; une seule carte, grande.
- Les lumières allumées sont des pilules larges (56 px) avec la vignette teintée : un appui éteint ; « Tout éteindre » passe par la feuille.
- Alarme en pied de la carte chauffage (bascule agrandie 1,15×) ; désarmer demande confirmation.
- La nuit (sun.sun) l'écran baisse à 72 % de luminosité ; `wakeLock` garde l'écran allumé tant que la page est visible.
- La cloche du rail ouvre la liste des alertes dans la même feuille ; badge = nombre d'alertes.

## 6i. Notifications — décisions

- Une ligne dans la cloche, pas un écran « réglages » : « Recevoir les alertes ici » + interrupteur, et « Envoyer un test » une fois abonné. L'état impossible (navigateur sans push, permission refusée) est dit dans le sous-titre, l'interrupteur reste visible mais inactif.
- Sur téléphone, la cloche est dans l'en-tête de l'accueil (le rail n'existe pas) ; même popover, même contenu.
- On ne notifie que ce qui mérite de sortir l'app : nouvelles alertes `warn`/`crit` (jamais les piles), alarme déclenchée, HA perdu. Rien au démarrage du serveur (les alertes déjà là ne re-sonnent pas), re-sonne si une alerte disparaît puis revient. `tag` = identifiant de l'alerte : une seule notification par alerte, remplacée.
- Crit : `requireInteraction` + vibration longue ; le clic ouvre la pièce ou la sécurité.

## 6j. Apparence et accessibilité — décisions

- Apparence par appareil (Auto / Sombre / Clair) dans la feuille de la cloche, section « Cet appareil », à côté des notifications : deux réglages qui ne valent que pour l'appareil qu'on tient. Stockée localement, appliquée en `<html data-theme>` (que `tokens.css` comprenait déjà) avant le premier rendu ; la couleur de barre d'état (`theme-color`) suit.
- Audit axe-core (WCAG 2.1 AA + bonnes pratiques) sur toutes les pages, 390 / 1280, sombre et clair : zéro violation, contrastes compris. Ce qui a changé : `<main>` autour du contenu, un `h1` par page (masqué quand la page n'a pas de titre visible), titres de section en `h2`, prévision horaire défilable au clavier.
- Carte lampe : plus de bouton dans un bouton. `.dev-open` (bouton transparent qui couvre la carte, « réglages ») et le bouton marche sont frères ; le doigt ne voit pas de différence.
- Cibles ≥ 44 px sans grossir le dessin : zone de toucher étendue par un `::before` transparent (bascules, puces, marche, pas à pas, navigation de l'agenda, segment d'apparence), curseurs à 44 px de haut, lien « Semaine » avec marge négative.

## 6k. Agenda en écriture et saisie vocale — décisions

- L'agenda appartient à l'app (plus de calendriers HA) ; la saisie se fait dans une feuille `.sheet.-form` qui défile, champs de 44 px en 16 px (pas de zoom iOS), sélecteurs natifs de date et d'heure.
- En tête de la feuille, une **saisie rapide** (`.quick`) : une phrase tapée ou dictée pré-remplit le formulaire, la transcription est rappelée (`.heard`, « Compris : … — vérifie avant d'enregistrer »). Jamais d'enregistrement sans « Enregistrer ».
- Le micro (`.mic`) est terracotta au repos et **rouge qui respire** à l'écoute : l'anneau suit le niveau sonore (`--lvl`). L'écoute s'arrête seule après une pause (1,6 s après la parole) ou au toucher du carré ; « Je comprends ta phrase… » (`.thinking`) pendant l'aller-retour.
- Le micro de la page démarre l'enregistrement dans le toucher même (Safari n'autorise l'audio que là), puis la feuille s'ouvre en mode écoute.
- Calendrier : pastilles colorées (`.cal-pick`) ; répétition : segment de cinq choix qui passe à la ligne ; un événement répété porte une petite icône `repeat` et la feuille prévient que la modification vaut pour toute la série. Suppression en deux temps (icône poubelle → « Supprimer ? »).
- Date et heures : la date sur sa ligne au téléphone, les trois côte à côte dès 560 px.

## 7. Journal

- **2026-11-09 · v2.14** — section 24 Formulaire (`.sheet.-form`, `.field`, `.input`, `.quick`, `.heard`, `.mic`, `.listen`, `.thinking`, `.cal-pick`, `.agenda-tools`), `button.ev` cliquable, icônes `repeat`, `mic`, `trash`, `stop`.
- **2026-11-09 · v2.13** — apparence par appareil (`.theme-row`, `.sheet-sec`), section 23 Accessibilité (`.dev-open`, zones de toucher), `.sec h2`, `.sec-link` hors des sections.
- **2026-11-09 · v2.12** — journal des actions (carte en bas de Sécurité, lignes sur 2–3 colonnes, heure tabulaire `.jr-time`, « Voir plus »).
- **2026-11-09 · v2.11** — notifications push (ligne dans la cloche, cloche sur téléphone, `sw.js` `push`/`notificationclick`, badge monochrome `icons/badge.png`).
- **2026-11-09 · v2.10** — mode mur (`wall.css`, page `/mur`), cloche d'alertes, `button.hero` en pleine largeur (bug : héros bouton sans contenu en flux = largeur nulle).

- **2026-11-09 · v2.9** — panneau lumière (couleur / blanc / intensité / pastilles) dans le catalogue et dans l'app ; cartes lampe cliquables ; `Popover` (feuille sans actions).

- **2026-11-09 · v2.8** — étape 2 : section Commande (bascule et bouton actifs, pas à pas, curseur, confirmation, toast), actions branchées dans l'app (lumières, volets, prises, chauffage, clim, médias, caméras, alarme, chauffage maison), simulation côté démo.

- **2026-11-09 · v2.7** — agenda (`agenda.css`, section Agenda), qualité de l'air sur la page Météo de l'app, accueil / pièces / sécurité réorganisés, images produit par lampe (allumé / éteint) et teinte couleur.

- **2026-11-08 · v2.6** — photos : `media/` (6 pièces, 9 appareils), `/media/` côté serveur, `image:` dans `control.yaml`, `has-img` + `.thumb` dans `widgets.css`, catalogue et app passés sur les photos.

- **2026-11-11 · maquettes Figma → app** — d'après le fichier Figma « Projects », page Control : bannière de pièce sur toute la largeur (`.rbanner` : photo basse 240 px caméra éteinte, vidéo 16:9 caméra allumée, bouton sur photo `.chip.-btn`), médias (`.stations` / `.station` avec barres `.eq`, résultats Spotify `.tracks` / `.track`, champ `.search`), mur de caméras (`.camwall`, vignettes `.camthumb`, « À l'écran »), Météo en colonnes à hauteur naturelle, écran de veille (`.sleep` dans `wall.css`). Icônes `search` et `radio` ajoutées au sprite.
- **2026-11-11 · lecteur d'enceinte** — `.mp` (section 23b) : pochette ou logo de la radio, barre de lecture `.mp-seek` (on y avance) ou « En direct », titre précédent / lecture / suivant `.mp-btn`, volume sous le lecteur sans pourcentage. Résultats : titres `.track` (zone `.track-hit`, étoile `.fav`) et albums en rangée défilante `.albums` / `.album` ; onglet Favoris. Logos de radios `.station-logo.-img`. Icônes `skip-back`, `skip-forward`, `star`, `volume`, `volume-low`, `disc`.
- **2026-11-12 · retouches Figma** — rail : Réglages (`settings`, nouvelle feuille : alertes sur cet appareil, apparence, personnes et visages), puis alertes, veille, avatar ; la feuille Alertes ne garde que les alertes. Accueil : l'heure après la date, plus de pastille de présence ; Chauffage : modes Planning / Nuit / Boost en petites pastilles à icône (`calendar`, `moon`, `rocket`), plus d'« Arrêt » ; humidité de la maison sous la moyenne (au lieu de la puissance de chauffe). Thermostat d'une pièce sans pied de carte. Tuiles des pièces sans indicateur. Sécurité : mur de caméras sur toute la largeur, puis Alarme | Ouvrants (liste) | Mouvement, puis Journal. Icônes `settings`, `rocket`.
- **2026-11-12 · accueil, sécurité, médias** — Accueil sans caméra en grand ni carte « Allumées » : tuiles Alarme / Lumières / Caméras (la tuile ouvre Sécurité ; activer = mode privé coupé et enregistrement, couper = mode privé sans enregistrement), puis Agenda (toute la carte ouvre l'agenda) et Ouvrants côte à côte. Chauffage : bouton power à la place de la flamme (couper / relancer le planning), pastilles de mode qui prennent leur couleur une fois choisies (`.chip.-mode` : planning vert, nuit violet `--atmo-6`, boost accent), consignes en vert au planning et en violet en mode nuit (`.pair.is-schedule`, `.pair.is-night`). Sécurité : le journal s'ouvre dans une fenêtre depuis une ligne (`.sec-journal-btn`). Nouvelle page Médias (`/medias`) : le lecteur, puis Radio | Recherche | Favoris en colonnes (`.media-cols`).

- **2026-11-08 · app étape 1** — portage React (`packages/ui`, `apps/web`) et serveur Go (`server/`) : accueil, pièces, sécurité, météo en lecture temps réel (SSE), mode démo, PWA. Captures vérifiées à 390 et 1440 px sans débordement ni erreur console. Section 6c ci-dessus ; le point 5 du « à faire » est fait (sans Storybook, le catalogue reste la doc vivante).

- **2026-11-08 · v2.5** — qualité de l'air ajoutée à la page Météo (indice ATMO, polluants, pollens, pilules, tuile, alerte, états) ; icônes `air` et `flower`.

- **2026-11-08 · v2.4** — page Météo : 15 glyphes jour / nuit en 5 tailles, 19 icônes trait ajoutées au sprite, `weather.css` (en-tête, pilules, détails, boussole, UV, pression, pluie dans l'heure, horaire + courbe, journalier + jour ouvert, soleil, vigilance, dedans / dehors, tuiles, états), `widgets.js` étendu (glyphes, courbes). Les anciens glyphes CSS de la v2 sont remplacés partout par `data-cond`.

- **2026-11-08 · v2.3** — menu réduit à trois pages : System (fondations, widgets, référence), Mobile (écrans iPhone), Dashboard.

- **2026-11-08 · v2.2** — catalogue découpé en fichiers (`catalogue.css/js`, `widgets.js`, `dashboard.css`), menu haut commun, dashboard sorti sur sa propre page et enrichi : rangée d'alertes, prévisions à 3 jours, puissance de chauffe, pied « présence tado », tuiles sur une rangée de quatre, cartes de pièce en 3 × 2 au centre, liste des lumières allumées sous l'arc, badge de compteur sur le rail ; arc recadré (plus de chevauchement avec ce qui suit) ; trois paliers responsive.

- **2026-11-08 · v2.1** — dashboard grand écran ajouté au catalogue : rail d'icônes, barre de pièces centrée, grille 320 / 1fr / 300 en zones nommées (météo · héros 21:8 avec lecteur · chauffage · appareils · tuiles · sécurité + lumières). Mêmes widgets, aucun nouveau composant ; le cadre déborde de la colonne du catalogue sur grand écran et défile horizontalement sur téléphone.

- **2026-11-08 · v0.1** — tokens clair/sombre (ivoire + bleu nuit accent + terracotta chaleur), 13 widgets sobres. Rejetée le jour même (« pas très joli, trop simple »).
- **2026-11-08 · v2** — refonte « ambiance » : nuit + verre + glow terracotta, Outfit/Inter, rayons 14/22/30, 20 widgets dont pilule, bascule, carte héros, carte appareil, tuile, liste, lecteur, barre de pièces, jauges à points et à arc ; deux écrans composés en tête de catalogue. Vérifié à 390 et 1280 px dans les deux thèmes, sans débordement ni erreur console (hors chargement des polices Google, bloqué dans l'environnement de rendu).

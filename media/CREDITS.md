# Photos

Les photos de ce dossier sont des images libres de droits de **Pexels** (licence Pexels : usage libre, modification permise, attribution non requise), choisies en attendant les photos de la maison. Elles sont servies par le serveur sous `/media/` (`serve.media_dir` dans `maison.yaml`, ou `MAISON_MEDIA`).

Pour les remplacer : déposer tes propres photos sous le même nom (JPEG, paysage ≈ 3:2, 1200 px de large suffit pour les pièces ; carré 640 px pour les appareils). Aucun redémarrage nécessaire.

## Pièces (`rooms/`)

| Fichier | Pexels |
| --- | --- |
| salon.jpg | https://www.pexels.com/photo/36363111/ |
| cuisine.jpg | https://www.pexels.com/photo/33309972/ |
| entree.jpg | https://www.pexels.com/photo/30739543/ (assombrie) |
| bureau.jpg | https://www.pexels.com/photo/11148009/ |
| chambre.jpg | https://www.pexels.com/photo/10217037/ |
| bebe.jpg | https://www.pexels.com/photo/7938053/ |

## Appareils (`devices/`)

L'interface choisit l'image par type d'appareil (`image:` sur un appareil de `maison.yaml` la remplace).

| Fichier | Type | Pexels |
| --- | --- | --- |
| bulb-on.jpg | lumière allumée | https://www.pexels.com/photo/577513/ |
| bulb-off.jpg | lumière éteinte | https://www.pexels.com/photo/20766488/ |
| speaker.jpg | média | https://www.pexels.com/photo/14309807/ |
| blind.jpg | volet / store | https://www.pexels.com/photo/72579/ |
| window.jpg | ouvrant | https://www.pexels.com/photo/13251396/ |
| plug.jpg | prise | https://www.pexels.com/photo/11163465/ |
| heating.jpg | chauffage | https://www.pexels.com/photo/4819052/ |
| camera.jpg | caméra (mode privé / injoignable) | https://www.pexels.com/photo/16423102/ |
| ac.jpg | climatisation | https://www.pexels.com/photo/38788452/ |
| hue-play-on/off.jpg | barre lumineuse (Hue Play) | https://www.pexels.com/photo/13870950/ (éteinte : retouche) |
| led-strip-on/off.jpg | ruban LED | https://www.pexels.com/photo/10482375/ (désaturé ; éteint : retouche) |
| lamp-on/off.jpg | lampe à abat-jour | https://www.pexels.com/photo/36650038/ (éteinte : retouche) |
| spots-on/off.jpg | lampadaire à spots | https://www.pexels.com/photo/11919044/ (éteint : retouche) |
| spotlight-on/off.jpg | spot orientable | https://www.pexels.com/photo/30315596/ (éteint : retouche) |

Les variantes « éteint » des lampes sont dérivées de la photo allumée (couleur drainée, lumière écrasée) : une vraie photo de l'appareil éteint, sous le même nom, fera mieux. Dans `maison.yaml`, un appareil prend `image:` (allumé) et `image_off:` (éteint).

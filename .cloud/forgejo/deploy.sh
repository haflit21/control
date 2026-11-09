#!/bin/sh
# Déploie $TAG sur la stack $STACK par le moteur vérifié de Sentinel
# (deploy-self : nouvelle image, conteneur sain, $CHECK_URL répond, sinon
# retour automatique à l'image précédente), lancé depuis la dernière image
# Sentinel du registre.
#
# Première fois (la stack n'existe pas, ou forge-ci ne la voit pas) : demande
# à Sentinel de créer le service (POST $SENTINEL_URL/api/ci/services/$APP).
# Sentinel juge .cloud/ avec ses garde-fous : conforme → il crée la stack, l'ouvre
# à forge-ci, vérifie le service et ajoute $HOSTNAME dans Pi-hole, et ce script
# attend la fin ; à vérifier → proposition à approuver dans Sentinel (succès
# ici, le service arrive après l'approbation) ; refusé → échec avec la raison.
set -eu
cd "$(dirname "$0")/../.."
. .cloud/forgejo/app.env
: "${REGISTRY:?}" "${TAG:?}" "${PORTAINER_URL:?}" "${PORTAINER_API_KEY:?}"
ST=$(curl -fsS "http://$REGISTRY/v2/sentinel/tags/list" | node -e '
  let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{
    const k=x=>x.slice(0,13);
    const t=(JSON.parse(s).tags||[]).filter(x=>/^\d{8}-\d{4}(-|$)/.test(x)).sort((a,b)=>k(a)===k(b)?(a<b?-1:1):(k(a)<k(b)?-1:1));
    console.log(t.pop()||"")})')
[ -n "$ST" ] || { echo "aucune image Sentinel dans $REGISTRY"; exit 1; }
echo "moteur de déploiement : sentinel:$ST"
export STACK CONTAINER CHECK_URL

# sentinel 'commande' : la commande de Sentinel, avec le compte forge-ci
sentinel() {
  docker run --rm -e PORTAINER_URL -e PORTAINER_API_KEY -e TAG -e STACK -e CONTAINER -e CHECK_URL \
    --entrypoint sh "$REGISTRY/sentinel:$ST" -c '
    printf "portainer:\n  enabled: true\n  url: %s\n  api_key: %s\n  insecure_tls: true\n" "$PORTAINER_URL" "$PORTAINER_API_KEY" > /tmp/ci.yaml
    /app/sentinel --config /tmp/ci.yaml '"$1"
}

if sentinel 'portainer stacks' | grep -q "^$STACK "; then
  sentinel 'deploy-self --stack "$STACK" --container "$CONTAINER" --tag "$TAG" --no-config --check "$CHECK_URL"'
  exit 0
fi

echo "La stack « $STACK » n'existe pas encore (ou forge-ci n'y a pas accès)."
if [ -z "${SENTINEL_URL:-}" ] || [ -z "${SENTINEL_CI_TOKEN:-}" ]; then
  echo "SENTINEL_URL ou SENTINEL_CI_TOKEN absent : crée-la depuis .cloud/portainer-stack.yml avec le tag $TAG, ouvre-la à forge-ci, puis relance ce workflow."
  exit 0
fi

# field NOM < json : un champ de la réponse de Sentinel
field() { node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>{let v;try{v=JSON.parse(s)[process.argv[1]]}catch(e){}
  if(Array.isArray(v))v=v.join("\n");console.log(v==null?"":String(v))})' "$1"; }
api() { curl -sS -H "Authorization: Bearer $SENTINEL_CI_TOKEN" "$@"; }

echo "Demande de création à Sentinel ($SENTINEL_URL)…"
R=$(api -X POST "$SENTINEL_URL/api/ci/services/$APP")
STATUS=$(echo "$R" | field status)
JOB=$(echo "$R" | field job)
NOTE=$(echo "$R" | field note)
[ -z "$NOTE" ] || echo "$NOTE"
case "$STATUS" in
  exists)
    echo "Sentinel voit déjà la stack « $STACK » : l'ouvrir à forge-ci (Portainer > Access control > Restricted), puis relancer."
    exit 0 ;;
  refused)
    echo "Refusé par les garde-fous de Sentinel : $(echo "$R" | field reason)"
    echo "Corriger .cloud/ (app.env, portainer-stack.yml) puis pousser."
    exit 1 ;;
  proposed)
    echo "Le service demande une vérification : proposition $JOB à approuver dans Sentinel (Claude > Actions en attente)."
    echo "Après l'approbation, Sentinel crée le service ; les push suivants le déploient."
    exit 0 ;;
  scheduled|running) ;;
  *)
    echo "Réponse inattendue de Sentinel : $R"
    exit 1 ;;
esac

echo "Création en cours (job $JOB, image $(echo "$R" | field tag))…"
i=0
while [ $i -lt 180 ]; do   # 15 min
  sleep 5
  i=$((i + 1))
  J=$(api "$SENTINEL_URL/api/ci/jobs/$JOB") || continue
  S=$(echo "$J" | field status)
  case "$S" in
    done)
      echo "$J" | field log
      echo "Service créé : $(echo "$J" | field message)"
      exit 0 ;;
    failed|cancelled|rejected)
      echo "$J" | field log
      echo "Création en échec : $(echo "$J" | field message)"
      exit 1 ;;
  esac
done
echo "Toujours pas terminé après 15 min : voir le job $JOB dans Sentinel."
exit 1

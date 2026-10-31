#!/usr/bin/env bash
# Mise à jour de Nodra : ./update.sh [version] (dernière release par défaut).
# Lancée à la main ou par la mise à jour automatique (tâche sur le serveur localhost).
# Les migrations de la base s'appliquent au démarrage du nouveau conteneur.
set -euo pipefail

REPO="Murathan-Aydin/nodra"
SRC="$(cd "$(dirname "$0")" && pwd)"
ENV="$SRC/.env"
[ -f "$ENV" ] || { echo "Fichier $ENV introuvable : lancez install.sh" >&2; exit 1; }

VERSION="${1:-}"
if [ -z "$VERSION" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)
fi
case "$VERSION" in
  v[0-9]*|edge|latest) ;;
  *) echo "Version invalide : $VERSION" >&2; exit 1 ;;
esac

echo "$(date -u +%FT%TZ) mise à jour vers $VERSION"
sed -i "s/^NODRA_VERSION=.*/NODRA_VERSION=$VERSION/" "$ENV"
cd "$SRC"
docker compose pull --quiet nodra
docker compose up -d --remove-orphans
for _ in $(seq 1 60); do
  if [ "$(docker inspect -f '{{.State.Health.Status}}' nodra 2>/dev/null)" = "healthy" ]; then
    echo "$(date -u +%FT%TZ) Nodra $VERSION démarré"
    exit 0
  fi
  sleep 2
done
echo "Nodra ne répond pas après la mise à jour : docker compose -f $SRC/docker-compose.yml logs nodra" >&2
exit 1

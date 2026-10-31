#!/usr/bin/env bash
# Installation de Nodra sur un serveur Linux (root) :
#   curl -fsSL https://raw.githubusercontent.com/Murathan-Aydin/nodra/main/install/install.sh | bash
# Variables facultatives : NODRA_VERSION (ex. v1.0.0, sinon la dernière release), NODRA_PORT (8000),
# NODRA_DIR (/data/nodra), NODRA_BRANCH (fichiers d'installation, main), NODRA_SKIP_DOCKER=1.
# Relancer le script met à jour les fichiers sans toucher aux secrets existants.
set -euo pipefail

REPO="Murathan-Aydin/nodra"
BRANCH="${NODRA_BRANCH:-main}"
DIR="${NODRA_DIR:-/data/nodra}"
SRC="$DIR/source"
PORT="${NODRA_PORT:-8000}"
RAW="https://raw.githubusercontent.com/$REPO/$BRANCH/install"

log() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31mErreur :\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "lancez ce script en root (sudo)."
command -v curl >/dev/null || die "curl est requis."

# --- Docker ---
if ! command -v docker >/dev/null 2>&1; then
  [ "${NODRA_SKIP_DOCKER:-}" = "1" ] && die "Docker est absent (NODRA_SKIP_DOCKER=1)."
  log "Installation de Docker (get.docker.com)"
  curl -fsSL https://get.docker.com | sh
fi
docker compose version >/dev/null 2>&1 || die "le plugin docker compose est requis."
systemctl enable --now docker >/dev/null 2>&1 || true

# --- Version ---
if [ -z "${NODRA_VERSION:-}" ]; then
  NODRA_VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null \
    | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1 || true)
  NODRA_VERSION="${NODRA_VERSION:-edge}"
fi
log "Version : $NODRA_VERSION"

# --- Fichiers ---
mkdir -p "$SRC/ssh"
chmod 700 "$SRC"
for f in docker-compose.yml update.sh; do
  if [ -n "${NODRA_LOCAL_FILES:-}" ]; then # tests : fichiers du dépôt local
    cp "$NODRA_LOCAL_FILES/$f" "$SRC/$f"
  else
    curl -fsSL "$RAW/$f" -o "$SRC/$f"
  fi
done
chmod +x "$SRC/update.sh"

rand_b64() { head -c 32 /dev/urandom | base64 | tr -d '\n'; }
rand_hex() { head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n'; }
public_ip() {
  curl -fsS4 --max-time 5 https://ifconfig.me 2>/dev/null || hostname -I 2>/dev/null | awk '{print $1}' || echo localhost
}

ENV="$SRC/.env"
if [ ! -f "$ENV" ]; then
  log "Génération des secrets ($ENV)"
  umask 077
  cat > "$ENV" <<CONF
NODRA_VERSION=$NODRA_VERSION
NODRA_PORT=$PORT
NODRA_APP_KEY=$(rand_b64)
POSTGRES_PASSWORD=$(rand_hex)
NODRA_PUBLIC_URL=http://$(public_ip):$PORT
NODRA_SERVER_DATA_DIR=$DIR
CONF
else
  sed -i "s/^NODRA_VERSION=.*/NODRA_VERSION=$NODRA_VERSION/" "$ENV"
fi
chmod 600 "$ENV"

# --- Clé SSH du serveur localhost (Nodra pilote l'hôte comme les autres serveurs) ---
KEY="$SRC/ssh/id_ed25519"
if [ ! -f "$KEY" ]; then
  log "Clé SSH du serveur localhost"
  ssh-keygen -q -t ed25519 -N "" -C "nodra-localhost" -f "$KEY"
fi
mkdir -p /root/.ssh && chmod 700 /root/.ssh
touch /root/.ssh/authorized_keys && chmod 600 /root/.ssh/authorized_keys
grep -qF "$(cut -d' ' -f2 "$KEY.pub")" /root/.ssh/authorized_keys || cat "$KEY.pub" >> /root/.ssh/authorized_keys
chown 65532:65532 "$KEY" && chmod 400 "$KEY" # utilisateur de l'image (distroless nonroot)

# --- Réseau partagé avec le proxy ---
docker network inspect nodra >/dev/null 2>&1 || docker network create --attachable --label nodra.managed=true nodra >/dev/null

# --- Démarrage ---
log "Démarrage de Nodra"
cd "$SRC"
docker compose pull --quiet
docker compose up -d --remove-orphans
for _ in $(seq 1 60); do
  if [ "$(docker inspect -f '{{.State.Health.Status}}' nodra 2>/dev/null)" = "healthy" ]; then
    url=$(sed -n 's/^NODRA_PUBLIC_URL=//p' "$ENV")
    log "Nodra est prêt : $url (le premier compte créé devient administrateur)."
    exit 0
  fi
  sleep 2
done
die "Nodra n'a pas démarré : docker compose -f $SRC/docker-compose.yml logs nodra"

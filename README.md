# Nodra

Nodra est une plateforme auto-hébergée (PaaS) pour déployer applications, bases de données et services sur vos propres serveurs : backend en **Go**, interface en **React**.

## Inspiration

Nodra est une refonte en Go et React inspirée de [Coolify](https://github.com/coollabsio/coolify) (Andras Bacsai et contributeurs). C'est Coolify qui m'a donné l'idée de ce projet. Coolify est distribué sous licence Apache-2.0.

Nodra va plus loin sur la gestion d'équipes : rôles personnalisés, permissions par ressource, sous-équipes, journal d'audit et approbations de déploiement.

## Installation (serveur Linux)

En root, sur une machine Linux avec un accès Internet :

```bash
curl -fsSL https://raw.githubusercontent.com/Murathan-Aydin/nodra/main/install/install.sh | bash
```

Le script installe Docker si besoin et prépare `/data/nodra/source` : `docker-compose.yml`, secrets générés dans `.env`, clé SSH du serveur « localhost ». Il lance ensuite Nodra et PostgreSQL, puis affiche l'adresse (`http://IP:8000`). Le premier compte créé devient administrateur de l'instance, et l'hôte apparaît comme serveur `localhost` dans son équipe.

Variables facultatives : `NODRA_VERSION` (par défaut, la dernière release), `NODRA_PORT` (8000), `NODRA_DIR` (`/data/nodra`).

Une fois le domaine de l'instance réglé (`PATCH /api/v1/admin/settings` `{fqdn}`), l'interface est servie en HTTPS par le proxy du serveur localhost.

Mise à jour : `/data/nodra/source/update.sh [vX.Y.Z]`. Elle peut aussi se faire depuis l'interface (`POST /api/v1/admin/update`) ou automatiquement, selon les réglages de l'instance.

Images : `ghcr.io/murathan-aydin/nodra:<version>`, `latest` et `edge` (branche `main`). Elles sont publiées par `.github/workflows/release.yml`, et un tag `vX.Y.Z` crée aussi la release GitHub.

## Démarrage (développement)

```bash
make up-dev                                      # PostgreSQL sur le port 5433
cp .env.example .env                            # renseigner NODRA_APP_KEY (openssl rand -base64 32)
set -a; . ./.env; set +a
go run ./cmd/nodra serve                        # http://localhost:8000
```

Front (`web/`, React + Vite + TanStack Router/Query + Tailwind) : dans un second terminal, `npm --prefix web install` puis `npm --prefix web run dev` → http://localhost:5173 (proxy `/api` vers `:8000`, WebSocket compris). `npm --prefix web run build` produit `web/dist`, embarqué dans le binaire Go (le dossier doit exister pour compiler : `mkdir -p web/dist` suffit sans front).

Commandes : `nodra serve` (applique les migrations puis démarre), `nodra migrate`, `nodra version`, `nodra healthcheck`.

## Documentation

- [docs/PLAN.md](docs/PLAN.md) : plan complet
- [docs/PROGRESS.md](docs/PROGRESS.md) : état d'avancement
- [docs/API.md](docs/API.md) : contrat de l'API

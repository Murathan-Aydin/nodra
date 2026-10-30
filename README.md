# Nodra

Nodra est une plateforme auto-hébergée (PaaS) pour déployer applications, bases de données et services sur vos propres serveurs : backend en **Go**, interface en **React**.

## Inspiration

Nodra est une refonte en Go et React inspirée de [Coolify](https://github.com/coollabsio/coolify) (Andras Bacsai et contributeurs). C'est Coolify qui m'a donné l'idée de ce projet. Coolify est distribué sous licence Apache-2.0.

Nodra va plus loin sur la gestion d'équipes : rôles personnalisés, permissions par ressource, sous-équipes, journal d'audit et approbations de déploiement.

## Démarrage (développement)

```bash
make up-dev                                      # PostgreSQL sur le port 5433
cp .env.example .env                            # renseigner NODRA_APP_KEY (openssl rand -base64 32)
set -a; . ./.env; set +a
go run ./cmd/nodra serve                        # http://localhost:8000
```

`make up` sera utilisé avec la configuration Docker Compose standard lorsqu'elle sera ajoutée.

Commandes : `nodra serve` (applique les migrations puis démarre), `nodra migrate`.

## Documentation

- [docs/PLAN.md](docs/PLAN.md) : plan complet
- [docs/PROGRESS.md](docs/PROGRESS.md) : état d'avancement
- [docs/API.md](docs/API.md) : contrat de l'API

# Plan Nodra — refonte de Coolify en Go (backend) + React (front)

## Contexte
Nodra est un PaaS auto-hébergé inspiré de Coolify (`~/dev/coolify`, Laravel/Livewire). Objectif : refaire toutes les fonctionnalités de la **version open source self-hosted** de Coolify, backend Go d'abord, puis front React. On repart de zéro (`~/dev/nodra` est vide). La gestion d'équipes doit aller plus loin que Coolify (RBAC, groupes, permissions par ressource, audit, approbations).

Hors périmètre : Stripe/mode cloud, création de serveurs chez des providers cloud, GitHub runners, DNS managé, Docker Swarm. Pas de Swagger (contrat dans `docs/API.md`).

## Stack
- **Backend** : Go 1.23+, `chi` (HTTP), `pgx` + `sqlc` (requêtes typées), `goose` (migrations), **PostgreSQL**, **River** (jobs dans Postgres, pas de Redis), `golang.org/x/crypto/ssh` (exécution distante), client Docker via SSH, WebSocket (`coder/websocket`) pour logs/terminal/statuts temps réel, chiffrement AES-GCM des secrets.
- **Front** : React + Vite + TypeScript, TanStack Router/Query, Tailwind + shadcn/ui, xterm.js (terminal), CodeMirror (compose/env/config).
- **Livraison** : un binaire Go qui embarque le build front (`embed`), image Docker, `docker-compose.yml` d'installation, script `install.sh`.

## Arborescence
```
cmd/nodra/            main (serve, migrate, worker, admin reset-password)
internal/
  api/                handlers HTTP + middleware (auth, RBAC, audit)
  auth/               sessions, tokens API, 2FA TOTP, OAuth
  rbac/               rôles, permissions, résolution par ressource
  store/              sqlc + migrations
  ssh/                pool de connexions multiplexées, exécution de commandes
  docker/             génération compose, labels, statut conteneurs
  deploy/             pipeline de build/déploiement (Dockerfile, compose, image, static, Nixpacks)
  proxy/              Traefik/Caddy : config dynamique, certificats
  databases/          démarrage des 9 types de DB
  services/           templates one-click (parse `templates/compose` de Coolify)
  backup/             sauvegardes DB/volumes, S3
  notify/             email, Discord, Slack, Telegram, Pushover, webhook
  jobs/               workers River + planificateur cron
  realtime/           hub WebSocket
web/                  front React
docs/API.md
README.md
```

## Phase 1 — Backend (ordre d'implémentation)

1. **Socle** : config env, logs structurés (`slog`), migrations, chiffrement des secrets, healthcheck, embed du front, graceful shutdown.
2. **Auth & utilisateurs** (équiv. `app/Actions/Fortify`, `OauthSetting`, `PersonalAccessToken`) : inscription (premier user = admin d'instance), login session, reset mot de passe, vérif e-mail, 2FA TOTP + codes de secours, OAuth (GitHub, GitLab, Google…), tokens API avec scopes et expiration.
3. **Équipes avancées** (au-delà de `Team`/`TeamInvitation`/`ResourceUser`) :
   - Organisation → équipes → **sous-équipes/groupes** (hiérarchie, héritage des droits).
   - **Rôles personnalisés** : permissions atomiques (`app.deploy`, `app.env.read`, `server.terminal`, `db.backup.restore`, `team.members.manage`…), rôles système (owner, admin, developer, viewer) + rôles créés par l'équipe.
   - **Permissions par ressource** : attributions sur projet / environnement / serveur / ressource, pour un user ou un groupe ; résolution « le plus spécifique gagne », deny explicite possible.
   - **Audit** : journal immuable de toutes les actions (qui, quoi, quand, IP, avant/après), filtrable, export.
   - **Approbations** : règles par environnement (ex. prod = 1 ou N approbateurs avant déploiement/suppression), file de demandes, notifications.
   - Invitations par lien/e-mail avec rôle et portée, transfert de propriété, suppression d'équipe avec nettoyage.
   - Middleware RBAC unique appliqué à chaque route + tokens API limités à une équipe/portée.
4. **Clés SSH & serveurs** (`PrivateKey`, `Server`, `Actions/Server`) : clés (génération/import), ajout de serveur, validation (connexion, OS, Docker), installation Docker et prérequis, serveur localhost, réseaux Docker (destinations), nettoyage Docker planifié, métriques (CPU/RAM/disque) via un agent léger type Sentinel, terminal web, check stockage, mises à jour paquets, Cloudflare Tunnel, reboot.
5. **Proxy** (`Actions/Proxy`) : Traefik (défaut) et Caddy, start/stop/restart, édition de config, configs dynamiques, certificats Let's Encrypt, certificats custom, vérification DNS des domaines.
6. **Projets & environnements** (`Project`, `Environment`, tags) : CRUD, clonage d'environnement, variables partagées (équipe/projet/environnement), déplacement de ressources.
7. **Sources Git** (`GithubApp`, `GitlabApp`, `GiteaApp`) : GitHub App (création via manifest), GitLab/Gitea, dépôts publics, deploy keys, webhooks (push, PR).
8. **Applications & déploiements** (`Application`, `ApplicationDeploymentJob`, `ApplicationPreview`) : build packs Nixpacks, Dockerfile, Docker Compose, image Docker, statique ; file de déploiement par serveur, logs en streaming, annulation, rollback sur images précédentes, previews de PR (déploiement + nettoyage), healthchecks, limites ressources, volumes persistants et fichiers, variables d'env (build/runtime, preview), domaines multiples + redirections, labels custom, commandes pre/post-deploy, multi-serveurs, déploiement par webhook/API.
9. **Bases de données** (`Standalone*`, `Actions/Database`) : PostgreSQL, MySQL, MariaDB, MongoDB, Redis, KeyDB, Dragonfly, ClickHouse ; exposition publique (proxy TCP), SSL, config custom, import de dump, flush.
10. **Services one-click** (`Service`, `templates/compose`) : import et parsing des ~390 templates compose (variables magiques `SERVICE_FQDN_*`, `SERVICE_PASSWORD_*`…), édition du compose, start/stop par sous-service.
11. **Stockage S3 & sauvegardes** (`S3Storage`, `DatabaseBackupJob`, `VolumeBackupJob`) : destinations S3, sauvegardes planifiées DB et volumes, rétention locale/S3, restauration, alerte sauvegardes manquantes.
12. **Tâches planifiées** (`ScheduledTask`) : commandes cron dans les conteneurs, historique d'exécution.
13. **Notifications** (`*NotificationSettings`) : e-mail (SMTP/Resend), Discord, Slack, Telegram, Pushover, webhook ; abonnement par type d'événement, throttling.
14. **Temps réel** : hub WebSocket par équipe (statuts conteneurs, logs de déploiement, terminal), vérification périodique des statuts (`ServerCheckJob`).
15. **Instance** (`InstanceSettings`) : domaine de l'instance, inscriptions ouvertes/fermées, auto-update, changelog, SMTP par défaut.
16. **API publique** : endpoints REST couvrant tout ce qui précède (parité avec `routes/api.php`), documentés dans `docs/API.md`. Serveur MCP en option en fin de phase.

Chaque module : migrations + requêtes sqlc + service + handlers + tests (unitaires, intégration Postgres via testcontainers, SSH/Docker contre un conteneur `docker:dind`).

## Phase 2 — Front React
1. Socle : layout, thème clair/sombre, client API typé, auth (login, 2FA, OAuth), sélecteur d'organisation/équipe.
2. Dashboard (projets, serveurs, déploiements récents).
3. Serveurs : liste, assistant d'ajout, détail (métriques, proxy, terminal, nettoyage, destinations).
4. Projets → environnements → ressources : création assistée (app, DB, service), pages de détail avec onglets (général, env, domaines, stockage, déploiements, logs, backups, tâches, webhooks, danger zone).
5. Logs et déploiements en direct (WebSocket), terminal xterm.js.
6. Catalogue de services one-click avec recherche.
7. Équipes : membres, groupes, éditeur de rôles (matrice de permissions), attributions par ressource, journal d'audit, file d'approbations.
8. Paramètres : sources Git, clés, S3, notifications, tokens API, profil, instance.

## README
Présentation de Nodra, stack, installation, et une section **Remerciements / Inspiration** : « Nodra est une refonte en Go et React inspirée de [Coolify](https://github.com/coollabsio/coolify) d'Andras Bacsai, qui m'a donné l'idée de ce projet. » (+ mention de la licence Apache-2.0 de Coolify si des templates de services sont repris).

## Fichiers de référence Coolify
- Déploiement : `app/Jobs/ApplicationDeploymentJob.php`
- Bases : `app/Actions/Database/Start*.php`
- Proxy : `app/Actions/Proxy/*`, serveur : `app/Actions/Server/*`
- Services : `templates/compose/*`, `bootstrap/helpers/*` (parsing compose)
- API : `routes/api.php`, `openapi.yaml` (référence de parité uniquement)

## Vérification
- `go test ./...` + tests d'intégration (Postgres, dind) en CI GitHub Actions.
- Scénario bout en bout sur le serveur de test : ajouter un serveur, déployer une app depuis GitHub avec domaine + HTTPS, créer une DB Postgres avec backup S3, déployer un service one-click, vérifier qu'un « viewer » ne peut pas déployer et qu'un déploiement prod exige une approbation, contrôler le journal d'audit.
- Front : tests Vitest + Playwright sur les parcours principaux.

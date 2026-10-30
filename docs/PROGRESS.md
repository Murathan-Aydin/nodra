# Suivi d'avancement Nodra

> **À lire en premier par toute nouvelle session Claude.** Mettre ce fichier à jour à CHAQUE modification (fait / reste à faire / décisions).
> Plan complet : [PLAN.md](PLAN.md). Contrat API : [API.md](API.md).

## Décisions
- Repartir de zéro (2026-10-30). Uniquement la version open source auto-hébergée de Coolify (`~/dev/coolify` sert de référence) : pas de Stripe, providers cloud, GitHub runners, DNS managé, Swarm.
- PostgreSQL + River (pas de Redis). Pas de Swagger : contrat dans `docs/API.md`.
- Requêtes SQL : pgx écrit à la main (sqlc non installé) ; migrations goose embarquées (`internal/store/migrations`, numérotées `000NN_nom.sql`).
- Équipes avancées : rôles personnalisés, permissions par ressource, sous-équipes/groupes, audit, approbations.
- README crédite Coolify comme inspiration.

## Lancer / tester
```bash
docker compose -f docker-compose.dev.yml up -d
NODRA_ENV=development NODRA_DATABASE_URL='postgres://nodra:nodra@localhost:5433/nodra?sslmode=disable' \
NODRA_APP_KEY=$(openssl rand -base64 32) go run ./cmd/nodra serve
go vet ./... && go test ./...
```

## Fait
- **Étape 1 — Socle** (2026-10-30) : module Go `github.com/murathanaydin/nodra`, config par env (`internal/config`), logs JSON `slog`, chiffrement AES-256-GCM (`internal/crypto`, testé), pool pgx + migrations goose embarquées (`internal/store`, table `instance_settings`), routeur chi avec `/api/v1/health` et service du front embarqué en SPA (`web/embed.go`, placeholder `web/dist/index.html`), arrêt gracieux, `docker-compose.dev.yml`, `.env.example`, README. Vérifié : health OK et fallback SPA en local.

## Reste à faire (backend)
- [ ] 2. Auth & utilisateurs (sessions, reset mdp, vérif e-mail, 2FA TOTP, OAuth, tokens API)
- [ ] 3. Équipes avancées (orga/équipes/groupes, rôles perso, permissions par ressource, audit, approbations, invitations)
- [ ] 4. Clés SSH & serveurs
- [ ] 5. Proxy Traefik/Caddy
- [ ] 6. Projets & environnements
- [ ] 7. Sources Git
- [ ] 8. Applications & déploiements
- [ ] 9. Bases de données
- [ ] 10. Services one-click
- [ ] 11. S3 & sauvegardes
- [ ] 12. Tâches planifiées
- [ ] 13. Notifications
- [ ] 14. Temps réel (WebSocket)
- [ ] 15. Réglages d'instance
- [ ] 16. API publique complète (+ MCP optionnel)
- [ ] Dockerfile + install.sh + CI GitHub Actions

## Reste à faire (front React, phase 2)
- [ ] Socle, dashboard, serveurs, projets/ressources, logs/terminal, catalogue services, équipes, paramètres

## Prochaine action
Étape 2 : auth (tables `users`, `sessions`, `api_tokens` ; hash argon2id ; cookie de session HttpOnly).

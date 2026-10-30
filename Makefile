.PHONY: up up-dev down down-dev

COMPOSE ?= docker compose
COMPOSE_FILE ?= docker-compose.yml
DEV_COMPOSE_FILE ?= docker-compose.dev.yml

up:
	$(COMPOSE) -f $(COMPOSE_FILE) up -d

up-dev:
	$(COMPOSE) -f $(DEV_COMPOSE_FILE) up -d

down:
	$(COMPOSE) -f $(COMPOSE_FILE) down

down-dev:
	$(COMPOSE) -f $(DEV_COMPOSE_FILE) down

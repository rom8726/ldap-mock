COMPOSE_FILE := dev/docker-compose.yml

.PHONY: compose-up compose-down

compose-up:
	docker compose -f $(COMPOSE_FILE) up --build -d

compose-down:
	docker compose -f $(COMPOSE_FILE) down

.PHONY: up down test seed generate
up:
	docker compose up --build
down:
	docker compose down
test:
	cd server && go test ./...
	cd web && npm run build
	cd web && npm run test:e2e
seed:
	docker compose exec api /app/pac seed
generate:
	cd server && sqlc generate

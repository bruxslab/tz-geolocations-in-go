.PHONY: build test race vet fmt-check validate check verify db migrate import up down
build:
	go build ./cmd/...
test:
	go test ./...
race:
	CGO_ENABLED=1 go test -race ./...
vet:
	go vet ./...
fmt-check:
	@test -z "$$(gofmt -l .)" || (gofmt -l .; exit 1)
validate:
	go run ./cmd/validate
check: fmt-check vet test race validate
verify:
	docker compose run --rm --build test
db:
	docker compose up -d --wait postgres
migrate:
	docker compose run --rm --build migrate
import:
	docker compose run --rm --build import
up:
	docker compose up -d --build api
down:
	docker compose down

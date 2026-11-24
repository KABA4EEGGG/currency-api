# Основные команды разработки currency-api.
GO ?= go

IMAGE_NAME ?= currency-api
IMAGE_TAG  ?= dev
REGISTRY   ?= ghcr.io/kseno

.PHONY: help build test lint run mock docker-build compose-up compose-down \
        helm-lint helm-install helm-upgrade helm-uninstall helm-rollback

help:
	@echo "make build        — собрать бинарники (server, mockfx)"
	@echo "make test         — unit-тесты"
	@echo "make lint         — golangci-lint"
	@echo "make run          — запустить сервер локально"
	@echo "make mock         — запустить заглушку внешнего API"
	@echo "make docker-build — собрать OCI-образ (multi-stage)"
	@echo "make compose-up   — поднять stack локально (docker compose)"
	@echo "make compose-down — остановить stack"
	@echo "make helm-install — установить чарт в k8s (namespace currency)"
	@echo "make helm-upgrade — обновить релиз"
	@echo "make helm-rollback — откатить релиз на предыдущую ревизию"

build:
	$(GO) build -o bin/server ./cmd/server
	$(GO) build -o bin/mockfx ./cmd/mockfx

test:
	$(GO) test ./...

lint:
	golangci-lint run

run: build
	FX_DATABASE_URL=postgres://currency:currency@localhost:5432/currency?sslmode=disable ./bin/server

mock: build
	./bin/mockfx

docker-build:
	docker build -f deploy/docker/Dockerfile -t $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) .

compose-up:
	docker compose -f deploy/docker-compose.yml up --build -d
	@echo "Swagger UI: http://localhost:8081  API: http://localhost:8080"

compose-down:
	docker compose -f deploy/docker-compose.yml down -v

NAMESPACE   ?= currency
RELEASE     ?= currency-api

helm-lint:
	helm lint deploy/k8s/currency

helm-install:
	helm upgrade --install $(RELEASE) deploy/k8s/currency \
		--namespace $(NAMESPACE) --create-namespace \
		--set image.tag=$(IMAGE_TAG)

helm-upgrade: helm-install

helm-uninstall:
	helm uninstall $(RELEASE) --namespace $(NAMESPACE)

helm-rollback:
	helm rollback $(RELEASE) $(REV) --namespace $(NAMESPACE)
	@echo "Список ревизий: helm history $(RELEASE) -n $(NAMESPACE)"

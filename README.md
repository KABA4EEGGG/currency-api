# Currency Exchange API

Бэкенд веб-сервиса «Курсы валют и конвертер» на Go (проектная работа МИРЭА).

Сервис получает курсы валют от внешнего бесплатного API
[open.er-api.com](https://www.exchangerate-api.com/docs/open) (без ключа и
регистрации), кэширует их в памяти, выполняет конвертации с сохранением
истории в PostgreSQL и ведёт список избранных валютных пар.

## Функциональность

| Метод | Назначение | Сторонний API / БД |
|---|---|---|
| `GET /api/v1/rates/{base}` | курсы валют относительно базовой | внешний API (через кэш в памяти) |
| `POST /api/v1/convert` | конвертация суммы + запись в историю | **INSERT в PostgreSQL** |
| `GET /api/v1/conversions` | последние конвертации | SELECT из PostgreSQL |
| `GET /api/v1/stats` | топ валютных пар | SQL-агрегация (GROUP BY) |
| `POST/GET/DELETE /api/v1/favorites` | избранные пары | PostgreSQL |
| `GET /healthz`, `GET /readyz` | liveness / readiness | readiness проверяет БД |
| `GET /metrics` | метрики Prometheus | — |

Документация API — OpenAPI 3.0 (`api/openapi.yaml`), просмотр через Swagger UI
(контейнер в docker-compose, порт `8081`).

## Архитектура

```
                    ┌──────────────────────────────────────────────┐
                    │                k3s (cluster)                 │
   k6 (контейнер)   │  ┌──────────┐   ┌──────────────┐             │
 ──── load-test ────┼─▶│currency  │──▶│ PostgreSQL   │             │
                    │  │(HPA 1..4)│   │ (PVC)        │             │
                    │  └────┬─────┘   └──────────────┘             │
   GitHub-hosted    │       │ кэш 1ч                               │
   runner: lint/    │       ▼                                      │
   test/build ──▶ GHCR: ghcr.io/<user>/currency-api:<sha>          │
                    │  ┌──────────┐                               │
                    │  │ mockfx   │◀── FX_EXCHANGE_API_URL        │
                    │  │(заглушка)│    на время нагрузочного теста│
                    │  └──────────┘                               │
                    │  Prometheus (ServiceMonitor) ◀── /metrics   │
                    │  Grafana ◀── дашборд Currency Exchange API  │
                    └──────────────────────────────────────────────┘
```

- Сборка образа — на GitHub-hosted runner (облачная ВМ GitHub), деплой — на
  self-hosted runner рядом с k3s: **две разные машины** (Б2).
- Образ публикуется в GHCR с тегом SHA коммита; деплой-машина образы не
  собирает (В2).

## Соответствие требованиям задания

| Требование | Как реализовано |
|---|---|
| А1. Git | этот репозиторий (GitHub) |
| А2. БД | PostgreSQL 17 (compose и чарт) |
| А3. docker-compose | `deploy/docker-compose.yml`: server + postgres + mock + swagger-ui |
| А4. OpenAPI | `api/openapi.yaml` + Swagger UI |
| А5. Секреты | `.env.example` в репозитории, `.env` в `.gitignore`; в k8s — Secret чарта |
| Б1. CI/CD | GitHub Actions (см. оговорки ниже) |
| Б2. Две машины | lint/test/build — GitHub-hosted runner; deploy/smoke/load — self-hosted runner |
| Б3. Стадии | `lint → test → build-push → deploy → smoke → load-test`, needs-граф останавливает пайплайн |
| Б4. Секреты CI | GitHub Actions Secrets (`DB_PASSWORD`), образ пушится встроенным `GITHUB_TOKEN` |
| Б5. Healthcheck + smoke | healthcheck в compose (`/healthprobe`), `depends_on: service_healthy`; после деплоя smoke `/healthz`, `/readyz`, контрольная конвертация |
| Б6. Мониторинг | kube-prometheus-stack + ServiceMonitor (`/metrics`), дашборд `deploy/grafana/currency-api-dashboard.json` |
| В1. Multi-stage + non-root | `deploy/docker/Dockerfile`: golang:alpine → distroless `nonroot` |
| В2. Реестр + откат | GHCR, тег `:sha` и `:latest`; `helm rollback` (см. ниже) |
| В3. Нагрузочное тестирование | k6 (см. оговорки ниже), `test/load/` |
| В4. Модуль 10 (Kubernetes = 2 модуля) | k3s + Helm-чарт + HPA (CPU 60%, 1→4), автоскейлинг под k6, docker-compose сохранён |

### Оговорки (согласованы)

1. **GitHub Actions вместо GitLab CI/Jenkins** и **k6 вместо JMeter** —
   замены согласованы с преподавателем; все пункты задания (стадии, секреты,
   smoke, пороги ошибок/p95, HTML-отчёт артефактом, параметры теста,
   генератор нагрузки вне тестируемого окружения) эквивалентно покрыты.
2. **Генератор нагрузки**: отдельной физической машины нет — k6 запускается в
   изолированном контейнере `grafana/k6` (сеть host), тестируемый сервис — в
   кластере. Сборка при этом выполняется в облаке GitHub. При появлении VPS
   k6-задача переносится на него без изменения тестов (только `BASE_URL`).

## Локальный запуск (docker compose)

```bash
cp .env.example .env            # при необходимости изменить
docker compose -f deploy/docker-compose.yml up --build -d
# API:      http://localhost:8080
# Swagger:  http://localhost:8081
# Mock API: http://localhost:8090
```

Проверка:

```bash
curl -s http://localhost:8080/healthz
curl -s -X POST http://localhost:8080/api/v1/convert \
  -H 'Content-Type: application/json' \
  -d '{"from":"USD","to":"EUR","amount":100}'
curl -s http://localhost:8080/api/v1/stats
```

Нагрузочный тест локально (на заглушке — внешний API не задействуется):

```bash
docker compose -f deploy/docker-compose.yml down server
FX_EXCHANGE_API_URL=http://mock:8090 docker compose -f deploy/docker-compose.yml up -d server
k6 run -e BASE_URL=http://localhost:8080 test/load/currency-load-test.js
```

## CI/CD (GitHub Actions)

Пайплайн: `.github/workflows/ci-cd.yml`, запускается на push в `main`.

### Настройка один раз

1. **Секреты репозитория** (Settings → Secrets and variables → Actions):
   - `DB_PASSWORD` — пароль PostgreSQL для чарта (придумайте и сохраните).
2. **Self-hosted runner** (Settings → Actions → Runners → New self-hosted runner):
   ```bash
   mkdir ~/actions-runner && cd ~/actions-runner
   # скачать и распаковать раннер по инструкции GitHub, затем:
   ./config.sh --url https://github.com/<user>/<repo> --token <TOKEN> --labels self-hosted
   sudo ./svc.sh install kura   # или запуск вручную: ./run.sh
   sudo ./svc.sh start
   ```
   Требования к хосту раннера: `helm` и `kubectl` с доступом к k3s
   (`~/.local/bin` добавляется в PATH job'ом), `docker` (нужен для k6).
3. **Публичность образа**: после первого успешного пуша открыть
   `https://github.com/users/<user>/packages/container/currency-api/settings`
   и переключить пакет в **Public**, чтобы k3s тянул образ без pull-secret.

### Откат на предыдущую версию (В2)

```bash
helm history currency-api -n currency          # выбрать предыдущую ревизию
helm rollback currency-api <РЕВИЗИЯ> -n currency
# либо: kubectl -n currency rollout undo deploy/currency
```

### Механика стадий

| Job | Runner | Что делает |
|---|---|---|
| `lint` | GitHub-hosted | `golangci-lint run` |
| `test` | GitHub-hosted | `go test ./... -race` |
| `build-push` | GitHub-hosted | docker build (multi-stage) → `ghcr.io/<user>/currency-api:<sha>` + `:latest` |
| `deploy` | self-hosted | `helm upgrade --install --atomic` с тегом SHA |
| `smoke` | self-hosted | `/healthz`, `/readyz`, контрольный POST `/convert` |
| `load-test` | self-hosted | k6 в контейнере, thresholds, HTML-отчёт артефактом |

Пороги k6 (обоснование SLO: интерактивный REST-метод — не более ~300 мс на
p95; чтение из БД дешевле записи): ошибки < 1%, p95 конвертации < 300 мс,
p95 чтения < 250 мс, p95 кэша курсов < 100 мс. Нарушение любого порога
падает job `load-test`.

## Kubernetes (Helm, HPA)

```bash
helm upgrade --install currency-api deploy/k8s/currency \
  --namespace currency --create-namespace \
  --set image.tag=<TAG> \
  --set database.password=<ПАРОЛЬ> \
  --set externalApi.url='http://currency-mock:8090' \
  --set mock.enabled=true

kubectl -n currency get hpa -w        # наблюдение автоскейлинга
```

Сервис доступен через NodePort `http://<узел>:30080`.

Демонстрация автоскейлинга (модуль 10): запустить k6, наблюдать рост реплик
с 1 до 4 по CPU (target 60%), затем спад.

## Мониторинг (Б6)

1. ServiceMonitor ставится чартом; Prometheus (kube-prometheus-stack)
   подхватывает его автоматически (селекторы пустые — берёт все).
2. Дашборд: Grafana → Dashboards → Import →
   `deploy/grafana/currency-api-dashboard.json`.
3. Ключевые метрики: `http_requests_total`, `http_request_duration_seconds`,
   `fx_cache_hits_total` / `fx_cache_misses_total`, `db_pool_open_connections`.

## Разработка

```bash
make lint          # golangci-lint
make test          # go test ./...
make build         # бинарники в ./bin
make docker-build  # OCI-образ
make helm-lint     # проверка чарта
```

Слой кода: `internal/api` (HTTP) → `internal/service` (логика) →
`internal/store` (pgx) + `internal/fx` (внешний API + кэш + singleflight);
миграции встроены в бинарник (`migrations/`, golang-migrate).

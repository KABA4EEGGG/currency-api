# Multi-stage сборка (требование В1): тяжёлый компилятор остаётся в
# builder-стадии, финальный образ — distroless, запуск от непривилегированного
# пользователя nonroot.
FROM golang:1.27-alpine AS builder

ARG LDFLAGS="-s -w"
WORKDIR /src

# Кэш зависимостей: сначала только манифесты модуля.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="${LDFLAGS}" -o /out/server       ./cmd/server \
 && CGO_ENABLED=0 go build -trimpath -ldflags="${LDFLAGS}" -o /out/mockfx       ./cmd/mockfx \
 && CGO_ENABLED=0 go build -trimpath -ldflags="${LDFLAGS}" -o /out/healthprobe  ./cmd/healthprobe

# Финальный образ: без шелла и пакетного менеджера, пользователь nonroot (uid 65532).
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/server       /server
COPY --from=builder /out/mockfx       /mockfx
COPY --from=builder /out/healthprobe  /healthprobe

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/server"]

// healthprobe — минимальная проверка здоровья для контейнеров на distroless
// (в образе нет shell/wget/curl). Выполняет GET на TARGET_URL и завершается
// с кодом 0 при 2xx-ответе, 1 — при любой ошибке. Используется в
// docker-compose healthcheck (требование Б5).
package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	target := os.Getenv("TARGET_URL")
	if target == "" {
		target = "http://localhost:8080/healthz"
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthprobe: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		fmt.Fprintf(os.Stderr, "healthprobe: status %d\n", resp.StatusCode)
		os.Exit(1)
	}
	os.Exit(0)
}

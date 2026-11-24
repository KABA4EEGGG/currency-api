package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// writeJSON отправляет JSON-ответ с кодом status.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if payload != nil {
		// Ошибка маршелинга после отправки заголовков не восстановима,
		// тела структур состоят из примитивов.
		_ = json.NewEncoder(w).Encode(payload)
	}
}

// writeError отправляет JSON c полем error.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// queryLimit читает ?limit= с ограничением сверху.
func queryLimit(r *http.Request, def int) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return def, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return 0, errors.New("limit must be a positive integer")
	}
	if limit > 100 {
		return 0, fmt.Errorf("limit must not exceed 100")
	}
	return limit, nil
}

// serveMetrics отдаёт экспозицию Prometheus.
func serveMetrics(w http.ResponseWriter, r *http.Request) {
	promhttp.Handler().ServeHTTP(w, r)
}

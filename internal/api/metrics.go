package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Метрики HTTP-слоя: счётчик запросов и гистограмма времени ответа
// с меткой route (шаблон маршрута chi — без ID, чтобы не взрывать кардинальность).
var (
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Число HTTP-запросов",
	}, []string{"method", "route", "code"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "Длительность обработки HTTP-запроса",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"method", "route"})
)

// statusRecorder запоминает HTTP-статус ответа.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// metricsMiddleware измеряет длительность и число запросов.
func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		route := chi.RouteContext(r.Context()).RoutePattern()
		if route == "" {
			route = "unmatched"
		}
		httpDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
		httpRequests.WithLabelValues(r.Method, route, strconv.Itoa(rec.status)).Inc()
	})
}

// Package api содержит HTTP-роутер и обработчики REST API.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/kseno/currency-api/internal/service"
	"github.com/kseno/currency-api/internal/store"
)

// API — набор обработчиков поверх бизнес-логики.
type API struct {
	svc    *service.Service
	logger *slog.Logger
}

// NewRouter собирает HTTP-роутер со всеми middleware и маршрутами.
func NewRouter(svc *service.Service, logger *slog.Logger) http.Handler {
	a := &API{svc: svc, logger: logger}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(metricsMiddleware)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(logger))

	r.Get("/healthz", a.health)
	r.Get("/readyz", a.ready)
	r.Get("/metrics", serveMetrics)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/rates/{base}", a.rates)
		r.Post("/convert", a.convert)
		r.Get("/conversions", a.history)
		r.Get("/stats", a.stats)
		r.Get("/favorites", a.favorites)
		r.Post("/favorites", a.addFavorite)
		r.Delete("/favorites/{id}", a.deleteFavorite)
	})

	return r
}

// requestLogger пишет одну строку на запрос (кроме /healthz, /readyz, /metrics).
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/healthz", "/readyz", "/metrics":
				next.ServeHTTP(w, r)
				return
			}

			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}

func (a *API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.Ping(r.Context()); err != nil {
		a.logger.Error("readiness check failed", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (a *API) rates(w http.ResponseWriter, r *http.Request) {
	base := chi.URLParam(r, "base")

	result, err := a.svc.Rates(r.Context(), base)
	if err != nil {
		a.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type convertRequest struct {
	From   string  `json:"from"`
	To     string  `json:"to"`
	Amount float64 `json:"amount"`
}

func (a *API) convert(w http.ResponseWriter, r *http.Request) {
	var req convertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	result, err := a.svc.Convert(r.Context(), service.ConvertInput{
		From:   req.From,
		To:     req.To,
		Amount: req.Amount,
	})
	if err != nil {
		a.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (a *API) history(w http.ResponseWriter, r *http.Request) {
	limit, err := queryLimit(r, 20)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	items, err := a.svc.History(r.Context(), limit)
	if err != nil {
		a.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *API) stats(w http.ResponseWriter, r *http.Request) {
	limit, err := queryLimit(r, 10)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	items, err := a.svc.Stats(r.Context(), limit)
	if err != nil {
		a.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *API) favorites(w http.ResponseWriter, r *http.Request) {
	items, err := a.svc.Favorites(r.Context())
	if err != nil {
		a.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

type favoriteRequest struct {
	Base  string `json:"base"`
	Quote string `json:"quote"`
}

func (a *API) addFavorite(w http.ResponseWriter, r *http.Request) {
	var req favoriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	f, err := a.svc.AddFavorite(r.Context(), req.Base, req.Quote)
	if err != nil {
		a.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

func (a *API) deleteFavorite(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid favorite id")
		return
	}

	deleted, err := a.svc.RemoveFavorite(r.Context(), id)
	if err != nil {
		a.handleError(w, err)
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "favorite not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleError транслирует ошибки бизнес-логики в HTTP-коды.
func (a *API) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidCurrency), errors.Is(err, service.ErrInvalidAmount):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrUnknownCurrency):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, store.ErrDuplicate):
		writeError(w, http.StatusConflict, "favorite already exists")
	default:
		a.logger.Error("internal error", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

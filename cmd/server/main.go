// Точка входа сервиса курсов валют.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kseno/currency-api/internal/api"
	"github.com/kseno/currency-api/internal/config"
	"github.com/kseno/currency-api/internal/fx"
	"github.com/kseno/currency-api/internal/migrate"
	"github.com/kseno/currency-api/internal/service"
	"github.com/kseno/currency-api/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()
	logger := setupLogger(cfg.LogLevel)
	slog.SetDefault(logger)
	logger.Info("starting currency-api", "cfg", cfg.String())

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := connectDB(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		return err
	}
	defer pool.Close()
	logger.Info("database connected")

	if err := migrate.Run(cfg.DatabaseURL); err != nil {
		return err
	}
	logger.Info("migrations applied")

	st := store.New(pool)
	store.NewPoolCollector(pool) // метрики пула соединений

	provider := fx.NewCachedProvider(
		fx.NewERAPIClient(cfg.ExchangeAPIURL, cfg.HTTPTimeout),
		cfg.RatesTTL,
	)
	svc := service.New(provider, st)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.NewRouter(svc, logger),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", cfg.ListenAddr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Info("server stopped")
	return nil
}

// connectDB подключается к PostgreSQL с повторными попытками (БД может
// стартовать дольше приложения).
func connectDB(ctx context.Context, databaseURL string, logger *slog.Logger) (*pgxpool.Pool, error) {
	const attempts = 30

	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	cfg.MaxConns = 20

	for attempt := 1; attempt <= attempts; attempt++ {
		select {
		case <-ctx.Done():
			return nil, errors.New("shutdown while connecting to database")
		default:
		}

		p, err := pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			if pingErr := p.Ping(ctx); pingErr != nil {
				p.Close()
				err = fmt.Errorf("ping: %w", pingErr)
			} else {
				return p, nil
			}
		}

		if attempt%5 == 0 {
			logger.Warn("database not ready", "attempt", attempt, "err", err)
		}
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("database unreachable after %d attempts", attempts)
}

func setupLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}

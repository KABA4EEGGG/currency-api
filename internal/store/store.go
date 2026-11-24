// Package store реализует хранилище данных на PostgreSQL (pgx/v5).
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/kseno/currency-api/internal/service"
)

// Pool — интерфейс пула соединений, необходимый для тестов.
type Pool interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Ping(ctx context.Context) error
}

// Store — реализация service.Store на PostgreSQL.
type Store struct {
	pool Pool
}

// Compile-time check.
var _ service.Store = (*Store)(nil)

// New создаёт хранилище поверх пула pgx.
func New(pool Pool) *Store {
	return &Store{pool: pool}
}

// ErrNotFound — запись не найдена (используется API-слоем для 404).
var ErrNotFound = errors.New("not found")

// ErrDuplicate — нарушено ограничение уникальности (409 Conflict).
var ErrDuplicate = errors.New("duplicate")

// ErrUniqueViolation — код ошибки PostgreSQL при нарушении UNIQUE.
const errUniqueViolation = "23505"

// SaveConversion сохраняет конвертацию и возвращает её ID и время создания.
func (s *Store) SaveConversion(
	ctx context.Context, from, to string, amount, rate, result float64,
) (int64, time.Time, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO conversions (base, quote, amount, rate, result)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`, from, to, amount, rate, result)

	var id int64
	var createdAt time.Time
	if err := row.Scan(&id, &createdAt); err != nil {
		return 0, time.Time{}, fmt.Errorf("insert conversion: %w", err)
	}
	return id, createdAt, nil
}

// ListConversions возвращает последние конвертации.
func (s *Store) ListConversions(ctx context.Context, limit int) ([]service.Conversion, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, base, quote, amount, rate, result, created_at
		FROM conversions
		ORDER BY id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("select conversions: %w", err)
	}
	defer rows.Close()

	items := make([]service.Conversion, 0, limit)
	for rows.Next() {
		var c service.Conversion
		if err := rows.Scan(&c.ID, &c.Base, &c.Quote, &c.Amount, &c.Rate, &c.Result, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan conversion: %w", err)
		}
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversions: %w", err)
	}
	return items, nil
}

// ConversionStats возвращает топ пар по числу конвертаций.
func (s *Store) ConversionStats(ctx context.Context, limit int) ([]service.PairStat, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT base, quote, count(*) AS cnt
		FROM conversions
		GROUP BY base, quote
		ORDER BY cnt DESC, max(created_at) DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("select stats: %w", err)
	}
	defer rows.Close()

	items := make([]service.PairStat, 0, limit)
	for rows.Next() {
		var st service.PairStat
		if err := rows.Scan(&st.Base, &st.Quote, &st.Count); err != nil {
			return nil, fmt.Errorf("scan stat: %w", err)
		}
		items = append(items, st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stats: %w", err)
	}
	return items, nil
}

// AddFavorite добавляет валютную пару в избранное.
func (s *Store) AddFavorite(ctx context.Context, base, quote string) (service.Favorite, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO favorites (base, quote)
		VALUES ($1, $2)
		RETURNING id, base, quote, created_at
	`, base, quote)

	var f service.Favorite
	err := row.Scan(&f.ID, &f.Base, &f.Quote, &f.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == errUniqueViolation {
			return service.Favorite{}, ErrDuplicate
		}
		return service.Favorite{}, fmt.Errorf("insert favorite: %w", err)
	}
	return f, nil
}

// ListFavorites возвращает все избранные пары.
func (s *Store) ListFavorites(ctx context.Context) ([]service.Favorite, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, base, quote, created_at
		FROM favorites
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("select favorites: %w", err)
	}
	defer rows.Close()

	items := make([]service.Favorite, 0)
	for rows.Next() {
		var f service.Favorite
		if err := rows.Scan(&f.ID, &f.Base, &f.Quote, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan favorite: %w", err)
		}
		items = append(items, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate favorites: %w", err)
	}
	return items, nil
}

// DeleteFavorite удаляет пару по ID; false — если записи не было.
func (s *Store) DeleteFavorite(ctx context.Context, id int64) (bool, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM favorites WHERE id = $1`, id)
	if err != nil {
		return false, fmt.Errorf("delete favorite: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// Ping проверяет доступность БД.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// PoolCollector экспортирует статистику пула pgx в Prometheus.
type PoolCollector struct {
	pool *pgxpool.Pool

	open *prometheus.GaugeVec
}

// NewPoolCollector регистрирует метрики пула соединений.
func NewPoolCollector(pool *pgxpool.Pool) *PoolCollector {
	c := &PoolCollector{
		pool: pool,
		open: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "db_pool_open_connections",
			Help: "Открытые соединения пула PostgreSQL",
		}, []string{"state"}),
	}
	prometheus.MustRegister(c)
	return c
}

// Describe реализует prometheus.Collector.
func (c *PoolCollector) Describe(ch chan<- *prometheus.Desc) {
	c.open.Describe(ch)
}

// Collect реализует prometheus.Collector.
func (c *PoolCollector) Collect(ch chan<- prometheus.Metric) {
	stat := c.pool.Stat()
	c.open.WithLabelValues("idle").Set(float64(stat.IdleConns()))
	c.open.WithLabelValues("in_use").Set(float64(stat.AcquiredConns()))
	c.open.Collect(ch)
}

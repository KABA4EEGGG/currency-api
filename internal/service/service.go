// Package service содержит бизнес-логику: конвертацию валют, работу с
// историей конвертаций и избранными парами.
package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"time"

	"github.com/kseno/currency-api/internal/fx"
)

// Валидный ISO-код валюты: три заглавные буквы.
var currencyCodeRe = regexp.MustCompile(`^[A-Z]{3}$`)

const maxAmount = 1e12

// Ошибки, которые слой API транслирует в HTTP-коды.
var (
	ErrInvalidCurrency = errors.New("invalid currency code")
	ErrUnknownCurrency = errors.New("unknown currency")
	ErrInvalidAmount   = errors.New("invalid amount")
)

// Store — интерфейс хранилища. Позволяет подменять БД в тестах.
type Store interface {
	SaveConversion(ctx context.Context, from, to string, amount, rate, result float64) (int64, time.Time, error)
	ListConversions(ctx context.Context, limit int) ([]Conversion, error)
	ConversionStats(ctx context.Context, limit int) ([]PairStat, error)
	AddFavorite(ctx context.Context, base, quote string) (Favorite, error)
	ListFavorites(ctx context.Context) ([]Favorite, error)
	DeleteFavorite(ctx context.Context, id int64) (bool, error)
	Ping(ctx context.Context) error
}

// Conversion — запись истории конвертаций.
type Conversion struct {
	ID        int64     `json:"id"`
	Base      string    `json:"base"`
	Quote     string    `json:"quote"`
	Amount    float64   `json:"amount"`
	Rate      float64   `json:"rate"`
	Result    float64   `json:"result"`
	CreatedAt time.Time `json:"created_at"`
}

// Favorite — избранная валютная пара.
type Favorite struct {
	ID        int64     `json:"id"`
	Base      string    `json:"base"`
	Quote     string    `json:"quote"`
	CreatedAt time.Time `json:"created_at"`
}

// PairStat — статистика использования пары.
type PairStat struct {
	Base  string `json:"base"`
	Quote string `json:"quote"`
	Count int64  `json:"count"`
}

// RatesResult — ответ метода rates.
type RatesResult = fx.Rates

// Service — бизнес-логика приложения.
type Service struct {
	rates fx.Provider
	store Store
}

// New создаёт сервис.
func New(rates fx.Provider, store Store) *Service {
	return &Service{rates: rates, store: store}
}

// ConvertInput — вход конвертации.
type ConvertInput struct {
	From   string
	To     string
	Amount float64
}

// ConvertResult — результат конвертации с записью в историю.
type ConvertResult struct {
	ID        int64     `json:"id"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Amount    float64   `json:"amount"`
	Rate      float64   `json:"rate"`
	Result    float64   `json:"result"`
	CreatedAt time.Time `json:"created_at"`
}

// Convert выполняет конвертацию amount из From в To и сохраняет её в историю.
func (s *Service) Convert(ctx context.Context, in ConvertInput) (ConvertResult, error) {
	if !currencyCodeRe.MatchString(in.From) {
		return ConvertResult{}, fmt.Errorf("%w: from=%q", ErrInvalidCurrency, in.From)
	}
	if !currencyCodeRe.MatchString(in.To) {
		return ConvertResult{}, fmt.Errorf("%w: to=%q", ErrInvalidCurrency, in.To)
	}
	if in.Amount <= 0 || in.Amount > maxAmount || math.IsNaN(in.Amount) || math.IsInf(in.Amount, 0) {
		return ConvertResult{}, fmt.Errorf("%w: amount=%v", ErrInvalidAmount, in.Amount)
	}

	rates, err := s.rates.Rates(ctx, in.From)
	if err != nil {
		return ConvertResult{}, fmt.Errorf("fetch rates: %w", err)
	}

	rate, ok := rates.Rate(in.To)
	if !ok {
		return ConvertResult{}, fmt.Errorf("%w: %s->%s", ErrUnknownCurrency, in.From, in.To)
	}

	result := in.Amount * rate
	id, createdAt, err := s.store.SaveConversion(ctx, in.From, in.To, in.Amount, rate, result)
	if err != nil {
		return ConvertResult{}, fmt.Errorf("save conversion: %w", err)
	}

	return ConvertResult{
		ID:        id,
		From:      in.From,
		To:        in.To,
		Amount:    in.Amount,
		Rate:      rate,
		Result:    result,
		CreatedAt: createdAt,
	}, nil
}

// Rates возвращает курсы валют относительно base.
func (s *Service) Rates(ctx context.Context, base string) (RatesResult, error) {
	if !currencyCodeRe.MatchString(base) {
		return RatesResult{}, fmt.Errorf("%w: base=%q", ErrInvalidCurrency, base)
	}
	rates, err := s.rates.Rates(ctx, base)
	if err != nil {
		return RatesResult{}, fmt.Errorf("fetch rates: %w", err)
	}
	return rates, nil
}

// History возвращает последние конвертации (не больше 100).
func (s *Service) History(ctx context.Context, limit int) ([]Conversion, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return s.store.ListConversions(ctx, limit)
}

// Stats возвращает топ валютных пар по числу конвертаций.
func (s *Service) Stats(ctx context.Context, limit int) ([]PairStat, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	return s.store.ConversionStats(ctx, limit)
}

// AddFavorite добавляет пару в избранное.
func (s *Service) AddFavorite(ctx context.Context, base, quote string) (Favorite, error) {
	if !currencyCodeRe.MatchString(base) || !currencyCodeRe.MatchString(quote) {
		return Favorite{}, fmt.Errorf("%w: %s/%s", ErrInvalidCurrency, base, quote)
	}
	return s.store.AddFavorite(ctx, base, quote)
}

// Favorites возвращает список избранных пар.
func (s *Service) Favorites(ctx context.Context) ([]Favorite, error) {
	return s.store.ListFavorites(ctx)
}

// RemoveFavorite удаляет пару из избранного; false — если записи не было.
func (s *Service) RemoveFavorite(ctx context.Context, id int64) (bool, error) {
	return s.store.DeleteFavorite(ctx, id)
}

// Ping проверяет доступность хранилища (readiness).
func (s *Service) Ping(ctx context.Context) error {
	return s.store.Ping(ctx)
}

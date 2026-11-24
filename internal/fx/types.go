// Package fx отвечает за получение курсов валют от внешнего провайдера
// (open.er-api.com) и кэширование ответов, чтобы не превышать лимиты API
// и не создавать нагрузку на сторонний сервис (требование В3).
package fx

import (
	"context"
	"time"
)

// Rates — курсы валют относительно базовой валюты.
type Rates struct {
	Base      string             `json:"base"`
	Rates     map[string]float64 `json:"rates"`
	UpdatedAt time.Time          `json:"updated_at"`
}

// Rate возвращает курс валюты code относительно base.
func (r Rates) Rate(code string) (float64, bool) {
	v, ok := r.Rates[code]
	return v, ok
}

// Provider — источник курсов валют.
type Provider interface {
	// Rates возвращает курсы для базовой валюты base (ISO-код, например USD).
	Rates(ctx context.Context, base string) (Rates, error)
}

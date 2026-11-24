package fx

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/sync/singleflight"
)

// CachedProvider оборачивает Provider кэшем в памяти с TTL и защитой от
// «лавины» одинаковых запросов (singleflight). Кэш обязателен по требованию
// В3: под нагрузкой запросы не должны уходить во внешний API.
type CachedProvider struct {
	next Provider
	ttl  time.Duration
	grp  singleflight.Group

	mu    sync.RWMutex
	items map[string]cacheEntry

	hits        prometheus.Counter
	misses      prometheus.Counter
	fetchErrors prometheus.Counter
}

type cacheEntry struct {
	rates   Rates
	expires time.Time
}

// NewCachedProvider создаёт кэширующий провайдер и регистрирует метрики кэша.
func NewCachedProvider(next Provider, ttl time.Duration) *CachedProvider {
	return &CachedProvider{
		next:  next,
		ttl:   ttl,
		items: make(map[string]cacheEntry),
		hits: promauto.NewCounter(prometheus.CounterOpts{
			Name: "fx_cache_hits_total",
			Help: "Число попаданий в кэш курсов валют",
		}),
		misses: promauto.NewCounter(prometheus.CounterOpts{
			Name: "fx_cache_misses_total",
			Help: "Число промахов кэша курсов валют",
		}),
		fetchErrors: promauto.NewCounter(prometheus.CounterOpts{
			Name: "fx_fetch_errors_total",
			Help: "Число ошибок получения курсов от внешнего API",
		}),
	}
}

// Rates возвращает курсы из кэша либо из внешнего провайдера.
func (p *CachedProvider) Rates(ctx context.Context, base string) (Rates, error) {
	base = strings.ToUpper(base)

	if rates, ok := p.get(base); ok {
		p.hits.Inc()
		return rates, nil
	}
	p.misses.Inc()

	v, err, _ := p.grp.Do(base, func() (any, error) {
		// Вторая проверка: пока ждали в singleflight, другой запрос мог
		// уже положить курсы в кэш.
		if rates, ok := p.get(base); ok {
			return rates, nil
		}
		rates, err := p.next.Rates(ctx, base)
		if err != nil {
			p.fetchErrors.Inc()
			return nil, err
		}
		p.set(base, rates)
		return rates, nil
	})
	if err != nil {
		return Rates{}, err
	}
	return v.(Rates), nil
}

func (p *CachedProvider) get(base string) (Rates, bool) {
	p.mu.RLock()
	entry, ok := p.items[base]
	p.mu.RUnlock()
	if !ok || time.Now().After(entry.expires) {
		return Rates{}, false
	}
	return entry.rates, true
}

func (p *CachedProvider) set(base string, rates Rates) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.items[base] = cacheEntry{
		rates:   rates,
		expires: time.Now().Add(p.ttl),
	}
}

// Compile-time check: CachedProvider реализует Provider.
var _ Provider = (*CachedProvider)(nil)

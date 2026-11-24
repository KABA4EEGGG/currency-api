package fx

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

type fakeProvider struct {
	calls atomic.Int64
	delay time.Duration
	rates Rates
	err   error
}

func (f *fakeProvider) Rates(_ context.Context, base string) (Rates, error) {
	f.calls.Add(1)
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.err != nil {
		return Rates{}, f.err
	}
	return Rates{Base: base, Rates: f.rates.Rates, UpdatedAt: time.Now().UTC()}, nil
}

func newTestCache(next Provider, ttl time.Duration) *CachedProvider {
	return &CachedProvider{
		next:        next,
		ttl:         ttl,
		items:       make(map[string]cacheEntry),
		hits:        newTestCounter("hits"),
		misses:      newTestCounter("misses"),
		fetchErrors: newTestCounter("fetch_errors"),
	}
}

func newTestCounter(name string) prometheus.Counter {
	return prometheus.NewCounter(prometheus.CounterOpts{Name: name})
}

func TestCachedProvider_CachesRates(t *testing.T) {
	prov := &fakeProvider{rates: Rates{Rates: map[string]float64{"EUR": 0.9}}}
	cache := newTestCache(prov, time.Minute)
	ctx := context.Background()

	first, err := cache.Rates(ctx, "USD")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	second, err := cache.Rates(ctx, "USD")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if first.Base != second.Base || first.UpdatedAt != second.UpdatedAt {
		t.Fatalf("expected cached copy, got different results")
	}
	if got := prov.calls.Load(); got != 1 {
		t.Fatalf("provider called %d times, want 1", got)
	}
	if testutil.ToFloat64(cache.hits) != 1 {
		t.Fatalf("hits = %v, want 1", testutil.ToFloat64(cache.hits))
	}
	if testutil.ToFloat64(cache.misses) != 1 {
		t.Fatalf("misses = %v, want 1", testutil.ToFloat64(cache.misses))
	}
}

func TestCachedProvider_ExpiresEntry(t *testing.T) {
	prov := &fakeProvider{rates: Rates{Rates: map[string]float64{"EUR": 0.9}}}
	cache := newTestCache(prov, 20*time.Millisecond)
	ctx := context.Background()

	if _, err := cache.Rates(ctx, "USD"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	time.Sleep(40 * time.Millisecond)
	if _, err := cache.Rates(ctx, "USD"); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if got := prov.calls.Load(); got != 2 {
		t.Fatalf("provider called %d times after expiry, want 2", got)
	}
}

func TestCachedProvider_UppercasesBase(t *testing.T) {
	prov := &fakeProvider{rates: Rates{Rates: map[string]float64{"EUR": 0.9}}}
	cache := newTestCache(prov, time.Minute)
	ctx := context.Background()

	if _, err := cache.Rates(ctx, "usd"); err != nil {
		t.Fatalf("lowercase call: %v", err)
	}
	if _, err := cache.Rates(ctx, "USD"); err != nil {
		t.Fatalf("uppercase call: %v", err)
	}
	if got := prov.calls.Load(); got != 1 {
		t.Fatalf("provider called %d times, want 1 (cache shared for cases)", got)
	}
}

func TestCachedProvider_SingleflightDeduplicates(t *testing.T) {
	prov := &fakeProvider{delay: 100 * time.Millisecond, rates: Rates{Rates: map[string]float64{"EUR": 0.9}}}
	cache := newTestCache(prov, time.Minute)
	ctx := context.Background()

	const goroutines = 10
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := cache.Rates(ctx, "USD"); err != nil {
				t.Errorf("concurrent call: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := prov.calls.Load(); got != 1 {
		t.Fatalf("provider called %d times concurrently, want 1", got)
	}
}

func TestCachedProvider_PropagatesErrors(t *testing.T) {
	prov := &fakeProvider{err: errors.New("boom")}
	cache := newTestCache(prov, time.Minute)

	if _, err := cache.Rates(context.Background(), "USD"); err == nil {
		t.Fatal("expected error from provider")
	}
	if testutil.ToFloat64(cache.fetchErrors) != 1 {
		t.Fatalf("fetch_errors = %v, want 1", testutil.ToFloat64(cache.fetchErrors))
	}
}

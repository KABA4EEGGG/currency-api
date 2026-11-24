package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kseno/currency-api/internal/fx"
	"github.com/kseno/currency-api/internal/service"
	"github.com/kseno/currency-api/internal/store"
)

// fakeStore — подменное хранилище для HTTP-тестов.
type fakeStore struct {
	conversions []service.Conversion
	favorites   []service.Favorite
	dup         bool
	pingErr     error
}

func (s *fakeStore) SaveConversion(
	_ context.Context, from, to string, amount, rate, result float64,
) (int64, time.Time, error) {
	id := int64(len(s.conversions) + 1)
	s.conversions = append(s.conversions, service.Conversion{
		ID: id, Base: from, Quote: to, Amount: amount, Rate: rate, Result: result,
		CreatedAt: time.Now(),
	})
	return id, time.Now(), nil
}

func (s *fakeStore) ListConversions(_ context.Context, limit int) ([]service.Conversion, error) {
	if limit > len(s.conversions) {
		limit = len(s.conversions)
	}
	return s.conversions[:limit], nil
}

func (s *fakeStore) ConversionStats(_ context.Context, _ int) ([]service.PairStat, error) {
	return []service.PairStat{{Base: "USD", Quote: "EUR", Count: int64(len(s.conversions))}}, nil
}

func (s *fakeStore) AddFavorite(_ context.Context, base, quote string) (service.Favorite, error) {
	if s.dup {
		return service.Favorite{}, store.ErrDuplicate
	}
	f := service.Favorite{ID: int64(len(s.favorites) + 1), Base: base, Quote: quote, CreatedAt: time.Now()}
	s.favorites = append(s.favorites, f)
	return f, nil
}

func (s *fakeStore) ListFavorites(_ context.Context) ([]service.Favorite, error) {
	return s.favorites, nil
}

func (s *fakeStore) DeleteFavorite(_ context.Context, id int64) (bool, error) {
	for i, f := range s.favorites {
		if f.ID == id {
			s.favorites = append(s.favorites[:i], s.favorites[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (s *fakeStore) Ping(_ context.Context) error { return s.pingErr }

// fakeProvider — подменный провайдер курсов.
type fakeProvider struct{}

func (fakeProvider) Rates(_ context.Context, base string) (fx.Rates, error) {
	if strings.ToUpper(base) != "USD" {
		return fx.Rates{}, errors.New("no such base")
	}
	return fx.Rates{
		Base:      "USD",
		Rates:     map[string]float64{"USD": 1, "EUR": 0.9, "RUB": 90},
		UpdatedAt: time.Now(),
	}, nil
}

func newTestServer(st service.Store) *httptest.Server {
	provider := fakeProvider{}
	svc := service.New(provider, st)
	server := httptest.NewServer(NewRouter(svc, slog.Default()))
	return server
}

func doJSON(t *testing.T, server *httptest.Server, method, path, body string) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, server.URL+path, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, string(raw)
}

func TestHealthz(t *testing.T) {
	server := newTestServer(&fakeStore{})
	defer server.Close()

	code, body := doJSON(t, server, http.MethodGet, "/healthz", "")
	if code != http.StatusOK || !strings.Contains(body, "ok") {
		t.Fatalf("healthz: code=%d body=%s", code, body)
	}
}

func TestReadyz_HealthyAndUnhealthy(t *testing.T) {
	healthy := newTestServer(&fakeStore{})
	defer healthy.Close()
	if code, _ := doJSON(t, healthy, http.MethodGet, "/readyz", ""); code != http.StatusOK {
		t.Fatalf("readyz healthy: code=%d", code)
	}

	unhealthy := newTestServer(&fakeStore{pingErr: errors.New("db down")})
	defer unhealthy.Close()
	if code, _ := doJSON(t, unhealthy, http.MethodGet, "/readyz", ""); code != http.StatusServiceUnavailable {
		t.Fatalf("readyz unhealthy: code=%d", code)
	}
}

func TestConvert_CreatedAndSaved(t *testing.T) {
	st := &fakeStore{}
	server := newTestServer(st)
	defer server.Close()

	code, body := doJSON(t, server, http.MethodPost, "/api/v1/convert",
		`{"from":"USD","to":"EUR","amount":100}`)
	if code != http.StatusCreated {
		t.Fatalf("convert code = %d, want 201, body=%s", code, body)
	}

	var result struct {
		ID     int64   `json:"id"`
		Result float64 `json:"result"`
		Rate   float64 `json:"rate"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, body)
	}
	if result.Rate != 0.9 || result.Result < 89.9 || result.Result > 90.1 {
		t.Fatalf("bad convert result: %+v", result)
	}
	if len(st.conversions) != 1 {
		t.Fatalf("conversion not saved to store")
	}
}

func TestConvert_ValidationErrors(t *testing.T) {
	server := newTestServer(&fakeStore{})
	defer server.Close()

	cases := []struct {
		name string
		body string
		code int
	}{
		{"bad json", `{oops`, http.StatusBadRequest},
		{"bad currency", `{"from":"us","to":"EUR","amount":1}`, http.StatusBadRequest},
		{"unknown currency", `{"from":"USD","to":"XXX","amount":1}`, http.StatusUnprocessableEntity},
		{"bad amount", `{"from":"USD","to":"EUR","amount":-1}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _ := doJSON(t, server, http.MethodPost, "/api/v1/convert", tc.body)
			if code != tc.code {
				t.Fatalf("code = %d, want %d", code, tc.code)
			}
		})
	}
}

func TestRatesEndpoint(t *testing.T) {
	server := newTestServer(&fakeStore{})
	defer server.Close()

	code, body := doJSON(t, server, http.MethodGet, "/api/v1/rates/USD", "")
	if code != http.StatusOK || !strings.Contains(body, `"EUR":0.9`) {
		t.Fatalf("rates: code=%d body=%s", code, body)
	}

	code, _ = doJSON(t, server, http.MethodGet, "/api/v1/rates/12", "")
	if code != http.StatusBadRequest {
		t.Fatalf("rates invalid base: code=%d", code)
	}
}

func TestFavoritesCRUD(t *testing.T) {
	st := &fakeStore{}
	server := newTestServer(st)
	defer server.Close()

	code, body := doJSON(t, server, http.MethodPost, "/api/v1/favorites",
		`{"base":"USD","quote":"EUR"}`)
	if code != http.StatusCreated {
		t.Fatalf("add favorite: code=%d body=%s", code, body)
	}

	code, _ = doJSON(t, server, http.MethodGet, "/api/v1/favorites", "")
	if code != http.StatusOK {
		t.Fatalf("list favorites: code=%d", code)
	}

	code, _ = doJSON(t, server, http.MethodDelete, "/api/v1/favorites/1", "")
	if code != http.StatusNoContent {
		t.Fatalf("delete favorite: code=%d", code)
	}

	code, _ = doJSON(t, server, http.MethodDelete, "/api/v1/favorites/99", "")
	if code != http.StatusNotFound {
		t.Fatalf("delete missing favorite: code=%d", code)
	}
}

func TestFavorites_DuplicateConflict(t *testing.T) {
	server := newTestServer(&fakeStore{dup: true})
	defer server.Close()

	code, _ := doJSON(t, server, http.MethodPost, "/api/v1/favorites",
		`{"base":"USD","quote":"EUR"}`)
	if code != http.StatusConflict {
		t.Fatalf("duplicate favorite: code=%d, want 409", code)
	}
}

func TestHistoryAndStats(t *testing.T) {
	server := newTestServer(&fakeStore{})
	defer server.Close()

	// Populate: одна конвертация пишет запись в хранилище.
	code, _ := doJSON(t, server, http.MethodPost, "/api/v1/convert",
		`{"from":"USD","to":"EUR","amount":1}`)
	if code != http.StatusCreated {
		t.Fatalf("convert setup: code=%d", code)
	}

	code, body := doJSON(t, server, http.MethodGet, "/api/v1/conversions", "")
	if code != http.StatusOK || !strings.Contains(body, "USD") {
		t.Fatalf("conversions: code=%d body=%s", code, body)
	}

	code, body = doJSON(t, server, http.MethodGet, "/api/v1/stats", "")
	if code != http.StatusOK || !strings.Contains(body, `"count":1`) {
		t.Fatalf("stats: code=%d body=%s", code, body)
	}
}

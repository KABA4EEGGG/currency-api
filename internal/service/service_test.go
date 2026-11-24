package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kseno/currency-api/internal/fx"
)

type fakeProvider struct {
	rates fx.Rates
	err   error
}

func (f *fakeProvider) Rates(_ context.Context, base string) (fx.Rates, error) {
	if f.err != nil {
		return fx.Rates{}, f.err
	}
	return fx.Rates{Base: base, Rates: f.rates.Rates, UpdatedAt: time.Now()}, nil
}

type fakeStore struct {
	conversions []Conversion
	favorites   []Favorite
	saved       []ConvertInput
	pingErr     error
	dupOn       bool
}

func (s *fakeStore) SaveConversion(_ context.Context, from, to string, amount, rate, result float64) (int64, time.Time, error) {
	if s.dupOn {
		return 0, time.Time{}, errors.New("should not save")
	}
	s.saved = append(s.saved, ConvertInput{From: from, To: to, Amount: amount})
	id := int64(len(s.saved))
	s.conversions = append(s.conversions, Conversion{
		ID: id, Base: from, Quote: to, Amount: amount, Rate: rate, Result: result,
		CreatedAt: time.Now(),
	})
	return id, time.Now(), nil
}

func (s *fakeStore) ListConversions(_ context.Context, limit int) ([]Conversion, error) {
	if limit > len(s.conversions) {
		limit = len(s.conversions)
	}
	return s.conversions[:limit], nil
}

func (s *fakeStore) ConversionStats(_ context.Context, _ int) ([]PairStat, error) {
	return []PairStat{{Base: "USD", Quote: "EUR", Count: 3}}, nil
}

func (s *fakeStore) AddFavorite(_ context.Context, base, quote string) (Favorite, error) {
	if s.dupOn {
		return Favorite{}, ErrDuplicateStore
	}
	f := Favorite{ID: int64(len(s.favorites) + 1), Base: base, Quote: quote, CreatedAt: time.Now()}
	s.favorites = append(s.favorites, f)
	return f, nil
}

func (s *fakeStore) ListFavorites(_ context.Context) ([]Favorite, error) { return s.favorites, nil }

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

// ErrDuplicateStore имитирует ошибку уникальности из хранилища.
var ErrDuplicateStore = errors.New("duplicate")

func newTestService(st Store) *Service {
	provider := &fakeProvider{rates: fx.Rates{Rates: map[string]float64{
		"USD": 1, "EUR": 0.9, "RUB": 90,
	}}}
	return New(provider, st)
}

func TestConvert_HappyPath(t *testing.T) {
	st := &fakeStore{}
	svc := newTestService(st)

	res, err := svc.Convert(context.Background(), ConvertInput{From: "USD", To: "EUR", Amount: 100})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if res.ID != 1 {
		t.Fatalf("id = %d, want 1", res.ID)
	}
	if res.Result < 89.9 || res.Result > 90.1 {
		t.Fatalf("result = %v, want ~90", res.Result)
	}
	if len(st.saved) != 1 || st.saved[0].From != "USD" {
		t.Fatalf("conversion not saved: %+v", st.saved)
	}
}

func TestConvert_InvalidInputs(t *testing.T) {
	st := &fakeStore{}
	svc := newTestService(st)

	cases := []struct {
		name string
		in   ConvertInput
		want error
	}{
		{"bad from", ConvertInput{From: "usd", To: "EUR", Amount: 1}, ErrInvalidCurrency},
		{"short to", ConvertInput{From: "USD", To: "EU", Amount: 1}, ErrInvalidCurrency},
		{"zero amount", ConvertInput{From: "USD", To: "EUR", Amount: 0}, ErrInvalidAmount},
		{"negative amount", ConvertInput{From: "USD", To: "EUR", Amount: -5}, ErrInvalidAmount},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Convert(context.Background(), tc.in)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestConvert_UnknownQuoteCurrency(t *testing.T) {
	svc := newTestService(&fakeStore{})

	_, err := svc.Convert(context.Background(), ConvertInput{From: "USD", To: "XYZ", Amount: 1})
	if !errors.Is(err, ErrUnknownCurrency) {
		t.Fatalf("err = %v, want ErrUnknownCurrency", err)
	}
}

func TestHistory_LimitsApplied(t *testing.T) {
	st := &fakeStore{}
	for i := 0; i < 150; i++ {
		_, _, _ = st.SaveConversion(context.Background(), "USD", "EUR", 1, 0.9, 0.9)
	}
	svc := newTestService(st)

	items, err := svc.History(context.Background(), 500)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(items) != 100 {
		t.Fatalf("history len = %d, want 100", len(items))
	}
}

func TestAddFavorite_Validation(t *testing.T) {
	svc := newTestService(&fakeStore{})

	if _, err := svc.AddFavorite(context.Background(), "usd", "EUR"); !errors.Is(err, ErrInvalidCurrency) {
		t.Fatalf("err = %v, want ErrInvalidCurrency", err)
	}
}

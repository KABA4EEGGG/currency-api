// mockfx — заглушка внешнего API курсов (формат open.er-api.com) для
// нагрузочного тестирования: по требованию В3 под нагрузку нельзя ставить
// сторонний API, поэтому сервис переключается на этот мок через
// FX_EXCHANGE_API_URL.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// baseRates — курсы относительно USD, фиксированные и детерминированные.
var baseRates = map[string]float64{
	"USD": 1,
	"EUR": 0.92,
	"GBP": 0.79,
	"RUB": 92.5,
	"CNY": 7.24,
	"JPY": 149.3,
	"KZT": 468.1,
	"CHF": 0.88,
}

func handler(w http.ResponseWriter, r *http.Request) {
	base := strings.ToUpper(strings.TrimPrefix(r.URL.Path, "/v6/latest/"))

	rus, ok := ratesFor(base)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"result": "error"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"result":               "success",
		"provider":             "mockfx",
		"base_code":            base,
		"rates":                rus,
		"time_last_update_utc": time.Now().UTC().Format(time.RFC1123),
	})
}

// ratesFor пересчитывает базовые курсы относительно запрошенной валюты.
func ratesFor(base string) (map[string]float64, bool) {
	usdBase, ok := baseRates[base]
	if !ok {
		return nil, false
	}
	out := make(map[string]float64, len(baseRates))
	for code, usdRate := range baseRates {
		out[code] = round(usdRate / usdBase)
	}
	return out, true
}

func round(v float64) float64 {
	return float64(int(v*1e6)) / 1e6
}

func main() {
	addr := os.Getenv("FX_LISTEN_ADDR")
	if addr == "" {
		addr = ":8090"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v6/latest/", handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	log.Printf("mockfx listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

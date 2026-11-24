package fx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestERAPIClient_ParsesRates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"result": "success",
			"base_code": "USD",
			"rates": {"USD": 1, "EUR": 0.92, "RUB": 92.5}
		}`))
	}))
	defer server.Close()

	client := NewERAPIClient(server.URL, time.Second)
	rates, err := client.Rates(context.Background(), "USD")
	if err != nil {
		t.Fatalf("Rates: %v", err)
	}
	if rates.Base != "USD" {
		t.Fatalf("base = %q, want USD", rates.Base)
	}
	if rate, ok := rates.Rate("EUR"); !ok || rate != 0.92 {
		t.Fatalf("EUR rate = %v (ok=%v), want 0.92", rate, ok)
	}
}

func TestERAPIClient_BuildsURLFromBase(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"result":"success","base_code":"EUR","rates":{"USD":1.08}}`))
	}))
	defer server.Close()

	client := NewERAPIClient(server.URL, time.Second)
	if _, err := client.Rates(context.Background(), "EUR"); err != nil {
		t.Fatalf("Rates: %v", err)
	}
	if gotPath != "/EUR" {
		t.Fatalf("request path = %q, want /EUR", gotPath)
	}
}

func TestERAPIClient_ErrorOnBadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewERAPIClient(server.URL, time.Second)
	if _, err := client.Rates(context.Background(), "USD"); err == nil {
		t.Fatal("expected error on HTTP 429")
	}
}

func TestERAPIClient_ErrorOnBadPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"result":"error"}`))
	}))
	defer server.Close()

	client := NewERAPIClient(server.URL, time.Second)
	if _, err := client.Rates(context.Background(), "USD"); err == nil {
		t.Fatal("expected error on non-success result")
	}
}

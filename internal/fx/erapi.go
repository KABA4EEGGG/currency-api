package fx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// ERAPIClient — клиент внешнего API exchangerate-api (open.er-api.com).
// Ответ формата {"result":"success","base_code":"USD","rates":{...}}.
type ERAPIClient struct {
	baseURL string
	client  *http.Client
}

// NewERAPIClient создаёт клиент по базовому URL
// (по умолчанию https://open.er-api.com/v6/latest).
func NewERAPIClient(baseURL string, timeout time.Duration) *ERAPIClient {
	return &ERAPIClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		client:  &http.Client{Timeout: timeout},
	}
}

type erapiResponse struct {
	Result   string             `json:"result"`
	BaseCode string             `json:"base_code"`
	Rates    map[string]float64 `json:"rates"`
}

var errUnexpectedResult = errors.New("external api returned non-success result")

// Rates запрашивает курсы для базовой валюты base.
func (c *ERAPIClient) Rates(ctx context.Context, base string) (Rates, error) {
	base = strings.ToUpper(base)
	url := fmt.Sprintf("%s/%s", c.baseURL, base)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Rates{}, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return Rates{}, fmt.Errorf("external api request: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Warn("close response body", "err", cerr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return Rates{}, fmt.Errorf(
			"external api: unexpected status %d: %s",
			resp.StatusCode, string(body),
		)
	}

	var payload erapiResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Rates{}, fmt.Errorf("decode external api response: %w", err)
	}
	if payload.Result != "success" {
		return Rates{}, fmt.Errorf("%w: %q", errUnexpectedResult, payload.Result)
	}
	if payload.BaseCode == "" || len(payload.Rates) == 0 {
		return Rates{}, errors.New("external api: empty rates payload")
	}

	return Rates{
		Base:      payload.BaseCode,
		Rates:     payload.Rates,
		UpdatedAt: time.Now().UTC(),
	}, nil
}

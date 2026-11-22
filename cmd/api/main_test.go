package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func sample850Body() []byte {
	payload, _ := json.Marshal(map[string]any{
		"generate_997": true,
		"payload": "ISA*00*          *00*          *ZZ*SENDER123      *ZZ*RECEIVER456    *261122*0245*U*00401*000000101*0*P*:~" +
			"GS*PO*SENDER123*RECEIVER456*20261122*0245*101*X*004010~" +
			"ST*850*0001~BEG*00*NE*PO-99**20261122~PO1*1*5*EA*10**VP*ITEM1~SE*4*0001~GE*1*101~IEA*1*000000101~",
	})
	return payload
}

func TestHealthAndParseEndpoints(t *testing.T) {
	handler := newHandler()

	// 1. Health check (/health)
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	recHealth := httptest.NewRecorder()
	handler.ServeHTTP(recHealth, reqHealth)
	if recHealth.Code != http.StatusOK {
		t.Fatalf("expected 200 from /health, got %d", recHealth.Code)
	}

	// 2. Unauthorized parse check
	body := `{"payload":"ISA*00*...","generate_997":true}`
	reqUnauth := httptest.NewRequest(http.MethodPost, "/v1/parse", bytes.NewBufferString(body))
	reqUnauth.Header.Set("Content-Type", "application/json")
	recUnauth := httptest.NewRecorder()
	handler.ServeHTTP(recUnauth, reqUnauth)
	if recUnauth.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without API key, got %d", recUnauth.Code)
	}

	// 3. Valid parse check with test Bearer token
	reqOK := httptest.NewRequest(http.MethodPost, "/v1/parse", bytes.NewReader(sample850Body()))
	reqOK.Header.Set("Content-Type", "application/json")
	reqOK.Header.Set("Authorization", "Bearer edix_live_test_zohaib_001")
	recOK := httptest.NewRecorder()
	handler.ServeHTTP(recOK, reqOK)
	if recOK.Code != http.StatusOK {
		t.Fatalf("expected 200 from /v1/parse, got %d: %s", recOK.Code, recOK.Body.String())
	}
}

func TestQuotaExhaustionAfter25Requests(t *testing.T) {
	handler := newHandler()
	rawBody := sample850Body()

	// Requests 1..25 should succeed (200) and decrement X-RateLimit-Remaining down to 0
	for i := 1; i <= 25; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/parse", bytes.NewReader(rawBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer edix_live_test_zohaib_001")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200 OK, got %d: %s", i, rec.Code, rec.Body.String())
		}

		expectedRemaining := fmt.Sprintf("%d", 25-i)
		if gotRem := rec.Header().Get("X-RateLimit-Remaining"); gotRem != expectedRemaining {
			t.Errorf("request %d: expected X-RateLimit-Remaining=%s, got %q", i, expectedRemaining, gotRem)
		}
	}

	// 26th request should be blocked with 429 Too Many Requests
	req26 := httptest.NewRequest(http.MethodPost, "/v1/parse", bytes.NewReader(rawBody))
	req26.Header.Set("Content-Type", "application/json")
	req26.Header.Set("Authorization", "Bearer edix_live_test_zohaib_001")

	rec26 := httptest.NewRecorder()
	handler.ServeHTTP(rec26, req26)

	if rec26.Code != http.StatusTooManyRequests {
		t.Fatalf("request 26: expected 429 Too Many Requests, got %d: %s", rec26.Code, rec26.Body.String())
	}
	if gotRem := rec26.Header().Get("X-RateLimit-Remaining"); gotRem != "0" {
		t.Errorf("request 26: expected X-RateLimit-Remaining=0, got %q", gotRem)
	}
}

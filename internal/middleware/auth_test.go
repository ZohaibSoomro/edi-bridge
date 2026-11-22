package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMemoryKeyStoreQuotaAndExhaustion(t *testing.T) {
	store := NewMemoryKeyStore([]*KeyRecord{
		{
			Key:          "edix_live_test_001",
			TenantID:     "tenant_demo",
			Plan:         "starter",
			MonthlyLimit: 25,
			UsedThisMo:   23,
			Active:       true,
		},
		{
			Key:          "edix_live_inactive_002",
			TenantID:     "tenant_inactive",
			Plan:         "starter",
			MonthlyLimit: 25,
			UsedThisMo:   0,
			Active:       false,
		},
	})

	// 24th request (remaining should drop from 2 -> 1)
	rec1, err := store.ValidateAndConsume("edix_live_test_001")
	if err != nil || rec1 == nil {
		t.Fatalf("expected 24th request to succeed, got rec=%+v err=%v", rec1, err)
	}
	if rec1.UsedThisMo != 24 {
		t.Errorf("expected UsedThisMo=24, got %d", rec1.UsedThisMo)
	}

	// 25th request (remaining should drop from 1 -> 0)
	rec2, err := store.ValidateAndConsume("edix_live_test_001")
	if err != nil || rec2 == nil {
		t.Fatalf("expected 25th request to succeed, got rec=%+v err=%v", rec2, err)
	}
	if rec2.UsedThisMo != 25 {
		t.Errorf("expected UsedThisMo=25, got %d", rec2.UsedThisMo)
	}

	// 26th request should hit ErrQuotaExceeded and not increment past 25
	rec3, err := store.ValidateAndConsume("edix_live_test_001")
	if err != ErrQuotaExceeded {
		t.Fatalf("expected ErrQuotaExceeded on 26th request, got rec=%+v err=%v", rec3, err)
	}
	if rec3 == nil || rec3.UsedThisMo != 25 {
		t.Errorf("expected UsedThisMo to stay at 25, got %+v", rec3)
	}

	// Inactive key should return nil, nil
	inactiveRec, err := store.ValidateAndConsume("edix_live_inactive_002")
	if err != nil || inactiveRec != nil {
		t.Errorf("expected nil record and nil error for inactive key, got rec=%+v err=%v", inactiveRec, err)
	}
}

func TestDualAuthBearerQuotaHeadersAnd429(t *testing.T) {
	store := NewMemoryKeyStore([]*KeyRecord{
		{
			Key:          "edix_live_test_001",
			TenantID:     "tenant_demo",
			Plan:         "starter",
			MonthlyLimit: 25,
			UsedThisMo:   24,
			Active:       true,
		},
	})

	okHandler := DualAuth(store, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := r.Context().Value(AuthContextKey).(Principal)
		if !ok {
			t.Fatal("expected Principal in request context")
		}
		if p.Source != "saas" || p.TenantID != "tenant_demo" || p.Remaining != 0 {
			t.Errorf("unexpected Principal: %+v", p)
		}
		w.WriteHeader(http.StatusOK)
	}))

	// 25th call succeeds and reports remaining=0
	req1 := httptest.NewRequest(http.MethodPost, "/v1/parse", nil)
	req1.Header.Set("Authorization", "Bearer edix_live_test_001")
	rec1 := httptest.NewRecorder()
	okHandler.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200 on 25th request, got %d: %s", rec1.Code, rec1.Body.String())
	}
	if gotLimit := rec1.Header().Get("X-RateLimit-Limit"); gotLimit != "25" {
		t.Errorf("expected X-RateLimit-Limit=25, got %q", gotLimit)
	}
	if gotRem := rec1.Header().Get("X-RateLimit-Remaining"); gotRem != "0" {
		t.Errorf("expected X-RateLimit-Remaining=0, got %q", gotRem)
	}

	// 26th call returns 429 Too Many Requests with remaining=0
	req2 := httptest.NewRequest(http.MethodPost, "/v1/parse", nil)
	req2.Header.Set("Authorization", "Bearer edix_live_test_001")
	rec2 := httptest.NewRecorder()
	okHandler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests when quota exhausted, got %d: %s", rec2.Code, rec2.Body.String())
	}
	if gotRem := rec2.Header().Get("X-RateLimit-Remaining"); gotRem != "0" {
		t.Errorf("expected X-RateLimit-Remaining=0 on 429, got %q", gotRem)
	}
}

func TestDualAuthRapidAPISecret(t *testing.T) {
	t.Setenv("RAPIDAPI_PROXY_SECRET", "super-secret-proxy-value")

	store := NewMemoryKeyStore(nil)
	handler := DualAuth(store, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := r.Context().Value(AuthContextKey).(Principal)
		if p.Source != "rapidapi" || p.TenantID != "rapid-user-42" {
			t.Errorf("unexpected RapidAPI Principal: %+v", p)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/parse", nil)
	req.Header.Set("X-RapidAPI-Proxy-Secret", "super-secret-proxy-value")
	req.Header.Set("X-RapidAPI-User", "rapid-user-42")
	req.Header.Set("X-RapidAPI-Subscription", "PRO")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid RapidAPI secret, got %d: %s", rec.Code, rec.Body.String())
	}
}

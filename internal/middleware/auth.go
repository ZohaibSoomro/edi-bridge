package middleware

import (
	"context"
	"crypto/subtle"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

type contextKey string

const AuthContextKey contextKey = "auth_principal"

type Principal struct {
	Source    string // "rapidapi" | "saas" | "dev"
	TenantID  string
	Plan      string
	Remaining int
}

// KeyRecord represents a SaaS subscriber's API key and monthly quota.
type KeyRecord struct {
	Key          string
	TenantID     string
	Plan         string
	MonthlyLimit int
	UsedThisMo   int
	Active       bool
}

// KeyStore abstracts API key lookup & atomic quota increment (in-memory for dev, Cosmos DB for prod).
type KeyStore interface {
	ValidateAndConsume(apiKey string) (*KeyRecord, error)
}

// MemoryKeyStore provides a thread-safe store for local testing and single-instance deployments.
type MemoryKeyStore struct {
	mu   sync.Mutex
	keys map[string]*KeyRecord
}

func NewMemoryKeyStore(seed []*KeyRecord) *MemoryKeyStore {
	m := make(map[string]*KeyRecord, len(seed))
	for _, k := range seed {
		m[k.Key] = k
	}
	return &MemoryKeyStore{keys: m}
}

func (s *MemoryKeyStore) ValidateAndConsume(apiKey string) (*KeyRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.keys[apiKey]
	if !ok || !rec.Active {
		return nil, nil
	}
	if rec.MonthlyLimit > 0 && rec.UsedThisMo >= rec.MonthlyLimit {
		return rec, ErrQuotaExceeded
	}
	rec.UsedThisMo++
	copyRec := *rec
	return &copyRec, nil
}

var ErrQuotaExceeded = http.ErrAbortHandler

// DualAuth enforces either X-RapidAPI-Proxy-Secret or Authorization: Bearer edix_live_*
func DualAuth(store KeyStore, next http.Handler) http.Handler {
	rapidSecret := os.Getenv("RAPIDAPI_PROXY_SECRET")
	devBypass := strings.EqualFold(os.Getenv("DEV_BYPASS_AUTH"), "true")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow CORS preflight requests from your Azure Static Web App frontend
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-RapidAPI-Proxy-Secret")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// 1. Check RapidAPI Proxy Secret
		incomingRapid := r.Header.Get("X-RapidAPI-Proxy-Secret")
		if rapidSecret != "" && incomingRapid != "" {
			if subtle.ConstantTimeCompare([]byte(incomingRapid), []byte(rapidSecret)) == 1 {
				rapidUser := r.Header.Get("X-RapidAPI-User")
				if rapidUser == "" {
					rapidUser = "rapidapi-anonymous"
				}
				ctx := context.WithValue(r.Context(), AuthContextKey, Principal{
					Source:   "rapidapi",
					TenantID: rapidUser,
					Plan:     r.Header.Get("X-RapidAPI-Subscription"),
				})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			writeJSONError(w, http.StatusUnauthorized, "invalid RapidAPI proxy secret")
			return
		}

		// 2. Check Direct SaaS Bearer API Key
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
			rec, err := store.ValidateAndConsume(token)
			if err == ErrQuotaExceeded {
				w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rec.MonthlyLimit))
				w.Header().Set("X-RateLimit-Remaining", "0")
				writeJSONError(w, http.StatusTooManyRequests, "monthly API quota exceeded; upgrade plan in dashboard")
				return
			}
			if rec == nil {
				writeJSONError(w, http.StatusUnauthorized, "invalid or inactive API key")
				return
			}

			remaining := rec.MonthlyLimit - rec.UsedThisMo
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rec.MonthlyLimit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

			ctx := context.WithValue(r.Context(), AuthContextKey, Principal{
				Source:    "saas",
				TenantID:  rec.TenantID,
				Plan:      rec.Plan,
				Remaining: remaining,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// 3. Optional Local Dev Bypass
		if devBypass {
			ctx := context.WithValue(r.Context(), AuthContextKey, Principal{
				Source:   "dev",
				TenantID: "local-dev",
				Plan:     "scale",
			})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		writeJSONError(w, http.StatusUnauthorized, "missing credentials: provide Authorization: Bearer <api_key> or route via RapidAPI")
	})
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"success":false,"error":"` + msg + `"}`))
}

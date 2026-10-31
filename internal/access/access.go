// Package access protects the unlisted-board API. A board URL remains a bearer
// link: anyone holding it can read and edit; origin checks are not authentication.
package access

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func ValidateOrigin(value string) error {
	if value == "" {
		return nil
	}
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return fmt.Errorf("PUBLIC_ORIGIN must be an http(s) origin without a path, credentials or query")
	}
	return nil
}

// No forwarded headers are trusted. Set PUBLIC_ORIGIN behind a TLS proxy.
func OriginAllowed(r *http.Request, publicOrigin string) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	origins := r.Header.Values("Origin")
	if len(origins) == 0 {
		return true
	} // Native clients do not send Origin.
	expected := publicOrigin
	if expected == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		expected = scheme + "://" + r.Host
	}
	return len(origins) == 1 && strings.EqualFold(origins[0], expected)
}

func SameOrigin(publicOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !OriginAllowed(r, publicOrigin) {
				http.Error(w, "Origin denied", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func Headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		next.ServeHTTP(w, r)
	})
}

// Board IDs and query strings are bearer links and must not enter access logs.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		defer func() {
			pattern := chi.RouteContext(r.Context()).RoutePattern()
			if pattern == "" {
				pattern = "unmatched"
			}
			log.Printf("HTTP %s %s status=%d elapsed=%s", r.Method, pattern, wrapped.Status(), time.Since(started))
		}()
		next.ServeHTTP(wrapped, r)
	})
}

type Counter interface {
	// Take atomically consumes one request and returns remaining window duration.
	Take(context.Context, string, int, time.Duration) (bool, time.Duration, error)
}

type Policy struct {
	Name   string
	Limit  int
	Window time.Duration
}

// Peers can be a whole classroom behind NAT. Keep the general budget generous;
// expensive creation and connection churn have their own smaller budgets.
var API = Policy{"api", 6000, time.Minute}
var Create = Policy{"create", 30, time.Minute}
var Socket = Policy{"socket", 120, time.Minute}

func Limit(counter Counter, policy Policy) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// RemoteAddr is the actual peer. X-Forwarded-For/X-Real-IP are untrusted.
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				http.Error(w, "Invalid peer address", http.StatusBadRequest)
				return
			}
			if ip := net.ParseIP(host); ip != nil {
				host = ip.String()
			}
			key := fmt.Sprintf("limits:v1:%s:%x", policy.Name, sha256.Sum256([]byte(host)))
			ctx, cancel := context.WithTimeout(r.Context(), time.Second)
			defer cancel()
			allowed, retry, err := counter.Take(ctx, key, policy.Limit, policy.Window)
			w.Header().Set("Cache-Control", "no-store")
			if err != nil {
				w.Header().Set("Retry-After", "2")
				http.Error(w, "Request protection unavailable", http.StatusServiceUnavailable)
				return
			}
			if !allowed {
				seconds := int64((retry + time.Second - 1) / time.Second)
				if seconds < 1 {
					seconds = 1
				}
				w.Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
				http.Error(w, "Too many requests; try again later", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

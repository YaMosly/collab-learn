package config

import (
	"net/url"
	"strings"
	"testing"
)

func TestDatabaseCredentialsRoundTrip(t *testing.T) {
	values := map[string]string{"DB_USER": "user @:/", "DB_PASSWORD": "p@ss ' &/?=#%:word", "DB_NAME": "db/name ?", "DB_HOST": "::1"}
	cfg, err := Load(func(key string) string { return values[key] })
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	password, _ := u.User.Password()
	if password != values["DB_PASSWORD"] || u.User.Username() != values["DB_USER"] || u.Path != "/"+values["DB_NAME"] || u.Host != "[::1]:5432" {
		t.Fatal("database credentials did not round-trip")
	}
	if u.Query().Get("connect_timeout") != "2" {
		t.Fatal("missing connection timeout")
	}
}

func TestInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"DB_PASSWORD", ""}, {"DB_PORT", "0"}, {"REDIS_PORT", "70000"}, {"PORT", "abc"},
		{"HTTP_HOST", "bad/host"}, {"DB_SSLMODE", "bad"}, {"STARTUP_TIMEOUT", "1ms"}, {"STARTUP_TIMEOUT", "6m"},
		{"PUBLIC_ORIGIN", "https://example.org/path"},
	} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			_, err := Load(func(key string) string {
				if key == tc.key {
					return tc.value
				}
				if key == "DB_PASSWORD" {
					return "secret"
				}
				return ""
			})
			if err == nil || !strings.Contains(err.Error(), tc.key) || strings.Contains(err.Error(), "secret") {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestProbeUsesLoopback(t *testing.T) {
	for _, tc := range []struct{ host, expected string }{{"0.0.0.0", "127.0.0.1:8080"}, {"::", "[::1]:8080"}, {"127.0.0.2", "127.0.0.2:8080"}} {
		cfg, err := Load(func(key string) string {
			if key == "HTTP_HOST" {
				return tc.host
			}
			if key == "DB_PASSWORD" {
				return "secret"
			}
			return ""
		})
		if err != nil || cfg.ProbeURL != "http://"+tc.expected+"/readyz" {
			t.Fatalf("probe=%s err=%v", cfg.ProbeURL, err)
		}
	}
}

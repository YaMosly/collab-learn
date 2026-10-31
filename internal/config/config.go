package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddress, ProbeURL, DatabaseURL, RedisAddress string
	StartupTimeout                                   time.Duration
}

// Load validates configuration without including secret values in errors.
func Load(getenv func(string) string) (Config, error) {
	var c Config
	value := func(key, fallback string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return fallback
	}
	host := value("HTTP_HOST", "0.0.0.0")
	port := value("PORT", "8080")
	if err := validatePort("PORT", port); err != nil {
		return c, err
	}
	if host != "localhost" && net.ParseIP(host) == nil {
		return c, fmt.Errorf("HTTP_HOST must be an IP address or localhost")
	}
	c.HTTPAddress = net.JoinHostPort(host, port)
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		if ip.To4() != nil {
			host = "127.0.0.1"
		} else {
			host = "::1"
		}
	}
	c.ProbeURL = "http://" + net.JoinHostPort(host, port) + "/readyz"
	dbPort, redisPort := value("DB_PORT", "5432"), value("REDIS_PORT", "6379")
	if err := validatePort("DB_PORT", dbPort); err != nil {
		return c, err
	}
	if err := validatePort("REDIS_PORT", redisPort); err != nil {
		return c, err
	}
	password := getenv("DB_PASSWORD")
	if password == "" {
		return c, fmt.Errorf("DB_PASSWORD is required")
	}
	sslmode := value("DB_SSLMODE", "disable")
	switch sslmode {
	case "disable", "require", "verify-ca", "verify-full":
	default:
		return c, fmt.Errorf("invalid DB_SSLMODE")
	}
	u := &url.URL{Scheme: "postgres", Host: net.JoinHostPort(value("DB_HOST", "localhost"), dbPort),
		User: url.UserPassword(value("DB_USER", "collablearn"), password), Path: "/" + value("DB_NAME", "collablearn")}
	q := url.Values{"sslmode": {sslmode}, "connect_timeout": {"2"}}
	u.RawQuery = q.Encode()
	c.DatabaseURL = u.String()
	c.RedisAddress = net.JoinHostPort(value("REDIS_HOST", "localhost"), redisPort)
	timeout, err := time.ParseDuration(value("STARTUP_TIMEOUT", "45s"))
	if err != nil || timeout < time.Second || timeout > 5*time.Minute {
		return c, fmt.Errorf("STARTUP_TIMEOUT must be between 1s and 5m")
	}
	c.StartupTimeout = timeout
	return c, nil
}

func validatePort(key, value string) error {
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("%s must be a port between 1 and 65535", key)
	}
	return nil
}

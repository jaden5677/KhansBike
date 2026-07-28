// Package config loads and validates all runtime configuration from the
// process environment into a single immutable Config value.
//
// Why fail-fast-with-aggregation: a shop owner deploying a single .exe should
// learn about every misconfiguration in one run, not fix one variable, restart,
// and discover the next. Load therefore collects all problems and returns them
// joined, rather than bailing on the first bad variable.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// Backend enumerates the supported blob storage backends.
const (
	MediaBackendFS = "fs"
	MediaBackendR2 = "r2"
)

// Config is the fully-parsed, validated configuration for every binary in the
// project. It is constructed once at startup and treated as read-only
// thereafter; nothing mutates it, so it is safe to share across goroutines.
type Config struct {
	AppEnv        string
	HTTPAddr      string
	PublicBaseURL string

	DatabaseURL string
	DBMaxConns  int32

	SessionCookieName string
	SessionTTL        time.Duration
	CSRFKey           []byte // decoded 32 bytes

	MediaBackend         string
	MediaFSRoot          string
	MediaMaxUploadBytes  int64
	MediaMaxPixels       int64
	MediaRenditionWidths []int

	R2AccountID       string
	R2Bucket          string
	R2AccessKeyID     string
	R2SecretAccessKey string
	R2PublicBase      string

	WorkerEnabled     bool
	WorkerConcurrency int

	TrustCloudflareIPs bool

	LogLevel  slog.Level
	LogFormat string
}

// IsProduction reports whether the process is running in the production
// environment, which callers use to pick stricter defaults (secure cookies,
// JSON logs) without re-parsing APP_ENV.
func (c *Config) IsProduction() bool { return c.AppEnv == "production" }

// Getenv matches os.Getenv so tests can inject a fake environment without
// touching real process state.
type Getenv func(string) string

// Load parses configuration using getenv (pass os.Getenv in production) and
// returns either a valid Config or an error that aggregates every problem
// discovered, one per line.
func Load(getenv Getenv) (*Config, error) {
	p := &parser{getenv: getenv}
	c := &Config{}

	c.AppEnv = p.str("APP_ENV", "development")
	c.HTTPAddr = p.str("HTTP_ADDR", "127.0.0.1:8080")
	c.PublicBaseURL = p.required("PUBLIC_BASE_URL")

	c.DatabaseURL = p.required("DATABASE_URL")
	c.DBMaxConns = int32(p.intDefault("DB_MAX_CONNS", 20))

	c.SessionCookieName = p.str("SESSION_COOKIE_NAME", "bz_session")
	c.SessionTTL = p.durationDefault("SESSION_TTL", 720*time.Hour)
	c.CSRFKey = p.base64Bytes("CSRF_KEY", 32)

	c.MediaBackend = p.enum("MEDIA_BACKEND", MediaBackendFS, MediaBackendFS, MediaBackendR2)
	c.MediaFSRoot = p.str("MEDIA_FS_ROOT", "")
	c.MediaMaxUploadBytes = p.int64Default("MEDIA_MAX_UPLOAD_BYTES", 26214400)
	c.MediaMaxPixels = p.int64Default("MEDIA_MAX_PIXELS", 50000000)
	c.MediaRenditionWidths = p.intList("MEDIA_RENDITION_WIDTHS", []int{320, 640, 960, 1280, 1920})

	c.R2AccountID = p.str("R2_ACCOUNT_ID", "")
	c.R2Bucket = p.str("R2_BUCKET", "")
	c.R2AccessKeyID = p.str("R2_ACCESS_KEY_ID", "")
	c.R2SecretAccessKey = p.str("R2_SECRET_ACCESS_KEY", "")
	c.R2PublicBase = p.str("R2_PUBLIC_BASE", "")

	c.WorkerEnabled = p.boolDefault("WORKER_ENABLED", true)
	c.WorkerConcurrency = p.intDefault("WORKER_CONCURRENCY", 2)

	c.TrustCloudflareIPs = p.boolDefault("TRUST_CLOUDFLARE_IPS", true)

	c.LogLevel = p.logLevel("LOG_LEVEL", slog.LevelInfo)
	c.LogFormat = p.enum("LOG_FORMAT", "json", "json", "text")

	// Cross-field validation that only makes sense once fields are parsed.
	if c.MediaBackend == MediaBackendFS && c.MediaFSRoot == "" {
		p.errf("MEDIA_FS_ROOT is required when MEDIA_BACKEND=fs")
	}
	if c.MediaBackend == MediaBackendR2 {
		for _, k := range []string{"R2_ACCOUNT_ID", "R2_BUCKET", "R2_ACCESS_KEY_ID", "R2_SECRET_ACCESS_KEY"} {
			if p.getenv(k) == "" {
				p.errf("%s is required when MEDIA_BACKEND=r2", k)
			}
		}
	}

	if err := p.err(); err != nil {
		return nil, err
	}
	return c, nil
}

// parser accumulates validation errors while pulling typed values from the
// environment, so a single Load call can report everything wrong at once.
type parser struct {
	getenv Getenv
	errs   []error
}

func (p *parser) errf(format string, a ...any) { p.errs = append(p.errs, fmt.Errorf(format, a...)) }

func (p *parser) err() error {
	if len(p.errs) == 0 {
		return nil
	}
	return fmt.Errorf("invalid configuration:\n%w", errors.Join(p.errs...))
}

func (p *parser) str(key, def string) string {
	if v := strings.TrimSpace(p.getenv(key)); v != "" {
		return v
	}
	return def
}

func (p *parser) required(key string) string {
	v := strings.TrimSpace(p.getenv(key))
	if v == "" {
		p.errf("%s is required", key)
	}
	return v
}

func (p *parser) intDefault(key string, def int) int {
	raw := strings.TrimSpace(p.getenv(key))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		p.errf("%s must be an integer, got %q", key, raw)
		return def
	}
	return n
}

func (p *parser) int64Default(key string, def int64) int64 {
	raw := strings.TrimSpace(p.getenv(key))
	if raw == "" {
		return def
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		p.errf("%s must be an integer, got %q", key, raw)
		return def
	}
	return n
}

func (p *parser) boolDefault(key string, def bool) bool {
	raw := strings.TrimSpace(p.getenv(key))
	if raw == "" {
		return def
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		p.errf("%s must be a boolean (true/false), got %q", key, raw)
		return def
	}
	return b
}

func (p *parser) durationDefault(key string, def time.Duration) time.Duration {
	raw := strings.TrimSpace(p.getenv(key))
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		p.errf("%s must be a Go duration (e.g. 720h), got %q", key, raw)
		return def
	}
	return d
}

func (p *parser) enum(key, def string, allowed ...string) string {
	v := p.str(key, def)
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	p.errf("%s must be one of %s, got %q", key, strings.Join(allowed, "|"), v)
	return def
}

func (p *parser) intList(key string, def []int) []int {
	raw := strings.TrimSpace(p.getenv(key))
	if raw == "" {
		return def
	}
	parts := strings.Split(raw, ",")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			p.errf("%s must be a comma-separated list of integers, got %q", key, raw)
			return def
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return def
	}
	return out
}

func (p *parser) base64Bytes(key string, want int) []byte {
	raw := strings.TrimSpace(p.getenv(key))
	if raw == "" {
		p.errf("%s is required (%d random bytes, base64-encoded)", key, want)
		return nil
	}
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		p.errf("%s must be valid base64: %v", key, err)
		return nil
	}
	if len(b) != want {
		p.errf("%s must decode to %d bytes, got %d", key, want, len(b))
		return nil
	}
	return b
}

func (p *parser) logLevel(key string, def slog.Level) slog.Level {
	raw := strings.ToLower(strings.TrimSpace(p.getenv(key)))
	switch raw {
	case "":
		return def
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		p.errf("%s must be one of debug|info|warn|error, got %q", key, raw)
		return def
	}
}

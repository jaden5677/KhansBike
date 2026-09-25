package config

import (
	"strings"
	"testing"
	"time"
)

// validEnv is the smallest environment Load accepts.
func validEnv() map[string]string {
	return map[string]string{
		"PUBLIC_BASE_URL": "https://khansbikezone.example/",
		"DATABASE_URL":    "postgres://u:p@127.0.0.1:5432/db",
		"CSRF_KEY":        "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=", // 32 bytes
		"MEDIA_FS_ROOT":   "/tmp/media",
	}
}

func load(env map[string]string) (*Config, error) {
	return Load(func(k string) string { return env[k] })
}

func TestLoadDefaults(t *testing.T) {
	c, err := load(validEnv())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.PublicBaseURL != "https://khansbikezone.example" {
		t.Errorf("PublicBaseURL = %q, want the trailing slash stripped", c.PublicBaseURL)
	}
	if c.SessionTTL != 720*time.Hour || c.WorkerConcurrency != 2 || c.EmailBackend != EmailBackendLog || len(c.CSRFKey) != 32 {
		t.Errorf("unexpected defaults: %+v", c)
	}
}

func TestLoadReportsEveryProblemAtOnce(t *testing.T) {
	env := validEnv()
	delete(env, "DATABASE_URL")
	env["PUBLIC_BASE_URL"] = "khansbikezone.example" // no scheme
	env["CSRF_KEY"] = "c2hvcnQ="                     // too short
	env["WORKER_CONCURRENCY"] = "0"
	env["EMAIL_BACKEND"] = "smtp" // without SMTP_HOST / EMAIL_FROM
	_, err := load(env)
	if err == nil {
		t.Fatal("Load accepted an invalid environment")
	}
	for _, want := range []string{"DATABASE_URL", "PUBLIC_BASE_URL", "CSRF_KEY", "WORKER_CONCURRENCY", "SMTP_HOST", "EMAIL_FROM"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %s:\n%v", want, err)
		}
	}
}

func TestLoadR2RequiresCredentials(t *testing.T) {
	env := validEnv()
	env["MEDIA_BACKEND"] = "r2"
	env["R2_BUCKET"] = "   " // blank, as make leaves `R2_BUCKET= # comment`
	_, err := load(env)
	if err == nil || !strings.Contains(err.Error(), "R2_BUCKET") {
		t.Errorf("r2 without credentials: err = %v", err)
	}
}

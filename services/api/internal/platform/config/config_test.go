package config

import "testing"

func TestConfigRejectsMissingDatabaseAndUnsafeOrigin(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WEB_ORIGIN", "http://localhost:3000")
	if _, err := Load(); err == nil {
		t.Fatal("missing database accepted")
	}
	t.Setenv("DATABASE_URL", "postgres://localhost/attendance")
	t.Setenv("WEB_ORIGIN", "*")
	if _, err := Load(); err == nil {
		t.Fatal("wildcard origin accepted")
	}
	t.Setenv("WEB_ORIGIN", "https://attendance.example.com/path")
	if _, err := Load(); err == nil {
		t.Fatal("non-origin URL accepted")
	}
	t.Setenv("WEB_ORIGIN", "https://attendance.example.com")
	t.Setenv("API_ADDRESS", "127.0.0.1:8080")
	cfg, err := Load()
	if err != nil || cfg.Address != "127.0.0.1:8080" {
		t.Fatalf("valid config failed: %v", err)
	}
}

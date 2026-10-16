package config

import "testing"

func TestConfigValidatesTrustedProxyCIDRs(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/attendance")
	t.Setenv("WEB_ORIGIN", "http://localhost:3000")
	t.Setenv("ALLOW_LOCAL_WEB_PORT_RANGE", "0")
	for _, value := range []string{"not-a-cidr", "0.0.0.0/0", "::/0", "10.0.0.1"} {
		t.Setenv("TRUSTED_PROXY_CIDRS", value)
		if _, err := Load(); err == nil {
			t.Fatalf("unsafe proxy config accepted: %q", value)
		}
	}
	t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.0.2/32, fd00::/64")
	cfg, err := Load()
	if err != nil || len(cfg.TrustedProxies) != 2 {
		t.Fatalf("trusted proxy config failed: %v", err)
	}
}

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

func TestConfigNeedsNoSchoolEnvironmentAndRequiresPublicHTTPS(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/attendance")
	t.Setenv("WEB_ORIGIN", "https://attendance.example.com")
	if _, err := Load(); err != nil {
		t.Fatalf("school environment is unnecessary: %v", err)
	}
	t.Setenv("WEB_ORIGIN", "http://attendance.example.com")
	if _, err := Load(); err == nil {
		t.Fatal("public insecure auth origin accepted")
	}
}

func TestPublicAndLocalConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/attendance")
	t.Setenv("API_ADDRESS", "127.0.0.1:8080")
	t.Setenv("ALLOW_LOCAL_WEB_PORT_RANGE", "0")
	for _, origin := range []string{"http://localhost:3000", "https://attendance.example.com"} {
		t.Setenv("WEB_ORIGIN", origin)
		if _, err := Load(); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("ALLOW_LOCAL_WEB_PORT_RANGE", "1")
	for _, origin := range []string{"http://localhost:3001", "http://localhost:3010"} {
		t.Setenv("WEB_ORIGIN", origin)
		t.Setenv("ALLOW_LOCAL_WEB_PORT_RANGE", "1")
		cfg, err := Load()
		if err != nil || !cfg.AllowLocalWebPortRange {
			t.Fatalf("fallback localhost origin %q was not enabled: %v", origin, err)
		}
	}
	for _, origin := range []string{"https://attendance.example.com", "http://127.0.0.1:3002", "http://localhost:3000", "http://localhost:3011"} {
		t.Setenv("WEB_ORIGIN", origin)
		t.Setenv("ALLOW_LOCAL_WEB_PORT_RANGE", "1")
		if _, err := Load(); err == nil {
			t.Fatalf("unsafe local allowlist origin %q was accepted", origin)
		}
	}
}

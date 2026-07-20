package config

import "testing"

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		envDataDir, envPort, envJWTSecretKey, EnvStorageURL, envAdminAPIToken,
		envMQTTPort, envHashSchemaVersion, envRegistrationOpen, envICEServers,
		envTLSCert, envTLSKey, envHTTPSCookie, envTrustProxy,
	} {
		t.Setenv(k, "")
	}
}

func TestFromEnvDefaults(t *testing.T) {
	clearConfigEnv(t)

	cfg := FromEnv()

	if cfg.Port != DefaultPort {
		t.Errorf("Port: expected %q, got %q", DefaultPort, cfg.Port)
	}
	if cfg.MQTTPort != "8883" {
		t.Errorf("MQTTPort: expected 8883, got %q", cfg.MQTTPort)
	}
	if cfg.HashSchemaVersion != "3" {
		t.Errorf("HashSchemaVersion: expected 3, got %q", cfg.HashSchemaVersion)
	}
	if cfg.AdminAPIToken != "" {
		t.Errorf("AdminAPIToken: expected empty (admin API disabled), got %q", cfg.AdminAPIToken)
	}
	if !cfg.JWTRandom {
		t.Error("JWTRandom: expected true when JWT_SECRET_KEY unset")
	}
	if cfg.StorageURL != "https://"+DefaultHost {
		t.Errorf("StorageURL: expected https://%s, got %q", DefaultHost, cfg.StorageURL)
	}
	if cfg.CloudHost != DefaultHost {
		t.Errorf("CloudHost: expected %q, got %q", DefaultHost, cfg.CloudHost)
	}
	if cfg.RegistrationOpen {
		t.Error("RegistrationOpen: expected false by default")
	}
	if len(cfg.ICEServers) != 1 {
		t.Fatalf("ICEServers: expected 1 default stun entry, got %d", len(cfg.ICEServers))
	}
}

func TestFromEnvOverrides(t *testing.T) {
	clearConfigEnv(t)
	dataDir := t.TempDir()
	t.Setenv(envPort, "8080")
	t.Setenv(envDataDir, dataDir)
	t.Setenv(envJWTSecretKey, "test-secret")
	t.Setenv(EnvStorageURL, "https://rm.example.com")
	t.Setenv(envAdminAPIToken, "tok123")
	t.Setenv(envMQTTPort, "9993")
	t.Setenv(envHashSchemaVersion, "4")
	t.Setenv(envRegistrationOpen, "true")

	cfg := FromEnv()

	if cfg.Port != "8080" {
		t.Errorf("Port: expected 8080, got %q", cfg.Port)
	}
	if cfg.DataDir != dataDir {
		t.Errorf("DataDir: expected %q, got %q", dataDir, cfg.DataDir)
	}
	if cfg.JWTRandom {
		t.Error("JWTRandom: expected false when JWT_SECRET_KEY set")
	}
	if cfg.CloudHost != "rm.example.com" {
		t.Errorf("CloudHost: expected rm.example.com, got %q", cfg.CloudHost)
	}
	if cfg.AdminAPIToken != "tok123" {
		t.Errorf("AdminAPIToken: expected tok123, got %q", cfg.AdminAPIToken)
	}
	if cfg.MQTTPort != "9993" {
		t.Errorf("MQTTPort: expected 9993, got %q", cfg.MQTTPort)
	}
	if cfg.HashSchemaVersion != "4" {
		t.Errorf("HashSchemaVersion: expected 4, got %q", cfg.HashSchemaVersion)
	}
	if !cfg.RegistrationOpen {
		t.Error("RegistrationOpen: expected true")
	}
}

// xochitl only accepts singular "url" entries, so "urls" arrays must be expanded.
func TestFromEnvICEServersNormalized(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv(envICEServers, `[{"urls":["stun:stun.example.com:3478","turn:turn.example.com:3478"],"username":"u","credential":"c"}]`)

	cfg := FromEnv()

	if len(cfg.ICEServers) != 2 {
		t.Fatalf("expected urls array expanded to 2 entries, got %d: %+v", len(cfg.ICEServers), cfg.ICEServers)
	}
	for i, s := range cfg.ICEServers {
		m, ok := s.(map[string]interface{})
		if !ok {
			t.Fatalf("entry %d: expected map, got %T", i, s)
		}
		if _, has := m["urls"]; has {
			t.Errorf("entry %d: plural \"urls\" key must not survive normalization: %+v", i, m)
		}
		if u, _ := m["url"].(string); u == "" {
			t.Errorf("entry %d: missing singular url: %+v", i, m)
		}
		if m["username"] != "u" || m["credential"] != "c" {
			t.Errorf("entry %d: credentials not preserved: %+v", i, m)
		}
	}
}

func TestFromEnvICEServersInvalidJSONFallsBack(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv(envICEServers, "{not json")

	cfg := FromEnv()

	if len(cfg.ICEServers) != 1 {
		t.Fatalf("expected fallback to default stun entry, got %+v", cfg.ICEServers)
	}
}

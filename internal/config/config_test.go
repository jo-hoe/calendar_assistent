package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_ValidConfig(t *testing.T) {
	content := `
server:
  address: ":9090"
  readTimeout: "10s"
  writeTimeout: "30s"
  idleTimeout: "45s"
  maxUpload: "5MiB"
  apiKey: "test-key"
  logLevel: "debug"
llm:
  provider: "aiproxy"
  aiproxy:
    baseUrl: "http://localhost:11434"
    apiKey: "sk-test"
    model: "gpt-4o"
    temperature: 0.5
    maxTokens: 2048
calendar:
  provider: "google"
  google:
    credentialsFile: "/app/secrets/creds.json"
    calendarId: "my-cal@group.calendar.google.com"
    timeZone: "Europe/Berlin"
`
	path := writeTempConfig(t, content)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Server.Address != ":9090" {
		t.Errorf("Address = %q, want %q", cfg.Server.Address, ":9090")
	}
	if cfg.Server.ReadTimeout.Duration != 10*time.Second {
		t.Errorf("ReadTimeout = %v, want 10s", cfg.Server.ReadTimeout.Duration)
	}
	if cfg.Server.WriteTimeout.Duration != 30*time.Second {
		t.Errorf("WriteTimeout = %v, want 30s", cfg.Server.WriteTimeout.Duration)
	}
	if cfg.Server.MaxUpload != 5*1024*1024 {
		t.Errorf("MaxUpload = %d, want %d", cfg.Server.MaxUpload, 5*1024*1024)
	}
	if cfg.Server.APIKey != "test-key" {
		t.Errorf("APIKey = %q, want %q", cfg.Server.APIKey, "test-key")
	}
	if cfg.LLM.Provider != LLMProviderAIProxy {
		t.Errorf("LLM.Provider = %q, want %q", cfg.LLM.Provider, LLMProviderAIProxy)
	}
	if cfg.LLM.AIProxy.BaseURL != "http://localhost:11434" {
		t.Errorf("AIProxy.BaseURL = %q, want %q", cfg.LLM.AIProxy.BaseURL, "http://localhost:11434")
	}
	if cfg.LLM.AIProxy.Temperature != 0.5 {
		t.Errorf("AIProxy.Temperature = %f, want 0.5", cfg.LLM.AIProxy.Temperature)
	}
	if cfg.Calendar.Google.CalendarID != "my-cal@group.calendar.google.com" {
		t.Errorf("CalendarID = %q, want %q", cfg.Calendar.Google.CalendarID, "my-cal@group.calendar.google.com")
	}
	if cfg.Calendar.Google.TimeZone != "Europe/Berlin" {
		t.Errorf("TimeZone = %q, want %q", cfg.Calendar.Google.TimeZone, "Europe/Berlin")
	}
}

func TestLoad_Defaults(t *testing.T) {
	content := `
llm:
  provider: "mock"
calendar:
  provider: "google"
  google:
    credentialsFile: "/tmp/creds.json"
`
	path := writeTempConfig(t, content)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Server.Address != ":8080" {
		t.Errorf("default Address = %q, want %q", cfg.Server.Address, ":8080")
	}
	if cfg.Server.ReadTimeout.Duration != 15*time.Second {
		t.Errorf("default ReadTimeout = %v, want 15s", cfg.Server.ReadTimeout.Duration)
	}
	if cfg.Server.MaxUpload != 10*1024*1024 {
		t.Errorf("default MaxUpload = %d, want %d", cfg.Server.MaxUpload, 10*1024*1024)
	}
	if cfg.Calendar.Google.CalendarID != "primary" {
		t.Errorf("default CalendarID = %q, want %q", cfg.Calendar.Google.CalendarID, "primary")
	}
}

func TestLoad_EnvExpansion(t *testing.T) {
	t.Setenv("TEST_API_KEY", "expanded-key")

	content := `
llm:
  provider: "mock"
calendar:
  provider: "google"
  google:
    credentialsFile: "/tmp/creds.json"
server:
  apiKey: "${TEST_API_KEY}"
`
	path := writeTempConfig(t, content)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Server.APIKey != "expanded-key" {
		t.Errorf("APIKey = %q, want %q", cfg.Server.APIKey, "expanded-key")
	}
}

func TestLoad_InvalidProvider(t *testing.T) {
	content := `
llm:
  provider: "invalid"
calendar:
  provider: "google"
  google:
    credentialsFile: "/tmp/creds.json"
`
	path := writeTempConfig(t, content)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for invalid provider")
	}
}

func TestLoad_MissingCredentials(t *testing.T) {
	content := `
llm:
  provider: "mock"
calendar:
  provider: "google"
  google:
    calendarId: "primary"
`
	path := writeTempConfig(t, content)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for missing credentialsFile")
	}
}

func TestLoad_AiproxyMissingBaseURL(t *testing.T) {
	content := `
llm:
  provider: "aiproxy"
  aiproxy:
    apiKey: "test"
calendar:
  provider: "google"
  google:
    credentialsFile: "/tmp/creds.json"
`
	path := writeTempConfig(t, content)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for missing aiproxy.baseUrl")
	}
}

func TestParseByteSize(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"1024", 1024},
		{"10MiB", 10 * 1024 * 1024},
		{"5Mi", 5 * 1024 * 1024},
		{"1GiB", 1 << 30},
		{"500KB", 500_000},
		{"2MB", 2_000_000},
		{"100B", 100},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseByteSize(tc.input)
			if err != nil {
				t.Fatalf("parseByteSize(%q) error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("parseByteSize(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseByteSize_Invalid(t *testing.T) {
	inputs := []string{"", "abc", "10XB"}
	for _, s := range inputs {
		t.Run(s, func(t *testing.T) {
			_, err := parseByteSize(s)
			if err == nil {
				t.Errorf("parseByteSize(%q) expected error", s)
			}
		})
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}
	return path
}

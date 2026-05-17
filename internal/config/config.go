package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	LLM      LLMConfig      `yaml:"llm"`
	Calendar CalendarConfig `yaml:"calendar"`
}

type ServerConfig struct {
	Address      string   `yaml:"address"`
	ReadTimeout  Duration `yaml:"readTimeout"`
	WriteTimeout Duration `yaml:"writeTimeout"`
	IdleTimeout  Duration `yaml:"idleTimeout"`
	APIKey       string   `yaml:"apiKey"`
	MaxUpload    ByteSize `yaml:"maxUpload"`
	LogLevel     string   `yaml:"logLevel"`
}

type LLMConfig struct {
	Provider string        `yaml:"provider"`
	AIProxy  AIProxyConfig `yaml:"aiproxy"`
}

type AIProxyConfig struct {
	BaseURL      string  `yaml:"baseUrl"`
	APIKey       string  `yaml:"apiKey"`
	Model        string  `yaml:"model"`
	SystemPrompt string  `yaml:"systemPrompt"`
	Temperature  float64 `yaml:"temperature"`
	MaxTokens    int     `yaml:"maxTokens"`
}

type CalendarConfig struct {
	Provider string               `yaml:"provider"`
	Google   GoogleCalendarConfig `yaml:"google"`
}

type GoogleCalendarConfig struct {
	CredentialsFile string `yaml:"credentialsFile"`
	CalendarID      string `yaml:"calendarId"`
	TimeZone        string `yaml:"timeZone"`
}

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = parsed
	return nil
}

func (d Duration) MarshalYAML() (any, error) {
	return d.String(), nil
}

type ByteSize int64

func (b *ByteSize) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := parseByteSize(s)
	if err != nil {
		return err
	}
	*b = ByteSize(parsed)
	return nil
}

func parseByteSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty byte size")
	}

	units := []struct {
		suffix     string
		multiplier int64
	}{
		{"GiB", 1 << 30},
		{"MiB", 1 << 20},
		{"KiB", 1 << 10},
		{"Gi", 1 << 30},
		{"Mi", 1 << 20},
		{"Ki", 1 << 10},
		{"GB", 1_000_000_000},
		{"MB", 1_000_000},
		{"KB", 1_000},
		{"B", 1},
	}

	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			numStr := strings.TrimSpace(s[:len(s)-len(u.suffix)])
			n, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid byte size %q: %w", s, err)
			}
			return int64(n * float64(u.multiplier)), nil
		}
	}

	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid byte size %q: %w", s, err)
	}
	return n, nil
}

const (
	defaultAddress      = ":8080"
	defaultReadTimeout  = 15 * time.Second
	defaultWriteTimeout = 2 * time.Minute
	defaultIdleTimeout  = 60 * time.Second
	defaultMaxUpload    = 10 * 1024 * 1024 // 10 MiB
	defaultLLMProvider  = "mock"
	defaultCalProvider  = "google"
	defaultCalendarID   = "primary"
	defaultTimeZone     = "UTC"
	defaultLogLevel     = "info"
	defaultTemperature  = 0.2
	defaultMaxTokens    = 4096
)

func Load(path string) (*Config, error) {
	if path == "" {
		path = os.Getenv("CALENDAR_ASSISTENT_CONFIG")
	}
	if path == "" {
		path = "config.yaml"
	}

	data, err := os.ReadFile(path) //nolint:gosec // path is from trusted config
	if err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}

	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %q: %w", path, err)
	}

	applyDefaults(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Address == "" {
		cfg.Server.Address = defaultAddress
	}
	if cfg.Server.ReadTimeout.Duration == 0 {
		cfg.Server.ReadTimeout.Duration = defaultReadTimeout
	}
	if cfg.Server.WriteTimeout.Duration == 0 {
		cfg.Server.WriteTimeout.Duration = defaultWriteTimeout
	}
	if cfg.Server.IdleTimeout.Duration == 0 {
		cfg.Server.IdleTimeout.Duration = defaultIdleTimeout
	}
	if cfg.Server.MaxUpload == 0 {
		cfg.Server.MaxUpload = ByteSize(defaultMaxUpload)
	}
	if cfg.Server.LogLevel == "" {
		cfg.Server.LogLevel = defaultLogLevel
	}
	if cfg.LLM.Provider == "" {
		cfg.LLM.Provider = defaultLLMProvider
	}
	if cfg.LLM.AIProxy.Temperature == 0 {
		cfg.LLM.AIProxy.Temperature = defaultTemperature
	}
	if cfg.LLM.AIProxy.MaxTokens == 0 {
		cfg.LLM.AIProxy.MaxTokens = defaultMaxTokens
	}
	if cfg.Calendar.Provider == "" {
		cfg.Calendar.Provider = defaultCalProvider
	}
	if cfg.Calendar.Google.CalendarID == "" {
		cfg.Calendar.Google.CalendarID = defaultCalendarID
	}
	if cfg.Calendar.Google.TimeZone == "" {
		cfg.Calendar.Google.TimeZone = defaultTimeZone
	}
}

func validate(cfg *Config) error {
	switch cfg.LLM.Provider {
	case "mock", "aiproxy":
	default:
		return fmt.Errorf("unsupported llm.provider %q (must be \"mock\" or \"aiproxy\")", cfg.LLM.Provider)
	}

	if cfg.LLM.Provider == "aiproxy" {
		if cfg.LLM.AIProxy.BaseURL == "" {
			return fmt.Errorf("llm.aiproxy.baseUrl is required when provider is \"aiproxy\"")
		}
	}

	switch cfg.Calendar.Provider {
	case "google", "mock":
	default:
		return fmt.Errorf("unsupported calendar.provider %q (must be \"google\" or \"mock\")", cfg.Calendar.Provider)
	}

	if cfg.Calendar.Provider == "google" {
		if cfg.Calendar.Google.CredentialsFile == "" {
			return fmt.Errorf("calendar.google.credentialsFile is required")
		}
	}

	return nil
}

package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type CalendarProvider string
type LLMProvider string
type StorageProvider string
type LogLevel string

const (
	CalendarProviderGoogle CalendarProvider = "google"
	CalendarProviderWebcal CalendarProvider = "webcal"
	CalendarProviderMock   CalendarProvider = "mock"
	CalendarProviderSMTP   CalendarProvider = "smtp"

	LLMProviderMock    LLMProvider = "mock"
	LLMProviderAIProxy LLMProvider = "aiproxy"

	StorageProviderS3   StorageProvider = "s3"
	StorageProviderMock StorageProvider = "mock"

	LogLevelDebug   LogLevel = "debug"
	LogLevelInfo    LogLevel = "info"
	LogLevelWarn    LogLevel = "warn"
	LogLevelWarning LogLevel = "warning"
	LogLevelError   LogLevel = "error"
)

type SMTPAuthMethod string

const (
	SMTPAuthNone  SMTPAuthMethod = "none"
	SMTPAuthPlain SMTPAuthMethod = "plain"
	SMTPAuthLogin SMTPAuthMethod = "login"
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
	LogLevel     LogLevel `yaml:"logLevel"`
}

type LLMConfig struct {
	Provider LLMProvider   `yaml:"provider"`
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
	Provider CalendarProvider     `yaml:"provider"`
	Google   GoogleCalendarConfig `yaml:"google"`
	Webcal   WebcalConfig         `yaml:"webcal"`
	SMTP     SMTPConfig           `yaml:"smtp"`
}

type WebcalConfig struct {
	EventTTL Duration      `yaml:"eventTtl"`
	Storage  StorageConfig `yaml:"storage"`
}

type StorageConfig struct {
	Provider StorageProvider `yaml:"provider"`
	S3       S3Config        `yaml:"s3"`
}

type S3Config struct {
	Bucket          string `yaml:"bucket"`
	Key             string `yaml:"key"`
	Region          string `yaml:"region"`
	CredentialsFile string `yaml:"credentialsFile"`
	Endpoint        string `yaml:"endpoint"`
	PublicURL       string `yaml:"publicUrl"`
}

type GoogleCalendarConfig struct {
	CredentialsFile string `yaml:"credentialsFile"`
	CalendarID      string `yaml:"calendarId"`
	TimeZone        string `yaml:"timeZone"`
}

type SMTPConfig struct {
	Host            string         `yaml:"host"`
	Port            int            `yaml:"port"`
	AuthMethod      SMTPAuthMethod `yaml:"authMethod"`
	CredentialsFile string         `yaml:"credentialsFile"` // required when authMethod != "none"
	From            string         `yaml:"from"`            // sender display name or email
	To              string         `yaml:"to"`              // recipient email address
	TLS             bool           `yaml:"tls"`             // use TLS (port 465); false = STARTTLS or plain
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
	defaultAddress         = ":8080"
	defaultReadTimeout     = 15 * time.Second
	defaultWriteTimeout    = 2 * time.Minute
	defaultIdleTimeout     = 60 * time.Second
	defaultMaxUpload       = 10 * 1024 * 1024 // 10 MiB
	defaultLLMProvider     = LLMProviderMock
	defaultCalProvider     = CalendarProviderGoogle
	defaultCalendarID      = "primary"
	defaultTimeZone        = "UTC"
	defaultLogLevel        = LogLevelInfo
	defaultTemperature     = 0.2
	defaultMaxTokens       = 4096
	defaultEventTTL        = 720 * time.Hour // 30 days
	defaultS3Key           = "calendar.ics"
	defaultStorageProvider = StorageProviderS3
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
	if cfg.Calendar.Webcal.EventTTL.Duration == 0 {
		cfg.Calendar.Webcal.EventTTL.Duration = defaultEventTTL
	}
	if cfg.Calendar.Webcal.Storage.Provider == "" {
		cfg.Calendar.Webcal.Storage.Provider = defaultStorageProvider
	}
	if cfg.Calendar.Webcal.Storage.S3.Key == "" {
		cfg.Calendar.Webcal.Storage.S3.Key = defaultS3Key
	}
	if cfg.Calendar.SMTP.Port == 0 {
		cfg.Calendar.SMTP.Port = 587
	}
	if cfg.Calendar.SMTP.AuthMethod == "" {
		cfg.Calendar.SMTP.AuthMethod = SMTPAuthPlain
	}
}

func validate(cfg *Config) error {
	if err := validateServer(&cfg.Server); err != nil {
		return err
	}
	if err := validateLLM(&cfg.LLM); err != nil {
		return err
	}
	return validateCalendar(&cfg.Calendar)
}

func validateServer(cfg *ServerConfig) error {
	switch cfg.LogLevel {
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelWarning, LogLevelError:
	default:
		return fmt.Errorf("unsupported server.logLevel %q (must be one of %q, %q, %q, %q, %q)",
			cfg.LogLevel, LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelWarning, LogLevelError)
	}
	return nil
}

func validateLLM(cfg *LLMConfig) error {
	switch cfg.Provider {
	case LLMProviderMock, LLMProviderAIProxy:
	default:
		return fmt.Errorf("unsupported llm.provider %q (must be %q or %q)", cfg.Provider, LLMProviderMock, LLMProviderAIProxy)
	}

	if cfg.Provider == LLMProviderAIProxy {
		if cfg.AIProxy.BaseURL == "" {
			return fmt.Errorf("llm.aiproxy.baseUrl is required when provider is %q", LLMProviderAIProxy)
		}
		if cfg.AIProxy.APIKey == "" {
			return fmt.Errorf("llm.aiproxy.apiKey is required when provider is %q", LLMProviderAIProxy)
		}
	}

	return nil
}

func validateCalendar(cfg *CalendarConfig) error {
	switch cfg.Provider {
	case CalendarProviderGoogle, CalendarProviderMock:
	case CalendarProviderWebcal:
		if err := validateWebcalStorage(&cfg.Webcal.Storage); err != nil {
			return err
		}
	case CalendarProviderSMTP:
		return validateSMTP(&cfg.SMTP)
	default:
		return fmt.Errorf("unsupported calendar.provider %q (must be %q, %q, %q, or %q)", cfg.Provider, CalendarProviderGoogle, CalendarProviderWebcal, CalendarProviderMock, CalendarProviderSMTP)
	}

	if cfg.Provider == CalendarProviderGoogle {
		if cfg.Google.CredentialsFile == "" {
			return fmt.Errorf("calendar.google.credentialsFile is required")
		}
	}

	return nil
}

func validateWebcalStorage(cfg *StorageConfig) error {
	switch cfg.Provider {
	case StorageProviderS3:
		s3 := cfg.S3
		if s3.Bucket == "" {
			return fmt.Errorf("calendar.webcal.storage.s3.bucket is required")
		}
		if s3.Region == "" {
			return fmt.Errorf("calendar.webcal.storage.s3.region is required")
		}
		if s3.CredentialsFile == "" {
			return fmt.Errorf("calendar.webcal.storage.s3.credentialsFile is required")
		}
	case StorageProviderMock:
	default:
		return fmt.Errorf("unsupported calendar.webcal.storage.provider %q (must be %q or %q)", cfg.Provider, StorageProviderS3, StorageProviderMock)
	}
	return nil
}

func validateSMTP(cfg *SMTPConfig) error {
	switch cfg.AuthMethod {
	case SMTPAuthNone, SMTPAuthPlain, SMTPAuthLogin:
	default:
		return fmt.Errorf("unsupported calendar.smtp.authMethod %q (must be one of %q, %q, %q)",
			cfg.AuthMethod, SMTPAuthNone, SMTPAuthPlain, SMTPAuthLogin)
	}
	if cfg.Host == "" {
		return fmt.Errorf("calendar.smtp.host is required")
	}
	if cfg.To == "" {
		return fmt.Errorf("calendar.smtp.to is required")
	}
	if cfg.From == "" {
		return fmt.Errorf("calendar.smtp.from is required")
	}
	if cfg.AuthMethod != SMTPAuthNone && cfg.CredentialsFile == "" {
		return fmt.Errorf("calendar.smtp.credentialsFile is required when authMethod is %q", cfg.AuthMethod)
	}
	return nil
}

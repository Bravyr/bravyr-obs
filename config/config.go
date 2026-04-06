// Package config provides environment-based configuration for the
// bravyr-obs observability stack.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Config holds all settings needed to initialize the observability stack.
type Config struct {
	ServiceName   string  `env:"OBS_SERVICE_NAME,required"`
	Environment   string  `env:"OBS_ENVIRONMENT"    envDefault:"development"`
	LogLevel      string  `env:"OBS_LOG_LEVEL"      envDefault:"info"`
	SeqURL        string  `env:"OBS_SEQ_URL"`
	SeqAPIKey     string  `env:"OBS_SEQ_API_KEY"`
	OTLPEndpoint  string  `env:"OBS_OTLP_ENDPOINT"`
	SampleRate    float64 `env:"OBS_SAMPLE_RATE"    envDefault:"1.0"`
	DevMode       bool    `env:"OBS_DEV_MODE"       envDefault:"false"`
	MetricsPrefix string  `env:"OBS_METRICS_PREFIX" envDefault:""`
}

// Validate checks that all required configuration fields are set and
// returns an error describing every violation found.
func (c Config) Validate() error {
	var errs []error

	if c.ServiceName == "" {
		errs = append(errs, errors.New("ServiceName is required"))
	}

	validLevels := map[string]bool{
		"debug": true, "info": true,
		"warn": true, "error": true, "fatal": true,
	}
	if !validLevels[strings.ToLower(c.LogLevel)] {
		errs = append(errs, fmt.Errorf("LogLevel %q is not valid", c.LogLevel))
	}

	if c.SeqURL != "" && !c.DevMode && !strings.HasPrefix(c.SeqURL, "https://") {
		errs = append(errs, fmt.Errorf("SeqURL must use https:// in non-dev mode, got %q", c.SeqURL))
	}

	if c.DevMode && strings.EqualFold(c.Environment, "production") {
		errs = append(errs, errors.New("DevMode must not be enabled in production environment"))
	}

	if c.SampleRate < 0 || c.SampleRate > 1 {
		errs = append(errs, fmt.Errorf("SampleRate must be between 0.0 and 1.0, got %f", c.SampleRate))
	}

	return errors.Join(errs...)
}

// String returns a human-readable representation of the configuration
// with sensitive fields redacted.
func (c Config) String() string {
	apiKey := ""
	if c.SeqAPIKey != "" {
		apiKey = "***"
	}

	return fmt.Sprintf(
		"Config{ServiceName:%q Environment:%q LogLevel:%q SeqURL:%q SeqAPIKey:%q OTLPEndpoint:%q SampleRate:%g DevMode:%t MetricsPrefix:%q}",
		c.ServiceName, c.Environment, c.LogLevel, c.SeqURL, apiKey, c.OTLPEndpoint, c.SampleRate, c.DevMode, c.MetricsPrefix,
	)
}

// MarshalJSON implements json.Marshaler with sensitive field redaction.
func (c Config) MarshalJSON() ([]byte, error) {
	apiKey := ""
	if c.SeqAPIKey != "" {
		apiKey = "***"
	}

	return json.Marshal(struct {
		ServiceName   string  `json:"service_name"`
		Environment   string  `json:"environment"`
		LogLevel      string  `json:"log_level"`
		SeqURL        string  `json:"seq_url"`
		SeqAPIKey     string  `json:"seq_api_key"`
		OTLPEndpoint  string  `json:"otlp_endpoint"`
		SampleRate    float64 `json:"sample_rate"`
		DevMode       bool    `json:"dev_mode"`
		MetricsPrefix string  `json:"metrics_prefix"`
	}{
		ServiceName:   c.ServiceName,
		Environment:   c.Environment,
		LogLevel:      c.LogLevel,
		SeqURL:        c.SeqURL,
		SeqAPIKey:     apiKey,
		OTLPEndpoint:  c.OTLPEndpoint,
		SampleRate:    c.SampleRate,
		DevMode:       c.DevMode,
		MetricsPrefix: c.MetricsPrefix,
	})
}

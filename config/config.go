// Package config provides environment-based configuration for the
// bravyr-obs observability stack.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
)

// Config holds all settings needed to initialize the observability stack.
type Config struct {
	ServiceName   string  `env:"OBS_SERVICE_NAME,required"`
	Environment   string  `env:"OBS_ENVIRONMENT"    envDefault:"development"`
	LogLevel      string  `env:"OBS_LOG_LEVEL"      envDefault:"info"`
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

	if c.DevMode && strings.EqualFold(c.Environment, "production") {
		errs = append(errs, errors.New("DevMode must not be enabled in production environment"))
	}

	if c.SampleRate < 0 || c.SampleRate > 1 {
		errs = append(errs, fmt.Errorf("SampleRate must be between 0.0 and 1.0, got %f", c.SampleRate))
	}

	if c.OTLPEndpoint != "" && !c.DevMode {
		if isDangerousHost(extractHost(c.OTLPEndpoint)) {
			errs = append(errs, fmt.Errorf("OTLPEndpoint %q targets a loopback or link-local address; set DevMode=true for local collectors", c.OTLPEndpoint))
		}
	}

	return errors.Join(errs...)
}

// String returns a human-readable representation of the configuration.
func (c Config) String() string {
	return fmt.Sprintf(
		"Config{ServiceName:%q Environment:%q LogLevel:%q OTLPEndpoint:%q SampleRate:%g DevMode:%t MetricsPrefix:%q}",
		c.ServiceName, c.Environment, c.LogLevel, c.OTLPEndpoint, c.SampleRate, c.DevMode, c.MetricsPrefix,
	)
}

// extractHost extracts the host portion from a host:port endpoint string.
func extractHost(endpoint string) string {
	// Strip optional scheme (e.g., "grpc://host:port" → "host:port").
	if _, after, found := strings.Cut(endpoint, "://"); found {
		endpoint = after
	}
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return endpoint
	}
	return host
}

// isDangerousHost returns true for loopback and link-local IP addresses
// which are SSRF targets (e.g., cloud metadata at 169.254.169.254).
// RFC-1918 private IPs (10.x, 172.16-31.x, 192.168.x) are allowed
// because they are legitimate in Docker/VM deployments.
func isDangerousHost(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		// Hostname, not IP literal — allow. A hostname that resolves to
		// loopback at runtime (e.g., "localhost") bypasses this check.
		// For additional protection, restrict egress at the network level.
		return false
	}
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}

// MarshalJSON implements json.Marshaler.
func (c Config) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ServiceName   string  `json:"service_name"`
		Environment   string  `json:"environment"`
		LogLevel      string  `json:"log_level"`
		OTLPEndpoint  string  `json:"otlp_endpoint"`
		SampleRate    float64 `json:"sample_rate"`
		DevMode       bool    `json:"dev_mode"`
		MetricsPrefix string  `json:"metrics_prefix"`
	}{
		ServiceName:   c.ServiceName,
		Environment:   c.Environment,
		LogLevel:      c.LogLevel,
		OTLPEndpoint:  c.OTLPEndpoint,
		SampleRate:    c.SampleRate,
		DevMode:       c.DevMode,
		MetricsPrefix: c.MetricsPrefix,
	})
}

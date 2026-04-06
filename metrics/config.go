package metrics

import (
	"fmt"
	"regexp"
)

var validPrometheusName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// Config holds settings for the Prometheus metrics registry.
type Config struct {
	// Prefix is prepended to all metric names with an underscore separator.
	// For example, prefix "myapp" produces "myapp_http_request_duration_seconds".
	// An empty prefix means no prefix is added. Must be a valid Prometheus
	// metric name component if non-empty.
	Prefix string
}

// Validate returns an error if the Config is not usable by Init.
func (c Config) Validate() error {
	if c.Prefix != "" && !validPrometheusName.MatchString(c.Prefix) {
		return fmt.Errorf("metrics: prefix %q is not a valid Prometheus name (must match [a-zA-Z_][a-zA-Z0-9_]*)", c.Prefix)
	}
	return nil
}

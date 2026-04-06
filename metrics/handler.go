package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Handler returns an http.Handler that serves the Prometheus text exposition
// format on the registry's custom (non-global) collector set.
//
// The /metrics endpoint should be protected by authentication middleware or
// served on an internal-only port, as it exposes route patterns, request
// rates, and custom business metrics that reveal API topology.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.reg, promhttp.HandlerOpts{})
}

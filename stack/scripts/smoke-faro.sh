#!/usr/bin/env bash
# Smoke test for the Grafana Faro ingest pipeline.
#
# Prerequisites: the full stack must be running (docker compose up from
# stack/). The test posts a synthetic Faro payload to the local Alloy
# receiver (bypassing Traefik) and asserts the resulting span surfaces
# in Tempo and the log entry surfaces in Loki.
#
# Usage: stack/scripts/smoke-faro.sh

set -euo pipefail

ALLOY_URL="${ALLOY_URL:-http://127.0.0.1:12347/collect}"
TEMPO_URL="${TEMPO_URL:-http://127.0.0.1:3200}"
LOKI_URL="${LOKI_URL:-http://127.0.0.1:3100}"
WAIT_SECONDS="${WAIT_SECONDS:-5}"

trace_id="$(openssl rand -hex 16)"
span_id="$(openssl rand -hex 8)"
now_ns="$(date +%s)000000000"
end_ns="$(( now_ns + 1000000000 ))"

payload=$(cat <<EOF
{
  "meta": {
    "app":     {"name":"socialup-web","version":"0.0.0-smoke","environment":"development"},
    "browser": {"name":"Chrome","os":"macOS","mobile":false,"userAgent":"smoke"},
    "session": {"id":"smoke-session"},
    "user":    {"id":"smoke-user"},
    "view":    {"name":"/smoke"},
    "trace":   {"trace_id":"${trace_id}","span_id":"${span_id}"}
  },
  "logs":  [{"timestamp":"$(date -u +%FT%TZ)","level":"info","message":"smoke-test","kind":"log"}],
  "traces": {
    "resourceSpans": [{
      "resource": {"attributes":[{"key":"service.name","value":{"stringValue":"socialup-web"}}]},
      "scopeSpans": [{"spans":[{
        "traceId":"${trace_id}","spanId":"${span_id}",
        "name":"smoke-test",
        "startTimeUnixNano":"${now_ns}","endTimeUnixNano":"${end_ns}",
        "kind":1,"status":{}
      }]}]
    }]
  }
}
EOF
)

echo "POST ${ALLOY_URL} (trace_id=${trace_id})"
status=$(curl -s -o /tmp/faro-smoke.resp -w "%{http_code}" \
  -X POST "${ALLOY_URL}" \
  -H "Content-Type: application/json" \
  -H "Origin: http://localhost:5173" \
  --data "${payload}")
if [[ "${status}" != "200" && "${status}" != "204" ]]; then
  echo "  FAIL — expected 200/204, got ${status}"
  cat /tmp/faro-smoke.resp
  exit 1
fi
echo "  OK (${status})"

echo "Waiting ${WAIT_SECONDS}s for ingestion..."
sleep "${WAIT_SECONDS}"

echo "GET ${TEMPO_URL}/api/traces/${trace_id}"
if ! curl -sf "${TEMPO_URL}/api/traces/${trace_id}" | grep -q "${span_id}"; then
  echo "  FAIL — trace not found in Tempo"
  exit 1
fi
echo "  OK"

echo "GET ${LOKI_URL}/loki/api/v1/query_range {source=faro}"
start_ns=$(( now_ns - 60000000000 ))
end_query_ns=$(( now_ns + 60000000000 ))
response=$(curl -sfG "${LOKI_URL}/loki/api/v1/query_range" \
  --data-urlencode 'query={source="faro"}' \
  --data-urlencode "start=${start_ns}" \
  --data-urlencode "end=${end_query_ns}")
count=$(printf '%s' "${response}" | grep -c "smoke-test" || true)
if [[ "${count}" == "0" ]]; then
  echo "  FAIL — 'smoke-test' not found in Loki within window"
  exit 1
fi
echo "  OK (${count} match)"

echo
echo "All checks passed."

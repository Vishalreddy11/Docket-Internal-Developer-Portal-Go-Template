#!/bin/sh
# Scan every image in the docket stack (HIGH + CRITICAL only) and write
# per-image HTML + JSON reports plus an index.html to /reports.
# Runs inside the aquasec/trivy container; expects the docker socket mounted.

set -u

REPORTS=/reports
mkdir -p "$REPORTS"

# Images the stack pulls, plus the locally built docket:local (read from the
# host docker daemon via the socket mount).
IMAGES="
docket:local
cgr.dev/chainguard/postgres:latest
nats:2.14-alpine
cgr.dev/chainguard/valkey:latest
chrislusf/seaweedfs:latest
jaegertracing/jaeger:latest
"

TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)

{
  echo '<!doctype html><html><head><meta charset="utf-8">'
  echo '<title>Docket vulnerability reports</title>'
  echo '<style>'
  echo 'body{font-family:-apple-system,system-ui,sans-serif;max-width:900px;margin:40px auto;padding:0 20px;color:#222}'
  echo 'h1{border-bottom:2px solid #333;padding-bottom:8px}'
  echo 'table{border-collapse:collapse;width:100%;margin-top:20px}'
  echo 'th,td{padding:10px 12px;text-align:left;border-bottom:1px solid #e0e0e0}'
  echo 'th{background:#f7f7f7;font-size:.85em;text-transform:uppercase;letter-spacing:.5px}'
  echo 'a{color:#0366d6;text-decoration:none}a:hover{text-decoration:underline}'
  echo 'code{background:#f4f4f4;padding:2px 6px;border-radius:3px;font-size:.9em}'
  echo '.meta{color:#666;font-size:.9em}'
  echo '</style></head><body>'
  echo '<h1>Docket container vulnerability reports</h1>'
  echo "<p class=\"meta\">Scanner: Trivy &middot; severity: HIGH + CRITICAL &middot; generated: ${TIMESTAMP}</p>"
  echo '<p>Re-scan with: <code>docker compose up trivy</code></p>'
  echo '<table><tr><th>Image</th><th>Report</th><th>Raw JSON</th></tr>'
} > "$REPORTS/index.html"

for img in $IMAGES; do
  [ -z "$img" ] && continue
  safe=$(echo "$img" | tr '/:' '__')
  echo "==> Scanning $img"

  trivy image \
    --severity HIGH,CRITICAL \
    --ignorefile /.trivyignore \
    --format template \
    --template "@/contrib/html.tpl" \
    --output "$REPORTS/${safe}.html" \
    "$img" || echo "!! HTML scan failed for $img"

  trivy image \
    --severity HIGH,CRITICAL \
    --ignorefile /.trivyignore \
    --format json \
    --output "$REPORTS/${safe}.json" \
    "$img" || echo "!! JSON scan failed for $img"

  echo "<tr><td><code>${img}</code></td><td><a href=\"${safe}.html\">HTML</a></td><td><a href=\"${safe}.json\">JSON</a></td></tr>" >> "$REPORTS/index.html"
done

echo '</table></body></html>' >> "$REPORTS/index.html"
echo "Done. Reports in /reports. Open http://localhost:8090"

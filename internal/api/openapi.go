package api

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.json
var openAPISpec []byte

const swaggerHTML = `<!doctype html>
<html>
<head>
  <title>Docket API — Swagger UI</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
  <style>
    body { margin: 0; }
    .topbar { display: none; }
    .pre-auth-banner {
      background: #1f6feb; color: white;
      padding: 10px 16px; font-family: -apple-system, BlinkMacSystemFont, sans-serif;
      font-size: 14px; line-height: 1.5;
    }
    .pre-auth-banner code {
      background: rgba(255,255,255,0.18); padding: 2px 6px; border-radius: 3px;
    }
  </style>
</head>
<body>
  <div class="pre-auth-banner">
    This page is pre-filled with the API key <code>dev-secret-change-me</code>. Every <em>Try it out</em> call sends that value in an <code>X-API-Key</code> header automatically. To require your own key, set the <code>DOCKET_API_KEY</code> env var when starting the app — after that, every write request must send a matching header, or the API rejects it with <code>401</code>. If the env var is empty, the check is skipped and anyone can write.
  </div>
  <div id="ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.ui = SwaggerUIBundle({
      url: "/openapi.json",
      dom_id: "#ui",
      deepLinking: true,
      docExpansion: "list",
      defaultModelsExpandDepth: -1,
      tryItOutEnabled: true,
      onComplete: function() {
        window.ui.preauthorizeApiKey("apiKey", "dev-secret-change-me");
      }
    });
  </script>
</body>
</html>`

func (h *handlers) OpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(openAPISpec)
}

func (h *handlers) SwaggerUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerHTML))
}

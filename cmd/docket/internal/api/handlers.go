package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/example/docket/internal/app"
	"github.com/example/docket/internal/events"
	"github.com/example/docket/internal/logging"
	"github.com/example/docket/internal/metadata"
	"github.com/example/docket/internal/metrics"
	"github.com/example/docket/internal/records"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type handlers struct {
	app *app.App
}

func newHandlers(a *app.App) *handlers { return &handlers{app: a} }

// ---------- Health ----------

func (h *handlers) Health(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"service":  "docket",
		"status":   "ok",
		"backends": h.app.Health(),
		"time":     time.Now().UTC(),
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---------- Upload ----------

func (h *handlers) UploadFile(w http.ResponseWriter, r *http.Request) {
	log := logging.FromContext(r.Context(), h.app.Log)

	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, "invalid multipart form: "+err.Error(), http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing 'file' form field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	owner := r.FormValue("owner")
	if owner == "" {
		owner = "anonymous"
	}
	desc := r.FormValue("description")
	tags := splitTags(r.FormValue("tags"))

	id := uuid.NewString()
	ct := header.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}

	if err := h.app.Storage.Put(r.Context(), id, file, header.Size, ct); err != nil {
		log.Error("storage put failed", "err", err)
		http.Error(w, "storage failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	metrics.UploadsTotal.WithLabelValues(h.app.Storage.Mode()).Inc()

	meta := metadata.Meta{
		ID:          id,
		FileName:    header.Filename,
		ContentType: ct,
		Size:        header.Size,
		Owner:       owner,
		Description: desc,
		Tags:        tags,
		Extra:       map[string]string{},
		UploadedAt:  time.Now().UTC(),
	}
	if err := h.app.Metadata.Insert(r.Context(), meta); err != nil {
		log.Error("metadata insert failed", "err", err)
	}

	rec := records.Record{
		ID:        uuid.NewString(),
		FileID:    id,
		Owner:     owner,
		FileName:  header.Filename,
		Size:      header.Size,
		Action:    "upload",
		CreatedAt: time.Now().UTC(),
	}
	if err := h.app.Records.Insert(r.Context(), rec); err != nil {
		log.Error("record insert failed", "err", err)
	}

	ev := events.Event{
		Type:      "file.uploaded",
		FileID:    id,
		Owner:     owner,
		Timestamp: time.Now().UTC(),
		Payload: map[string]any{
			"file_name":    header.Filename,
			"content_type": ct,
			"size":         header.Size,
		},
	}
	if err := h.app.Events.Publish(r.Context(), ev); err != nil {
		log.Warn("event publish failed", "err", err)
	} else {
		metrics.EventsPublished.WithLabelValues(h.app.Cfg.NATS.SubjectPrefix).Inc()
	}

	writeJSON(w, http.StatusCreated, meta)
}

// ---------- Get / List ----------

func (h *handlers) GetFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	meta, err := h.app.Metadata.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	views, _ := h.app.Cache.IncrView(r.Context(), id)
	metrics.CacheOps.WithLabelValues("incr_view", "ok").Inc()

	writeJSON(w, http.StatusOK, map[string]any{
		"meta":  meta,
		"views": views,
	})
}

func (h *handlers) ListFiles(w http.ResponseWriter, r *http.Request) {
	limit := atoiDefault(r.URL.Query().Get("limit"), 20)
	offset := atoiDefault(r.URL.Query().Get("offset"), 0)
	list, err := h.app.Metadata.List(r.Context(), limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  list,
		"limit":  limit,
		"offset": offset,
		"count":  len(list),
	})
}

func (h *handlers) DownloadFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	body, size, ct, err := h.app.Storage.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer body.Close()

	// Best-effort: look up the original filename from Mongo so the browser
	// saves the file under its real name. Falls back to the UUID.
	filename := id
	if meta, err := h.app.Metadata.Get(r.Context(), id); err == nil && meta.FileName != "" {
		filename = meta.FileName
	}

	disposition := "attachment"
	if r.URL.Query().Get("inline") == "true" {
		disposition = "inline"
	}

	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`%s; filename="%s"`, disposition, sanitizeFilename(filename)))

	if _, err := io.Copy(w, body); err != nil {
		logging.FromContext(r.Context(), h.app.Log).Warn("download write failed", "err", err)
	}
}

func sanitizeFilename(s string) string {
	// Strip characters that would break the Content-Disposition header.
	return strings.Map(func(r rune) rune {
		switch r {
		case '"', '\\', '\r', '\n':
			return -1
		}
		return r
	}, s)
}

func (h *handlers) AuditTrail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rows, err := h.app.Records.ListByFile(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"file_id": id, "records": rows})
}

// ---------- Delete ----------

func (h *handlers) DeleteFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	log := logging.FromContext(r.Context(), h.app.Log)

	if err := h.app.Storage.Delete(r.Context(), id); err != nil {
		log.Warn("storage delete failed", "err", err)
	}
	if err := h.app.Metadata.Delete(r.Context(), id); err != nil {
		log.Warn("metadata delete failed", "err", err)
	}
	_ = h.app.Records.Insert(r.Context(), records.Record{
		ID: uuid.NewString(), FileID: id, Owner: "system", Action: "delete", CreatedAt: time.Now().UTC(),
	})
	_ = h.app.Events.Publish(r.Context(), events.Event{
		Type: "file.deleted", FileID: id, Timestamp: time.Now().UTC(),
	})

	w.WriteHeader(http.StatusNoContent)
}

// ---------- Seed ----------

func (h *handlers) Seed(w http.ResponseWriter, r *http.Request) {
	n := atoiDefault(r.URL.Query().Get("n"), 20)
	if n > 1000 {
		n = 1000
	}
	created := make([]string, 0, n)
	for i := 0; i < n; i++ {
		id := uuid.NewString()
		body := fmt.Sprintf("seed-file-%d", i)
		_ = h.app.Storage.Put(r.Context(), id, strings.NewReader(body), int64(len(body)), "text/plain")
		_ = h.app.Metadata.Insert(r.Context(), metadata.Meta{
			ID:          id,
			FileName:    fmt.Sprintf("seed-%d.txt", i),
			ContentType: "text/plain",
			Size:        int64(len(body)),
			Owner:       "seed",
			Description: "seeded demo file",
			Tags:        []string{"seed", "demo"},
			Extra:       map[string]string{},
			UploadedAt:  time.Now().UTC(),
		})
		_ = h.app.Records.Insert(r.Context(), records.Record{
			ID: uuid.NewString(), FileID: id, Owner: "seed", FileName: fmt.Sprintf("seed-%d.txt", i),
			Size: int64(len(body)), Action: "upload", CreatedAt: time.Now().UTC(),
		})
		created = append(created, id)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"created": len(created), "ids": created})
}

// ---------- Helpers ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func splitTags(v string) []string {
	if v == "" {
		return []string{}
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Compile-time guard that we use context for request lifetimes.
var _ = context.Background

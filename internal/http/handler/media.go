package handler

import (
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"path"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/khansbikezone/bikezone-api/internal/http/dto"
	"github.com/khansbikezone/bikezone-api/internal/http/problem"
	"github.com/khansbikezone/bikezone-api/internal/service"
)

// Upload limits. The server's default read timeout (30s) is too short for a
// large photo from a phone on a slow connection, so upload handlers extend
// their own read deadline.
const (
	uploadReadTimeout = 5 * time.Minute
	multipartOverhead = 1 << 20 // headers and boundaries around the file part
)

// Media serves image uploads (admin) and rendition files (public).
type Media struct {
	base
	svc       *service.Media
	urls      dto.URLFunc
	maxUpload int64
}

// NewMedia builds the media handlers.
func NewMedia(log *slog.Logger, svc *service.Media, urls dto.URLFunc, maxUpload int64) *Media {
	return &Media{base: base{log: log}, svc: svc, urls: urls, maxUpload: maxUpload}
}

// filePart returns the multipart part named "file", streaming: the upload is
// never buffered whole in memory. It answers the request itself and returns
// false when there is no such part.
func (b base) filePart(w http.ResponseWriter, r *http.Request, maxBytes int64) (*multipart.Part, bool) {
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(uploadReadTimeout))
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+multipartOverhead)
	mr, err := r.MultipartReader()
	if err != nil {
		problem.Write(w, problem.New(http.StatusBadRequest, "Send the file as multipart/form-data in a part named \"file\"."))
		return nil, false
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			problem.Write(w, problem.New(http.StatusBadRequest, "The request has no part named \"file\"."))
			return nil, false
		}
		if err != nil {
			b.fail(w, r, err)
			return nil, false
		}
		if part.FormName() == "file" {
			return part, true
		}
		_ = part.Close()
	}
}

// Upload answers POST /admin/media. A new upload returns 201 with status
// "pending"; poll GET /admin/media/{id} until it is "ready". Uploading bytes
// that already exist returns the existing asset with 200.
func (h *Media) Upload(w http.ResponseWriter, r *http.Request) {
	part, ok := h.filePart(w, r, h.maxUpload)
	if !ok {
		return
	}
	defer func() { _ = part.Close() }()
	asset, created, err := h.svc.Upload(r.Context(), part.FileName(), part)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, dto.NewAsset(asset, h.urls))
}

// Asset answers GET /admin/media/{id}.
func (h *Media) Asset(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id")
	if !ok {
		return
	}
	a, err := h.svc.Asset(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.NewAsset(a, h.urls))
}

var renditionTypes = map[string]string{".webp": "image/webp", ".jpeg": "image/jpeg"}

// Serve answers GET /media/* with a rendition file. Rendition keys never
// change content, so they are cacheable forever (by browsers and Cloudflare).
// http.ServeContent handles HEAD and range requests.
func (h *Media) Serve(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "*")
	f, err := h.svc.OpenPublic(r.Context(), key)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	defer func() { _ = f.Close() }()
	if ct, ok := renditionTypes[path.Ext(key)]; ok {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, "", time.Time{}, f)
}

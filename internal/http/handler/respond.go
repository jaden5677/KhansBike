package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/http/middleware"
	"github.com/khansbikezone/bikezone-api/internal/http/problem"
)

// base is embedded by every handler group: the shared ways of answering.
type base struct {
	log *slog.Logger
}

// fail answers with the problem for err. Server-side failures are logged
// with the request id; the client only ever sees a generic message for them.
func (b base) fail(w http.ResponseWriter, r *http.Request, err error) {
	// The client went away (closed the tab, or the web app cancelled a
	// request it no longer needs). Nobody is waiting for the answer and the
	// server did nothing wrong, so record it quietly with 499, the usual
	// "client closed request" status, instead of logging a 500.
	if r.Context().Err() != nil {
		b.log.DebugContext(r.Context(), "request cancelled by client",
			"request_id", middleware.RequestIDFromContext(r.Context()),
			"method", r.Method, "path", r.URL.Path, "error", err)
		w.WriteHeader(problem.StatusClientClosedRequest)
		return
	}
	p := problem.FromError(err)
	if p.Status >= http.StatusInternalServerError {
		b.log.ErrorContext(r.Context(), "request failed",
			"request_id", middleware.RequestIDFromContext(r.Context()),
			"method", r.Method, "path", r.URL.Path, "error", err)
	}
	problem.Write(w, p)
}

// writeJSON sends body as JSON with the given status.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// maxJSONBody bounds every JSON request body; the largest legitimate one, a
// product document with many variants, is far smaller.
const maxJSONBody = 1 << 20

// decode reads a JSON request body into dst, answering 400 or 413 itself and
// returning false when the body is unusable. Unknown fields are rejected so
// a typo in a field name fails loudly instead of being silently ignored.
func (b base) decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody))
	dec.DisallowUnknownFields()
	err := dec.Decode(dst)
	if err == nil && dec.More() {
		err = errors.New("unexpected data after the JSON object")
	}
	if err == nil {
		return true
	}
	var (
		tooLarge  *http.MaxBytesError
		syntax    *json.SyntaxError
		typeError *json.UnmarshalTypeError
		detail    string
	)
	switch {
	case errors.As(err, &tooLarge):
		problem.Write(w, problem.New(http.StatusRequestEntityTooLarge, "The request body is too large."))
		return false
	case errors.Is(err, io.EOF):
		detail = "A JSON request body is required."
	case errors.As(err, &syntax):
		detail = fmt.Sprintf("The request body is not valid JSON (at byte %d).", syntax.Offset)
	case errors.As(err, &typeError):
		detail = fmt.Sprintf("Field %q must be a %s.", typeError.Field, typeError.Type)
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		detail = "Unknown field " + strings.TrimPrefix(err.Error(), "json: unknown field ") + "."
	default:
		detail = "The request body could not be read: " + err.Error()
	}
	problem.Write(w, problem.New(http.StatusBadRequest, detail))
	return false
}

// idParam reads a UUID path parameter. A malformed id cannot name anything,
// so it is answered as not found.
func (b base) idParam(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		problem.Write(w, problem.New(http.StatusNotFound, "No resource has this id."))
		return uuid.Nil, false
	}
	return id, true
}

// queryInt reads an optional integer query parameter (0 when absent).
func queryInt(v url.Values, name string) (int, error) {
	s := v.Get(name)
	if s == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, domain.Invalid(name, "must be an integer")
	}
	return n, nil
}

// productQuery reads the listing query string: category, brand, q, sort,
// cursor, limit, status (admin only) and attribute filters:
//
//	attr.<key>=v1,v2        option values (any of), a boolean, or an exact number
//	attr.<key>_min=1.9      numeric lower bound
//	attr.<key>_max=2.2      numeric upper bound
func productQuery(v url.Values) (domain.ProductQuery, error) {
	limit, err := queryInt(v, "limit")
	if err != nil {
		return domain.ProductQuery{}, err
	}
	q := domain.ProductQuery{
		CategorySlug: v.Get("category"),
		BrandSlug:    v.Get("brand"),
		Text:         v.Get("q"),
		Sort:         v.Get("sort"),
		Cursor:       v.Get("cursor"),
		Limit:        limit,
		Status:       domain.ProductStatus(v.Get("status")),
	}
	filters := map[string]*domain.AttributeFilter{}
	var keys []string
	verr := &domain.ValidationError{}
	for name, values := range v {
		key, ok := strings.CutPrefix(name, "attr.")
		if !ok || key == "" {
			continue
		}
		bound := ""
		if k, ok := strings.CutSuffix(key, "_min"); ok {
			key, bound = k, "min"
		} else if k, ok := strings.CutSuffix(key, "_max"); ok {
			key, bound = k, "max"
		}
		f, ok := filters[key]
		if !ok {
			f = &domain.AttributeFilter{Key: key}
			filters[key] = f
			keys = append(keys, key)
		}
		if bound == "" {
			for _, val := range values {
				for _, part := range strings.Split(val, ",") {
					if part = strings.TrimSpace(part); part != "" {
						f.Values = append(f.Values, part)
					}
				}
			}
			continue
		}
		n, err := strconv.ParseFloat(values[0], 64)
		if err != nil {
			verr.Add(name, "must be a number")
			continue
		}
		if bound == "min" {
			f.NumMin = &n
		} else {
			f.NumMax = &n
		}
	}
	slices.Sort(keys) // query maps are unordered; keep the SQL stable
	for _, k := range keys {
		q.Attributes = append(q.Attributes, *filters[k])
	}
	return q, verr.Err()
}

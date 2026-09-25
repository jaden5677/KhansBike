//go:build integration

package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/khansbikezone/bikezone-api/internal/app"
	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/testutil"
)

const (
	ownerEmail    = "owner@bikezone.test"
	ownerPassword = "correct horse battery staple"
)

// env is a running API on a fresh database.
type env struct {
	t       *testing.T
	app     *app.App
	srv     *httptest.Server
	logs    *testutil.LogBuffer
	clients atomic.Int32 // hands each client its own address
}

func newEnv(t *testing.T) *env {
	t.Helper()
	logs := &testutil.LogBuffer{}
	a := testutil.App(t, logs)
	srv := httptest.NewServer(a.Router(nil))
	t.Cleanup(srv.Close)
	if _, err := a.Auth.CreateUser(context.Background(), ownerEmail, "Owner", ownerPassword, domain.RoleAdmin); err != nil {
		t.Fatalf("create owner: %v", err)
	}
	return &env{t: t, app: a, srv: srv, logs: logs}
}

// drainJobs runs the queued background jobs, as the worker would.
func (e *env) drainJobs() { testutil.DrainJobs(e.t, e.app) }

// client talks to the API like a browser (cookie jar + CSRF header) or like
// the paired phone (bearer token).
type client struct {
	t      *testing.T
	base   string
	http   *http.Client
	csrf   string
	bearer string
	ip     string // sent as CF-Connecting-IP, so rate limits are per client
}

func (e *env) anon() *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: e.t, base: e.srv.URL, http: &http.Client{Jar: jar}, ip: fmt.Sprintf("198.51.100.%d", e.clients.Add(1))}
}

// admin returns a client signed in as the owner through the login endpoint.
func (e *env) admin() *client {
	c := e.anon()
	var s struct {
		CSRFToken string `json:"csrfToken"`
	}
	c.call(http.MethodPost, "/api/v1/auth/login", map[string]string{"email": ownerEmail, "password": ownerPassword}, http.StatusOK, &s)
	c.csrf = s.CSRFToken
	return c
}

// raw sends a request and returns the response; the caller closes the body.
func (c *client) raw(method, path string, body io.Reader, contentType string, headers map[string]string) *http.Response {
	c.t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, c.base+path, body)
	if err != nil {
		c.t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	if c.ip != "" {
		req.Header.Set("CF-Connecting-IP", c.ip)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

// call sends JSON, checks the status and decodes the response into out
// (when non-nil). It returns the response headers.
func (c *client) call(method, path string, in any, want int, out any, headers ...string) http.Header {
	c.t.Helper()
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			c.t.Fatal(err)
		}
		body = bytes.NewReader(b)
	}
	h := map[string]string{}
	for i := 0; i+1 < len(headers); i += 2 {
		h[headers[i]] = headers[i+1]
	}
	resp := c.raw(method, path, body, "application/json", h)
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		c.t.Fatalf("%s %s: status %d, want %d; body: %s", method, path, resp.StatusCode, want, data)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			c.t.Fatalf("%s %s: decode %s: %v", method, path, data, err)
		}
	}
	return resp.Header
}

// text GETs a path and returns the raw body, checking the status.
func (c *client) text(path string, want int) string {
	c.t.Helper()
	resp := c.raw(http.MethodGet, path, nil, "", nil)
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		c.t.Fatalf("GET %s: status %d, want %d; body: %s", path, resp.StatusCode, want, data)
	}
	return string(data)
}

// upload posts a file as multipart/form-data part "file".
func (c *client) upload(path, filename string, content []byte, want int, out any) {
	c.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		c.t.Fatal(err)
	}
	_, _ = part.Write(content)
	_ = mw.Close()
	resp := c.raw(http.MethodPost, path, &buf, mw.FormDataContentType(), nil)
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		c.t.Fatalf("upload %s: status %d, want %d; body: %s", path, resp.StatusCode, want, data)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			c.t.Fatalf("decode upload response: %v", err)
		}
	}
}

// idOf decodes {"id": ...} from a create call.
type idOf struct {
	ID string `json:"id"`
}

// catalogue is the fixture most tests start from: Wheels > Tyres, with
// wheel_size (required, product level), width (range) and colour (variant
// axis) bound to Tyres, a brand, and three products:
//
//	Rider 20  active, retail public: black (retail 100.00, cost 5.50 USD,
//	          supplier item SUP-778) and red (retail 120.00); 20", 1.95-2.125
//	Trail 26  active, retail NOT public: black (retail 200.00); 26", 2.3
//	Draft 20  draft: never visible publicly
type catalogue struct {
	wheelsID, tyresID string
	colourID          string
	riderID, trailID  string
	draftID           string
}

func (e *env) seedCatalogue(c *client) catalogue {
	var cat catalogue
	var id idOf
	c.call(http.MethodPost, "/api/v1/admin/attributes", map[string]any{
		"key": "wheel_size", "label": "Wheel Size", "dataType": "enum", "unit": "in",
		"options": []map[string]any{{"value": "20", "label": `20"`}, {"value": "26", "label": `26"`}},
	}, http.StatusCreated, &id)
	wheel := id.ID
	c.call(http.MethodPost, "/api/v1/admin/attributes", map[string]any{
		"key": "width", "label": "Width", "dataType": "number_range", "unit": "in",
	}, http.StatusCreated, &id)
	width := id.ID
	c.call(http.MethodPost, "/api/v1/admin/attributes", map[string]any{
		"key": "colour", "label": "Colour", "dataType": "color",
		"options": []map[string]any{
			{"value": "black", "label": "Black", "swatchHex": "#000000"},
			{"value": "red", "label": "Red", "swatchHex": "#d32f2f", "position": 1},
		},
	}, http.StatusCreated, &id)
	cat.colourID = id.ID

	c.call(http.MethodPost, "/api/v1/admin/categories", map[string]any{"name": "Wheels"}, http.StatusCreated, &id)
	cat.wheelsID = id.ID
	c.call(http.MethodPost, "/api/v1/admin/categories", map[string]any{"name": "Tyres", "parentId": cat.wheelsID}, http.StatusCreated, &id)
	cat.tyresID = id.ID
	base := "/api/v1/admin/categories/" + cat.tyresID + "/attributes/"
	c.call(http.MethodPut, base+wheel, map[string]any{"isRequired": true}, http.StatusNoContent, nil)
	c.call(http.MethodPut, base+width, map[string]any{"position": 1}, http.StatusNoContent, nil)
	c.call(http.MethodPut, base+cat.colourID, map[string]any{"position": 2, "isVariantAxis": true}, http.StatusNoContent, nil)

	c.call(http.MethodPost, "/api/v1/admin/brands", map[string]any{"name": "Kenda"}, http.StatusCreated, &id)
	brand := id.ID
	c.call(http.MethodPost, "/api/v1/admin/suppliers", map[string]any{"name": "Secret Supplier Ltd"}, http.StatusCreated, &id)
	supplier := id.ID

	c.call(http.MethodPost, "/api/v1/admin/products", map[string]any{
		"categoryId": cat.tyresID, "brandId": brand, "name": "Rider 20", "status": "active", "retailPriceIsPublic": true,
		"summary":    "A 20 inch BMX tyre",
		"attributes": map[string]any{"wheel_size": "20", "width": map[string]float64{"low": 1.95, "high": 2.125}},
		"variants": []map[string]any{
			{"sku": "RIDER-20-BK", "supplierId": supplier, "supplierItemNo": "SUP-778", "stockStatus": "in_stock",
				"attributes": map[string]any{"colour": "black"},
				"prices":     map[string]string{"retail_ttd": "100.00", "cost_usd": "5.50", "wholesale_ttd": "70.00"}},
			{"sku": "RIDER-20-RD", "stockStatus": "low", "attributes": map[string]any{"colour": "red"},
				"prices": map[string]string{"retail_ttd": "120.00"}},
		},
	}, http.StatusCreated, &id)
	cat.riderID = id.ID
	c.call(http.MethodPost, "/api/v1/admin/products", map[string]any{
		"categoryId": cat.tyresID, "name": "Trail 26", "status": "active",
		"attributes": map[string]any{"wheel_size": "26", "width": map[string]float64{"low": 2.3, "high": 2.3}},
		"variants": []map[string]any{
			{"sku": "TRAIL-26", "attributes": map[string]any{"colour": "black"}, "prices": map[string]string{"retail_ttd": "200.00"}},
		},
	}, http.StatusCreated, &id)
	cat.trailID = id.ID
	c.call(http.MethodPost, "/api/v1/admin/products", map[string]any{
		"categoryId": cat.tyresID, "name": "Draft 20", "status": "draft",
		"variants": []map[string]any{{"sku": "DRAFT-20"}},
	}, http.StatusCreated, &id)
	cat.draftID = id.ID
	return cat
}

// testJPEG is a small real photo-like JPEG.
func testJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 90, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

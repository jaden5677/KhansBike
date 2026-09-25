//go:build integration

package http_test

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestAuthenticationAndCSRF(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	anon := e.anon()

	anon.call(http.MethodGet, "/api/v1/admin/brands", nil, http.StatusUnauthorized, nil)
	anon.call(http.MethodPost, "/api/v1/auth/login", map[string]string{"email": ownerEmail, "password": "wrong password!!"}, http.StatusUnauthorized, nil)
	anon.call(http.MethodPost, "/api/v1/auth/login", map[string]string{"email": "nobody@bikezone.test", "password": ownerPassword}, http.StatusUnauthorized, nil)

	c := e.admin()
	c.call(http.MethodGet, "/api/v1/admin/brands", nil, http.StatusOK, nil)

	// A cookie-authenticated write needs the CSRF token.
	token := c.csrf
	c.csrf = ""
	c.call(http.MethodPost, "/api/v1/admin/brands", map[string]string{"name": "X"}, http.StatusForbidden, nil)
	c.csrf = "forged"
	c.call(http.MethodPost, "/api/v1/admin/brands", map[string]string{"name": "X"}, http.StatusForbidden, nil)
	c.csrf = token
	c.call(http.MethodPost, "/api/v1/admin/brands", map[string]string{"name": "X"}, http.StatusCreated, nil)

	var s struct {
		Kind      string `json:"kind"`
		CSRFToken string `json:"csrfToken"`
	}
	c.call(http.MethodGet, "/api/v1/auth/session", nil, http.StatusOK, &s)
	if s.Kind != "admin" || s.CSRFToken != token {
		t.Errorf("session = %+v", s)
	}

	// Changing the password signs out every other session.
	other := e.admin()
	c.call(http.MethodPut, "/api/v1/auth/password", map[string]string{"currentPassword": ownerPassword, "newPassword": "a brand new passphrase"}, http.StatusNoContent, nil)
	other.call(http.MethodGet, "/api/v1/admin/brands", nil, http.StatusUnauthorized, nil)
	c.call(http.MethodGet, "/api/v1/admin/brands", nil, http.StatusOK, nil)

	c.call(http.MethodPost, "/api/v1/auth/logout", nil, http.StatusNoContent, nil)
	c.call(http.MethodGet, "/api/v1/admin/brands", nil, http.StatusUnauthorized, nil)
}

func TestLoginLockout(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	// Each attempt comes from a different address, as in a distributed
	// guessing attack: the per-IP rate limit never triggers, but the
	// per-account lockout does.
	for i := 0; i < 5; i++ {
		e.anon().call(http.MethodPost, "/api/v1/auth/login", map[string]string{"email": ownerEmail, "password": "wrong password!!"}, http.StatusUnauthorized, nil)
	}
	// Locked: even the right password is refused (with the same vague answer).
	e.anon().call(http.MethodPost, "/api/v1/auth/login", map[string]string{"email": ownerEmail, "password": ownerPassword}, http.StatusUnauthorized, nil)

	// A single address hammering the endpoint is throttled before that.
	burst := e.anon()
	codes := ""
	for i := 0; i < 7; i++ {
		resp := burst.raw(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"x@y.z","password":"nope nope nope"}`), "application/json", nil)
		_ = resp.Body.Close()
		codes += strconv.Itoa(resp.StatusCode) + " "
	}
	if !strings.HasSuffix(codes, "429 ") {
		t.Errorf("login burst from one address = %s, want throttling", codes)
	}
}

// The price tiers are a security boundary: nothing but the retail price, and
// only when the product opts in, may ever reach an anonymous client.
func TestPublicCatalogueNeverLeaksAdminData(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	cat := e.seedCatalogue(e.admin())
	anon := e.anon()

	var page struct {
		Items []struct {
			Name      string `json:"name"`
			FromPrice *struct {
				Amount string `json:"amount"`
			} `json:"fromPrice"`
		} `json:"items"`
		Total int `json:"total"`
	}
	anon.call(http.MethodGet, "/api/v1/products?category=wheels", nil, http.StatusOK, &page) // parent includes descendants
	if page.Total != 2 {
		t.Fatalf("listing total = %d, want 2 (the draft must not appear)", page.Total)
	}
	prices := map[string]string{}
	for _, it := range page.Items {
		prices[it.Name] = "null"
		if it.FromPrice != nil {
			prices[it.Name] = it.FromPrice.Amount
		}
	}
	if prices["Rider 20"] != "100.00" || prices["Trail 26"] != "null" {
		t.Errorf("card prices = %v, want Rider 20 from 100.00 and Trail 26 hidden", prices)
	}

	for _, path := range []string{
		"/api/v1/products/rider-20", "/api/v1/products?category=tyres", "/api/v1/search?q=rider",
		"/api/v1/fitment/20", "/api/v1/categories/tyres/facets",
	} {
		body := anon.text(path, http.StatusOK)
		for _, secret := range []string{"SUP-778", "5.50", "70.00", "cost_usd", "wholesale", "Secret Supplier", "supplierId", "supplierItemNo"} {
			if strings.Contains(body, secret) {
				t.Errorf("GET %s leaks %q", path, secret)
			}
		}
	}

	var detail struct {
		Variants []struct {
			SKU   string `json:"sku"`
			Price *struct {
				Amount string `json:"amount"`
			} `json:"price"`
			Attributes []struct {
				Key, Display string
			} `json:"attributes"`
		} `json:"variants"`
		Attributes []struct {
			Key, Display string
		} `json:"attributes"`
	}
	anon.call(http.MethodGet, "/api/v1/products/rider-20", nil, http.StatusOK, &detail)
	if len(detail.Variants) != 2 || detail.Variants[0].Price == nil || detail.Variants[0].Price.Amount != "100.00" {
		t.Errorf("detail variants = %+v", detail.Variants)
	}
	if got := detail.Attributes; len(got) != 2 || got[0].Display != `20"` || got[1].Display != "1.95–2.125 in" {
		t.Errorf("detail attributes = %+v", got)
	}
	anon.call(http.MethodGet, "/api/v1/products/trail-26", nil, http.StatusOK, &detail)
	if detail.Variants[0].Price != nil {
		t.Errorf("Trail 26 shows a price although its retail price is not public")
	}
	anon.call(http.MethodGet, "/api/v1/products/draft-20", nil, http.StatusNotFound, nil)

	// The admin view of the same product carries every tier and the supplier.
	admin := e.admin()
	body := admin.text("/api/v1/admin/products/"+cat.riderID, http.StatusOK)
	for _, want := range []string{"SUP-778", `"cost_usd"`, `"5.50"`, `"70.00"`} {
		if !strings.Contains(body, want) {
			t.Errorf("admin product view lacks %s", want)
		}
	}

	// Deactivating the parent hides the whole subtree and its products.
	admin.call(http.MethodPut, "/api/v1/admin/categories/"+cat.wheelsID, map[string]any{"name": "Wheels", "isActive": false}, http.StatusOK, nil)
	anon.call(http.MethodGet, "/api/v1/products/rider-20", nil, http.StatusNotFound, nil)
	anon.call(http.MethodGet, "/api/v1/categories/tyres", nil, http.StatusNotFound, nil)
	anon.call(http.MethodGet, "/api/v1/products?category=tyres", nil, http.StatusNotFound, nil)
}

func TestFilteringFacetsAndSearch(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.seedCatalogue(e.admin())
	anon := e.anon()

	names := func(path string) []string {
		var page struct {
			Items []struct {
				Name string `json:"name"`
			} `json:"items"`
		}
		anon.call(http.MethodGet, path, nil, http.StatusOK, &page)
		var out []string
		for _, it := range page.Items {
			out = append(out, it.Name)
		}
		return out
	}
	tests := []struct {
		query string
		want  string
	}{
		{"attr.width_min=2.2", "Trail 26"},                   // ranges overlap: 1.95-2.125 does not reach 2.2
		{"attr.width=2", "Rider 20"},                         // a range containing 2
		{"attr.colour=red", "Rider 20"},                      // variant-level value
		{"attr.wheel_size=20,26", "Rider 20,Trail 26"},       // OR within one attribute
		{"attr.wheel_size=26&attr.colour=red", ""},           // AND across attributes
		{"attr.colour=black&sort=name", "Rider 20,Trail 26"}, // both have a black variant
	}
	for _, tc := range tests {
		got := strings.Join(names("/api/v1/products?category=tyres&"+tc.query), ",")
		if got != tc.want {
			t.Errorf("%s: got [%s], want [%s]", tc.query, got, tc.want)
		}
	}
	anon.call(http.MethodGet, "/api/v1/products?category=tyres&attr.valve=av", nil, http.StatusUnprocessableEntity, nil)
	anon.call(http.MethodGet, "/api/v1/products?category=tyres&attr.colour=purple", nil, http.StatusUnprocessableEntity, nil)

	// Facets are disjunctive: selecting red still counts black.
	var facets struct {
		Items []struct {
			Key    string `json:"key"`
			Values []struct {
				Value string `json:"value"`
				Count int    `json:"count"`
			} `json:"values"`
			NumMin *float64 `json:"numMin"`
			NumMax *float64 `json:"numMax"`
		} `json:"items"`
	}
	anon.call(http.MethodGet, "/api/v1/categories/tyres/facets?attr.colour=red", nil, http.StatusOK, &facets)
	got := map[string]string{}
	for _, f := range facets.Items {
		var parts []string
		for _, v := range f.Values {
			parts = append(parts, v.Value+":"+strconv.Itoa(v.Count))
		}
		if f.NumMin != nil {
			parts = append(parts, "min", strconv.FormatFloat(*f.NumMin, 'f', -1, 64), "max", strconv.FormatFloat(*f.NumMax, 'f', -1, 64))
		}
		got[f.Key] = strings.Join(parts, " ")
	}
	want := map[string]string{"colour": "black:2 red:1", "wheel_size": "20:1", "width": "min 1.95 max 2.125"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("facet %s = %q, want %q", k, got[k], v)
		}
	}

	// Pagination walks every product exactly once.
	var first struct {
		Items      []struct{ Name string } `json:"items"`
		NextCursor string                  `json:"nextCursor"`
	}
	anon.call(http.MethodGet, "/api/v1/products?category=tyres&sort=name&limit=1", nil, http.StatusOK, &first)
	if len(first.Items) != 1 || first.NextCursor == "" {
		t.Fatalf("first page = %+v", first)
	}
	second := names("/api/v1/products?category=tyres&sort=name&limit=1&cursor=" + first.NextCursor)
	if first.Items[0].Name != "Rider 20" || strings.Join(second, ",") != "Trail 26" {
		t.Errorf("pages = %v then %v", first.Items, second)
	}
	anon.call(http.MethodGet, "/api/v1/products?category=tyres&sort=-created&cursor="+first.NextCursor, nil, http.StatusUnprocessableEntity, nil)

	// Search: variants and attribute values reach the index through the
	// reindex job the database triggers enqueue.
	e.drainJobs()
	search := func(q string) string {
		var res struct {
			Groups []struct {
				Products []struct{ Name string } `json:"products"`
			} `json:"groups"`
		}
		anon.call(http.MethodGet, "/api/v1/search?q="+q, nil, http.StatusOK, &res)
		var out []string
		for _, g := range res.Groups {
			for _, p := range g.Products {
				out = append(out, p.Name)
			}
		}
		return strings.Join(out, ",")
	}
	for q, want := range map[string]string{
		"rider":       "Rider 20",
		"RIDER-20-RD": "Rider 20", // SKU
		"kenda":       "Rider 20", // brand
		"ridr":        "Rider 20", // typo
		"tyres%20bmx": "Rider 20", // category name + summary word
		"trail":       "Trail 26",
		"draft":       "", // drafts are never searchable publicly
	} {
		if got := search(q); got != want {
			t.Errorf("search %q = %q, want %q", q, got, want)
		}
	}
	var sugg struct {
		Items []struct{ Name string } `json:"items"`
	}
	anon.call(http.MethodGet, "/api/v1/search/suggest?q=rid", nil, http.StatusOK, &sugg)
	if len(sugg.Items) != 1 || sugg.Items[0].Name != "Rider 20" {
		t.Errorf("suggest = %+v", sugg.Items)
	}
}

func TestProductEditingConcurrencyAndHistory(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	admin := e.admin()
	cat := e.seedCatalogue(admin)
	path := "/api/v1/admin/products/" + cat.riderID

	var p struct {
		Variants []struct {
			ID     string                       `json:"id"`
			SKU    string                       `json:"sku"`
			Prices map[string]map[string]string `json:"prices"`
		} `json:"variants"`
	}
	h := admin.call(http.MethodGet, path, nil, http.StatusOK, &p)
	etag := h.Get("ETag")
	doc := func(retail string) map[string]any {
		return map[string]any{
			"categoryId": cat.tyresID, "name": "Rider 20", "status": "active", "retailPriceIsPublic": true,
			"attributes": map[string]any{"wheel_size": "20"},
			"variants": []map[string]any{{"id": p.Variants[0].ID, "sku": "RIDER-20-BK",
				"attributes": map[string]any{"colour": "black"}, "prices": map[string]string{"retail_ttd": retail}}},
		}
	}

	// The phone saves first...
	h2 := admin.call(http.MethodPut, path, doc("110.00"), http.StatusOK, &p, "If-Match", etag)
	// ...so the desktop's save, based on the old version, is refused.
	admin.call(http.MethodPut, path, doc("90.00"), http.StatusPreconditionFailed, nil, "If-Match", etag)
	admin.call(http.MethodPut, path, doc("95.00"), http.StatusOK, &p, "If-Match", h2.Get("ETag"))

	// The red variant was dropped; the untouched cost tier kept its value.
	if len(p.Variants) != 1 || p.Variants[0].Prices["retail_ttd"]["amount"] != "95.00" || p.Variants[0].Prices["cost_usd"]["amount"] != "5.50" {
		t.Errorf("after edits: %+v", p.Variants)
	}

	// Required attributes are enforced when publishing, not for drafts.
	bad := doc("95.00")
	bad["attributes"] = map[string]any{}
	admin.call(http.MethodPut, path, bad, http.StatusUnprocessableEntity, nil)
	bad["status"] = "draft"
	admin.call(http.MethodPut, path, bad, http.StatusOK, nil)

	var audit struct {
		Items []struct {
			Action string `json:"action"`
		} `json:"items"`
	}
	admin.call(http.MethodGet, "/api/v1/admin/audit?entityType=product&entityId="+cat.riderID, nil, http.StatusOK, &audit)
	if len(audit.Items) != 4 || audit.Items[0].Action != "product.update" || audit.Items[3].Action != "product.create" {
		t.Errorf("audit trail = %+v", audit.Items)
	}
}

func TestMediaPipeline(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	admin := e.admin()
	cat := e.seedCatalogue(admin)

	var asset struct {
		ID         string `json:"id"`
		Status     string `json:"status"`
		Width      int    `json:"width"`
		Renditions []struct {
			URL    string `json:"url"`
			Format string `json:"format"`
			Width  int    `json:"width"`
		} `json:"renditions"`
	}
	photo := testJPEG(t, 800, 600)
	admin.upload("/api/v1/admin/media", "IMG_0001.jpg", photo, http.StatusCreated, &asset)
	if asset.Status != "pending" {
		t.Fatalf("new upload status = %s", asset.Status)
	}
	admin.upload("/api/v1/admin/media", "copy.jpg", photo, http.StatusOK, nil) // identical bytes dedupe
	admin.upload("/api/v1/admin/media", "notes.txt", []byte("not an image"), http.StatusUnprocessableEntity, nil)

	e.drainJobs()
	admin.call(http.MethodGet, "/api/v1/admin/media/"+asset.ID, nil, http.StatusOK, &asset)
	if asset.Status != "ready" || len(asset.Renditions) != 4 { // 320 and 640, WebP and JPEG
		t.Fatalf("processed asset = %+v", asset)
	}

	admin.call(http.MethodPost, "/api/v1/admin/products/"+cat.riderID+"/media",
		map[string]any{"assetId": asset.ID, "role": "hero", "altText": "Rider tyre"}, http.StatusCreated, nil)
	var detail struct {
		Images []struct {
			Role    string `json:"role"`
			Sources []struct {
				URL string `json:"url"`
			} `json:"sources"`
		} `json:"images"`
	}
	anon := e.anon()
	anon.call(http.MethodGet, "/api/v1/products/rider-20", nil, http.StatusOK, &detail)
	if len(detail.Images) != 1 || detail.Images[0].Role != "hero" || len(detail.Images[0].Sources) != 4 {
		t.Fatalf("public images = %+v", detail.Images)
	}
	// The rendition URL is served by the API (fs backend) and cacheable forever.
	u := detail.Images[0].Sources[0].URL
	path := u[strings.Index(u, "/media/"):]
	resp := anon.raw(http.MethodGet, path, nil, "", nil)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
		t.Errorf("GET %s = %d, cache %q", path, resp.StatusCode, resp.Header.Get("Cache-Control"))
	}
	// Originals (which keep camera metadata) are never served.
	anon.text("/media/originals/00/"+strings.Repeat("0", 64), http.StatusNotFound)
}

func TestMailingList(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	anon := e.anon()

	for i := 0; i < 2; i++ { // a double-submitted form sends one email
		anon.call(http.MethodPost, "/api/v1/subscribers", map[string]string{"email": "rider@example.org", "name": "=cmd|' /C calc'!A0"}, http.StatusAccepted, nil)
	}
	e.anon().call(http.MethodPost, "/api/v1/subscribers", map[string]string{"email": "not-an-email"}, http.StatusUnprocessableEntity, nil)
	e.drainJobs()

	sent := regexp.MustCompile(`confirm\?token=([A-Za-z0-9_-]+)`).FindAllStringSubmatch(e.logs.String(), -1)
	if len(sent) != 1 {
		t.Fatalf("sent %d confirmation emails, want 1", len(sent))
	}
	token := sent[0][1]
	anon.call(http.MethodPost, "/api/v1/subscribers/confirm", map[string]string{"token": token}, http.StatusNoContent, nil)
	anon.call(http.MethodPost, "/api/v1/subscribers/confirm", map[string]string{"token": token}, http.StatusNotFound, nil) // single use

	admin := e.admin()
	var stats map[string]int
	admin.call(http.MethodGet, "/api/v1/admin/subscribers/stats", nil, http.StatusOK, &stats)
	if stats["confirmed"] != 1 {
		t.Errorf("stats = %v", stats)
	}
	csv := admin.text("/api/v1/admin/subscribers/export", http.StatusOK)
	if !strings.Contains(csv, "rider@example.org") || !strings.Contains(csv, `'=cmd`) {
		t.Errorf("export = %q (formula must be neutralised)", csv)
	}
	// Signing up again when already confirmed changes nothing and sends nothing.
	e.anon().call(http.MethodPost, "/api/v1/subscribers", map[string]string{"email": "rider@example.org"}, http.StatusAccepted, nil)
	e.drainJobs()
	if n := strings.Count(e.logs.String(), "confirm?token="); n != 1 {
		t.Errorf("confirmed address was emailed again (%d emails)", n)
	}
}

func TestDevicePairing(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	admin := e.admin()

	var pc struct {
		Code string `json:"code"`
		URL  string `json:"url"`
	}
	admin.call(http.MethodPost, "/api/v1/admin/devices/pairing-codes", nil, http.StatusCreated, &pc)
	if !strings.Contains(pc.URL, pc.Code) {
		t.Errorf("pairing URL %q does not carry the code", pc.URL)
	}
	phone := e.anon()
	var paired struct {
		Token  string `json:"token"`
		Device struct {
			ID string `json:"id"`
		} `json:"device"`
	}
	typed := strings.ToLower(pc.Code[:4] + "-" + pc.Code[4:])
	phone.call(http.MethodPost, "/api/v1/auth/devices", map[string]string{"code": typed, "name": "Owner's phone"}, http.StatusCreated, &paired)
	phone.call(http.MethodPost, "/api/v1/auth/devices", map[string]string{"code": pc.Code}, http.StatusUnauthorized, nil) // single use

	phone.bearer = paired.Token
	phone.call(http.MethodPost, "/api/v1/admin/brands", map[string]string{"name": "From Phone"}, http.StatusCreated, nil) // no CSRF needed
	phone.call(http.MethodPost, "/api/v1/admin/devices/pairing-codes", nil, http.StatusForbidden, nil)

	admin.call(http.MethodDelete, "/api/v1/admin/devices/"+paired.Device.ID, nil, http.StatusNoContent, nil)
	phone.call(http.MethodGet, "/api/v1/admin/brands", nil, http.StatusUnauthorized, nil)
}

func TestTaxonomyChanges(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	admin := e.admin()
	cat := e.seedCatalogue(admin)
	anon := e.anon()

	// Moving a category re-roots its subtree.
	var parts idOf
	admin.call(http.MethodPost, "/api/v1/admin/categories", map[string]any{"name": "Parts"}, http.StatusCreated, &parts)
	var moved struct {
		Path string `json:"path"`
	}
	admin.call(http.MethodPut, "/api/v1/admin/categories/"+cat.tyresID, map[string]any{"name": "Tyres", "parentId": parts.ID}, http.StatusOK, &moved)
	if moved.Path != "parts.tyres" {
		t.Errorf("moved path = %q", moved.Path)
	}
	var page struct {
		Total int `json:"total"`
	}
	anon.call(http.MethodGet, "/api/v1/products?category=parts", nil, http.StatusOK, &page)
	if page.Total != 2 {
		t.Errorf("products under the new parent = %d, want 2", page.Total)
	}
	admin.call(http.MethodPut, "/api/v1/admin/categories/"+parts.ID, map[string]any{"name": "Parts", "parentId": cat.tyresID}, http.StatusUnprocessableEntity, nil) // cycle
	admin.call(http.MethodDelete, "/api/v1/admin/categories/"+cat.tyresID, nil, http.StatusConflict, nil)                                                           // has products

	// The form schema follows the bindings, and its version changes with them.
	var fs1, fs2 struct {
		Version int `json:"version"`
		Fields  []struct {
			Key string `json:"key"`
		} `json:"fields"`
	}
	admin.call(http.MethodGet, "/api/v1/admin/categories/"+cat.tyresID+"/form-schema", nil, http.StatusOK, &fs1)
	admin.call(http.MethodDelete, "/api/v1/admin/categories/"+cat.tyresID+"/attributes/"+cat.colourID, nil, http.StatusNoContent, nil)
	admin.call(http.MethodGet, "/api/v1/admin/categories/"+cat.tyresID+"/form-schema", nil, http.StatusOK, &fs2)
	if len(fs1.Fields) != 3 || len(fs2.Fields) != 2 || fs1.Version == fs2.Version {
		t.Errorf("form schema before %+v, after %+v", fs1, fs2)
	}
	// Unbinding removed the values and rebuilt the filter projection.
	anon.call(http.MethodGet, "/api/v1/products?category=tyres&attr.colour=red", nil, http.StatusUnprocessableEntity, nil)
	body := admin.text("/api/v1/admin/products/"+cat.riderID, http.StatusOK)
	if strings.Contains(body, `"colour"`) {
		t.Error("unbound attribute still present on the product")
	}
	// An attribute still bound elsewhere or used cannot be deleted.
	var attrs struct {
		Items []struct {
			ID  string `json:"id"`
			Key string `json:"key"`
		} `json:"items"`
	}
	admin.call(http.MethodGet, "/api/v1/admin/attributes", nil, http.StatusOK, &attrs)
	for _, a := range attrs.Items {
		if a.Key == "wheel_size" {
			admin.call(http.MethodDelete, "/api/v1/admin/attributes/"+a.ID, nil, http.StatusConflict, nil)
		}
	}
	admin.call(http.MethodDelete, "/api/v1/admin/attributes/"+cat.colourID, nil, http.StatusNoContent, nil) // now unused
}

func TestErrorShapes(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	anon := e.anon()
	resp := anon.raw(http.MethodGet, "/api/v1/nope", nil, "", nil)
	defer func() { _ = resp.Body.Close() }()
	var p struct {
		Status int    `json:"status"`
		Title  string `json:"title"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&p)
	if resp.StatusCode != http.StatusNotFound || resp.Header.Get("Content-Type") != "application/problem+json" || p.Status != 404 {
		t.Errorf("unknown endpoint: %d %q %+v", resp.StatusCode, resp.Header.Get("Content-Type"), p)
	}
	admin := e.admin()
	admin.call(http.MethodPost, "/api/v1/admin/brands", map[string]any{"brandName": "unknown field"}, http.StatusBadRequest, nil)
	admin.call(http.MethodGet, "/api/v1/admin/products/not-a-uuid", nil, http.StatusNotFound, nil)
	if got := anon.text("/healthz", http.StatusOK); !strings.Contains(got, "ok") {
		t.Errorf("healthz = %q", got)
	}
	if got := anon.text("/readyz", http.StatusOK); !strings.Contains(got, `"db":"ok"`) {
		t.Errorf("readyz = %q", got)
	}
}

package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/platform"
)

// This file holds the catalogue queries whose shape depends on the request
// (optional filters, sort orders, free-text search). They are hand-written
// because sqlc cannot express optional predicates, and because the trigram
// functions (word_similarity, <%) come from the pg_trgm extension, which sqlc
// does not model.
//
// Filtering semantics: a product matches the attribute filters when at least
// one of its variants, combined with the product-level attributes, satisfies
// every filter. Each filter is a JSONPath predicate (built by the service from
// the attribute's data type) evaluated against that combined object,
// p.attrs || v.attrs. So "thread 1/2 + colour red" does not match a pedal that
// only comes as 1/2-black and 9/16-red.
//
// Search blends full-text rank with trigram similarity for typo tolerance:
// rank = ts_rank_cd * 0.7 + word_similarity * 0.3. The query is parsed with
// both the 'simple' and 'english' configurations because the search vector
// indexes names, SKUs and option labels unstemmed ('simple') and prose
// stemmed ('english'); a single configuration misses one or the other.

// ProductFilter is a compiled product query.
type ProductFilter struct {
	// PublicOnly restricts to active products whose category and every
	// ancestor category is active: the public visibility rule.
	PublicOnly bool
	// Status narrows admin listings ("" = any status). Ignored with PublicOnly.
	Status       domain.ProductStatus
	CategoryPath string // ltree path; the category and all its descendants
	BrandID      *uuid.UUID
	Text         string   // free-text query; "" for none
	AttrPaths    []string // JSONPath predicates a single variant must all satisfy
}

// queryBuilder accumulates positional arguments for a dynamic query.
type queryBuilder struct {
	args []any
}

// arg appends v and returns its placeholder.
func (b *queryBuilder) arg(v any) string {
	b.args = append(b.args, v)
	return "$" + strconv.Itoa(len(b.args))
}

// productQuery is the FROM/WHERE shared by listing, counting and faceting.
type productQuery struct {
	b     queryBuilder
	from  string
	where []string
	rank  string // relevance expression; empty without a text query
}

// variantAlias is the joined variant when attribute filters are applied
// per-variant directly (facets), or "" to use a correlated EXISTS (listings).
func newProductQuery(f ProductFilter, variantAlias string) *productQuery {
	q := &productQuery{from: "products p JOIN categories c ON c.id = p.category_id"}
	if variantAlias != "" {
		q.from += " JOIN product_variants " + variantAlias + " ON " + variantAlias + ".product_id = p.id"
	}
	if f.PublicOnly {
		q.where = append(q.where, "p.status = 'active'",
			"NOT EXISTS (SELECT 1 FROM categories anc WHERE anc.path @> c.path AND NOT anc.is_active)")
	} else if f.Status != "" {
		q.where = append(q.where, "p.status = "+q.b.arg(string(f.Status))+"::product_status")
	}
	if f.CategoryPath != "" {
		q.where = append(q.where, "c.path <@ "+q.b.arg(f.CategoryPath)+"::ltree")
	}
	if f.BrandID != nil {
		q.where = append(q.where, "p.brand_id = "+q.b.arg(*f.BrandID))
	}
	if f.Text != "" {
		t := q.b.arg(f.Text)
		q.from += " CROSS JOIN (SELECT websearch_to_tsquery('simple', f_unaccent(" + t + ")) || " +
			"websearch_to_tsquery('english', f_unaccent(" + t + ")) AS tsq) sq"
		q.where = append(q.where, "(p.search_vector @@ sq.tsq OR "+t+" <% p.name)")
		q.rank = "(coalesce(ts_rank_cd(p.search_vector, sq.tsq), 0)::float8 * 0.7 + word_similarity(" + t + ", p.name)::float8 * 0.3)"
	}
	if len(f.AttrPaths) > 0 {
		if variantAlias != "" {
			for _, path := range f.AttrPaths {
				q.where = append(q.where, "(p.attrs || "+variantAlias+".attrs) @? "+q.b.arg(path)+"::jsonpath")
			}
		} else {
			conds := make([]string, len(f.AttrPaths))
			for i, path := range f.AttrPaths {
				conds[i] = "(p.attrs || v.attrs) @? " + q.b.arg(path) + "::jsonpath"
			}
			q.where = append(q.where, "EXISTS (SELECT 1 FROM product_variants v WHERE v.product_id = p.id AND "+
				strings.Join(conds, " AND ")+")")
		}
	}
	return q
}

func (q *productQuery) whereSQL(extra ...string) string {
	conds := append(append([]string{}, q.where...), extra...)
	if len(conds) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(conds, " AND ")
}

// sortKey is one ORDER BY term; a sort order is a list of them ending in a
// unique key (p.id) so keyset pagination never skips or repeats a row.
type sortKey struct {
	expr string
	desc bool
}

func sortKeys(sort, rank string) ([]sortKey, error) {
	switch sort {
	case domain.SortFeatured:
		return []sortKey{{"p.is_featured", true}, {"p.name", false}, {"p.id", false}}, nil
	case domain.SortName:
		return []sortKey{{"p.name", false}, {"p.id", false}}, nil
	case domain.SortNewest:
		return []sortKey{{"p.created_at", true}, {"p.id", true}}, nil
	case domain.SortUpdated:
		return []sortKey{{"p.updated_at", true}, {"p.id", true}}, nil
	case domain.SortRelevance:
		if rank == "" {
			return nil, domain.Invalid("sort", "relevance sorting needs a search query")
		}
		return []sortKey{{rank, true}, {"p.id", false}}, nil
	default:
		return nil, domain.Invalid("sort", "unknown sort %q", sort)
	}
}

// productCursor holds the sort-key values of the last row on a page. Only the
// fields used by Sort are set; Sort guards against reusing a cursor with a
// different order.
type productCursor struct {
	Sort     string     `json:"s"`
	Featured *bool      `json:"f,omitempty"`
	Name     *string    `json:"n,omitempty"`
	At       *time.Time `json:"t,omitempty"`
	Rank     *float64   `json:"r,omitempty"`
	ID       uuid.UUID  `json:"i"`
}

// values returns the cursor's values in sort-key order, or false when the
// cursor lacks a value its sort order needs.
func (c productCursor) values() ([]any, bool) {
	switch c.Sort {
	case domain.SortFeatured:
		if c.Featured != nil && c.Name != nil {
			return []any{*c.Featured, *c.Name, c.ID}, true
		}
	case domain.SortName:
		if c.Name != nil {
			return []any{*c.Name, c.ID}, true
		}
	case domain.SortNewest, domain.SortUpdated:
		if c.At != nil {
			return []any{*c.At, c.ID}, true
		}
	case domain.SortRelevance:
		if c.Rank != nil {
			return []any{*c.Rank, c.ID}, true
		}
	}
	return nil, false
}

// afterCursor builds "row comes after the cursor" for keys that may mix
// directions: (k1 > v1) OR (k1 = v1 AND k2 > v2) OR ..., with < for DESC.
func afterCursor(b *queryBuilder, keys []sortKey, vals []any) string {
	var ors []string
	for i, k := range keys {
		ands := make([]string, 0, i+1)
		for j := 0; j < i; j++ {
			ands = append(ands, keys[j].expr+" = "+b.arg(vals[j]))
		}
		op := " > "
		if k.desc {
			op = " < "
		}
		ands = append(ands, k.expr+op+b.arg(vals[i]))
		ors = append(ors, "("+strings.Join(ands, " AND ")+")")
	}
	return "(" + strings.Join(ors, " OR ") + ")"
}

// ListProducts returns one keyset page of products matching f, in the given
// sort order, with Category and Brand populated on each Product.
func (q *Queries) ListProducts(ctx context.Context, f ProductFilter, sort, cursor string, limit int) (items []domain.ProductSummary, next string, err error) {
	pq := newProductQuery(f, "")
	keys, err := sortKeys(sort, pq.rank)
	if err != nil {
		return nil, "", err
	}
	var extra []string
	if cursor != "" {
		var c productCursor
		if err := platform.DecodeCursor(cursor, &c); err != nil || c.Sort != sort {
			return nil, "", domain.Invalid("cursor", "cursor is invalid or belongs to a different sort order")
		}
		vals, ok := c.values()
		if !ok {
			return nil, "", domain.Invalid("cursor", "cursor is invalid")
		}
		extra = append(extra, afterCursor(&pq.b, keys, vals))
	}
	order := make([]string, len(keys))
	for i, k := range keys {
		order[i] = k.expr
		if k.desc {
			order[i] += " DESC"
		}
	}
	rank := "0::float8"
	if pq.rank != "" {
		rank = pq.rank
	}
	sql := `SELECT p.id, p.category_id, p.brand_id, p.name, p.slug, p.summary, p.status, p.is_featured,
	               p.retail_price_is_public, p.created_at, p.updated_at, p.published_at,
	               c.name, c.slug, c.path::text, b.name, b.slug, ` + rank + `
	        FROM ` + pq.from + ` LEFT JOIN brands b ON b.id = p.brand_id` +
		pq.whereSQL(extra...) +
		` ORDER BY ` + strings.Join(order, ", ") +
		` LIMIT ` + pq.b.arg(limit+1) // one extra row reveals whether a next page exists

	rows, err := q.db.Query(ctx, sql, pq.b.args...)
	if err != nil {
		return nil, "", fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()
	var ranks []float64
	for rows.Next() {
		var (
			s                    domain.ProductSummary
			p                    = &s.Product
			brandName, brandSlug *string
			r                    float64
		)
		p.Category = &domain.Category{}
		if err := rows.Scan(&p.ID, &p.CategoryID, &p.BrandID, &p.Name, &p.Slug, &p.Summary, &p.Status,
			&p.IsFeatured, &p.RetailPriceIsPublic, &p.CreatedAt, &p.UpdatedAt, &p.PublishedAt,
			&p.Category.Name, &p.Category.Slug, &p.Category.Path, &brandName, &brandSlug, &r); err != nil {
			return nil, "", fmt.Errorf("scan product: %w", err)
		}
		p.Category.ID = p.CategoryID
		if p.BrandID != nil && brandName != nil && brandSlug != nil {
			p.Brand = &domain.Brand{ID: *p.BrandID, Name: *brandName, Slug: *brandSlug}
		}
		items = append(items, s)
		ranks = append(ranks, r)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("list products: %w", err)
	}
	if len(items) <= limit {
		return items, "", nil
	}
	items = items[:limit]
	last := items[limit-1].Product
	c := productCursor{Sort: sort, ID: last.ID}
	switch sort {
	case domain.SortFeatured:
		c.Featured, c.Name = &last.IsFeatured, &last.Name
	case domain.SortName:
		c.Name = &last.Name
	case domain.SortNewest:
		c.At = &last.CreatedAt
	case domain.SortUpdated:
		c.At = &last.UpdatedAt
	case domain.SortRelevance:
		c.Rank = &ranks[limit-1]
	}
	next, err = platform.EncodeCursor(c)
	return items, next, err
}

// CountProducts counts every product matching f (ignoring pagination).
func (q *Queries) CountProducts(ctx context.Context, f ProductFilter) (int, error) {
	pq := newProductQuery(f, "")
	var n int
	err := q.db.QueryRow(ctx, "SELECT count(*) FROM "+pq.from+pq.whereSQL(), pq.b.args...).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count products: %w", err)
	}
	return n, nil
}

// FacetCounts returns, per attribute key, how many products carry each value
// among the products matching f. A product counts for a value when one of its
// variants both satisfies f's attribute filters and has that value (at
// product or variant level). Array values (multi_enum) count per element.
func (q *Queries) FacetCounts(ctx context.Context, f ProductFilter, keys []string) (map[string]map[string]int, error) {
	out := make(map[string]map[string]int, len(keys))
	if len(keys) == 0 {
		return out, nil
	}
	pq := newProductQuery(f, "v")
	sql := `SELECT kv.key, val.v, count(DISTINCT p.id)
	        FROM ` + pq.from + `
	        CROSS JOIN LATERAL jsonb_each(p.attrs || v.attrs) AS kv(key, value)
	        CROSS JOIN LATERAL jsonb_array_elements_text(
	            CASE jsonb_typeof(kv.value) WHEN 'array' THEN kv.value ELSE jsonb_build_array(kv.value) END) AS val(v)` +
		pq.whereSQL("kv.key = ANY("+pq.b.arg(keys)+"::text[])") +
		` GROUP BY kv.key, val.v`
	rows, err := q.db.Query(ctx, sql, pq.b.args...)
	if err != nil {
		return nil, fmt.Errorf("facet counts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		var n int
		if err := rows.Scan(&key, &value, &n); err != nil {
			return nil, fmt.Errorf("scan facet count: %w", err)
		}
		if out[key] == nil {
			out[key] = map[string]int{}
		}
		out[key][value] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("facet counts: %w", err)
	}
	return out, nil
}

// NumericBounds is the observed range of a numeric attribute.
type NumericBounds struct{ Min, Max float64 }

// FacetRanges returns, per numeric attribute key, the smallest and largest
// value among the products matching f. number_range values contribute their
// low and high ends.
func (q *Queries) FacetRanges(ctx context.Context, f ProductFilter, keys []string) (map[string]NumericBounds, error) {
	out := make(map[string]NumericBounds, len(keys))
	if len(keys) == 0 {
		return out, nil
	}
	pq := newProductQuery(f, "v")
	sql := `SELECT kv.key,
	               min(CASE WHEN jsonb_typeof(kv.value) = 'number' THEN kv.value::float8 ELSE (kv.value->>'low')::float8 END),
	               max(CASE WHEN jsonb_typeof(kv.value) = 'number' THEN kv.value::float8 ELSE (kv.value->>'high')::float8 END)
	        FROM ` + pq.from + `
	        CROSS JOIN LATERAL jsonb_each(p.attrs || v.attrs) AS kv(key, value)` +
		pq.whereSQL("kv.key = ANY("+pq.b.arg(keys)+"::text[])") +
		` GROUP BY kv.key`
	rows, err := q.db.Query(ctx, sql, pq.b.args...)
	if err != nil {
		return nil, fmt.Errorf("facet ranges: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var lo, hi *float64
		if err := rows.Scan(&key, &lo, &hi); err != nil {
			return nil, fmt.Errorf("scan facet range: %w", err)
		}
		if lo != nil && hi != nil {
			out[key] = NumericBounds{Min: *lo, Max: *hi}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("facet ranges: %w", err)
	}
	return out, nil
}

// ListProductsGrouped returns the products matching f grouped by category,
// at most perGroup per group, each group with its full match count. With a
// text query, products and groups are ordered by relevance; without one, by
// name within the category tree order.
func (q *Queries) ListProductsGrouped(ctx context.Context, f ProductFilter, perGroup int) ([]domain.SearchGroup, error) {
	pq := newProductQuery(f, "")
	rank, groupOrder, rowOrder := "0::float8", "c_path", "h.name, h.id"
	if pq.rank != "" {
		rank, groupOrder, rowOrder = pq.rank, "best DESC, c_path", "h.rank DESC, h.id"
	}
	sql := `WITH hits AS (
	            SELECT p.id, p.category_id, p.brand_id, p.name, p.slug, p.summary, p.status, p.is_featured,
	                   p.retail_price_is_public, p.created_at, p.updated_at, p.published_at,
	                   c.name AS c_name, c.slug AS c_slug, c.path AS c_path, b.name AS b_name, b.slug AS b_slug,
	                   ` + rank + ` AS rank
	            FROM ` + pq.from + ` LEFT JOIN brands b ON b.id = p.brand_id` + pq.whereSQL() + `
	        ), ranked AS (
	            SELECT h.*,
	                   row_number() OVER (PARTITION BY h.category_id ORDER BY ` + rowOrder + `) AS rn,
	                   count(*) OVER (PARTITION BY h.category_id) AS group_total,
	                   max(h.rank) OVER (PARTITION BY h.category_id) AS best
	            FROM hits h
	        )
	        SELECT id, category_id, brand_id, name, slug, summary, status, is_featured, retail_price_is_public,
	               created_at, updated_at, published_at, c_name, c_slug, c_path::text, b_name, b_slug, group_total
	        FROM ranked
	        WHERE rn <= ` + pq.b.arg(perGroup) + `
	        ORDER BY ` + groupOrder + `, rn`
	rows, err := q.db.Query(ctx, sql, pq.b.args...)
	if err != nil {
		return nil, fmt.Errorf("list grouped products: %w", err)
	}
	defer rows.Close()
	var groups []domain.SearchGroup
	for rows.Next() {
		var (
			s                    domain.ProductSummary
			p                    = &s.Product
			brandName, brandSlug *string
			total                int
		)
		p.Category = &domain.Category{}
		if err := rows.Scan(&p.ID, &p.CategoryID, &p.BrandID, &p.Name, &p.Slug, &p.Summary, &p.Status,
			&p.IsFeatured, &p.RetailPriceIsPublic, &p.CreatedAt, &p.UpdatedAt, &p.PublishedAt,
			&p.Category.Name, &p.Category.Slug, &p.Category.Path, &brandName, &brandSlug, &total); err != nil {
			return nil, fmt.Errorf("scan grouped product: %w", err)
		}
		p.Category.ID = p.CategoryID
		if p.BrandID != nil && brandName != nil && brandSlug != nil {
			p.Brand = &domain.Brand{ID: *p.BrandID, Name: *brandName, Slug: *brandSlug}
		}
		if n := len(groups); n == 0 || groups[n-1].CategoryID != p.CategoryID {
			groups = append(groups, domain.SearchGroup{
				CategoryID:   p.CategoryID,
				CategoryName: p.Category.Name,
				CategorySlug: p.Category.Slug,
				Total:        total,
			})
		}
		g := &groups[len(groups)-1]
		g.Products = append(g.Products, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list grouped products: %w", err)
	}
	return groups, nil
}

// SuggestProducts is the typeahead: publicly visible products whose name
// contains the text or is trigram-similar to it (typos), prefix matches first.
func (q *Queries) SuggestProducts(ctx context.Context, text string, limit int) ([]domain.Suggestion, error) {
	var b queryBuilder
	t := b.arg(text)
	escaped := escapeLike(text)
	sql := `SELECT p.slug, p.name
	        FROM products p JOIN categories c ON c.id = p.category_id
	        WHERE p.status = 'active'
	          AND NOT EXISTS (SELECT 1 FROM categories anc WHERE anc.path @> c.path AND NOT anc.is_active)
	          AND (` + t + ` <% p.name OR p.name ILIKE ` + b.arg("%"+escaped+"%") + `)
	        ORDER BY (p.name ILIKE ` + b.arg(escaped+"%") + `) DESC, word_similarity(` + t + `, p.name) DESC, p.name
	        LIMIT ` + b.arg(limit)
	rows, err := q.db.Query(ctx, sql, b.args...)
	if err != nil {
		return nil, fmt.Errorf("suggest products: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Suggestion, error) {
		var s domain.Suggestion
		err := r.Scan(&s.Slug, &s.Name)
		return s, err
	})
	if err != nil {
		return nil, fmt.Errorf("suggest products: %w", err)
	}
	return out, nil
}

// escapeLike neutralises LIKE metacharacters in user input so "50%" matches
// a literal percent sign instead of acting as a wildcard.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// RefreshSearchVectors rebuilds the search vectors of up to batch products
// with ids after afterID, in id order, and returns the last id processed (or
// uuid.Nil when there was nothing left). Batching keeps each statement's row
// locks short while a full reindex runs alongside admin edits.
func (q *Queries) RefreshSearchVectors(ctx context.Context, afterID uuid.UUID, batch int) (uuid.UUID, error) {
	rows, err := q.db.Query(ctx, `SELECT id FROM products WHERE id > $1 ORDER BY id LIMIT $2`, afterID, batch)
	if err != nil {
		return uuid.Nil, fmt.Errorf("select reindex batch: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return uuid.Nil, fmt.Errorf("select reindex batch: %w", err)
	}
	if len(ids) == 0 {
		return uuid.Nil, nil
	}
	if _, err := q.db.Exec(ctx, `SELECT products_refresh_search_vector(id) FROM unnest($1::uuid[]) AS t(id)`, ids); err != nil {
		return uuid.Nil, fmt.Errorf("refresh search vectors: %w", err)
	}
	return ids[len(ids)-1], nil
}

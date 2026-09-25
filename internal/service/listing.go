package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/platform"
	"github.com/khansbikezone/bikezone-api/internal/store"
	"github.com/khansbikezone/bikezone-api/internal/store/gen"
)

// Listing limits.
const (
	defaultPageSize = 24
	maxPageSize     = 100
	maxQueryLen     = 200
)

// compiledQuery is a ProductQuery resolved against the database: slugs turned
// into ids and paths, attribute filters into JSONPath predicates.
type compiledQuery struct {
	base     store.ProductFilter // every constraint except the attribute filters
	paths    []attrPath          // one per attribute filter, in request order
	category *domain.Category    // set when the query names a category
	schema   *categorySchema     // the category's schema, when one is named
	sort     string
	limit    int
}

type attrPath struct {
	key  string
	path string
}

// filter returns the full filter, optionally leaving out one attribute's
// constraint (disjunctive faceting counts a facet without its own filter).
func (c *compiledQuery) filter(exceptKey string) store.ProductFilter {
	f := c.base
	f.AttrPaths = nil
	for _, p := range c.paths {
		if p.key != exceptKey {
			f.AttrPaths = append(f.AttrPaths, p.path)
		}
	}
	return f
}

// compileQuery validates and resolves a listing query. public selects the
// public visibility rule and the public default sort.
func compileQuery(ctx context.Context, q *store.Queries, pq domain.ProductQuery, public bool) (*compiledQuery, error) {
	c := &compiledQuery{base: store.ProductFilter{PublicOnly: public}, limit: platform.ClampLimit(pq.Limit, defaultPageSize, maxPageSize)}
	v := &domain.ValidationError{}

	if !public && pq.Status != "" {
		if !pq.Status.Valid() {
			v.Add("status", "is not a known product status")
		}
		c.base.Status = pq.Status
	}

	// The attributes a filter may reference: the category's bindings, or every
	// attribute when the listing spans categories.
	var attrs map[string]*domain.Attribute
	if pq.CategorySlug != "" {
		cat, err := visibleCategoryBySlug(ctx, q, pq.CategorySlug, public)
		if err != nil {
			return nil, err
		}
		c.category = cat
		c.base.CategoryPath = cat.Path
		if c.schema, err = loadCategorySchema(ctx, q, cat.ID); err != nil {
			return nil, err
		}
		attrs = make(map[string]*domain.Attribute, len(c.schema.bindings))
		for i := range c.schema.bindings {
			attrs[c.schema.bindings[i].Attribute.Key] = &c.schema.bindings[i].Attribute
		}
	} else if len(pq.Attributes) > 0 {
		var err error
		if attrs, err = loadAttributes(ctx, q); err != nil {
			return nil, err
		}
	}

	if pq.BrandSlug != "" {
		b, err := q.GetBrandBySlug(ctx, pq.BrandSlug)
		switch {
		case errors.Is(err, domain.ErrNotFound):
			v.Add("brand", "no brand has the slug %q", pq.BrandSlug)
		case err != nil:
			return nil, fmt.Errorf("load brand: %w", err)
		default:
			c.base.BrandID = &b.ID
		}
	}

	c.base.Text = strings.TrimSpace(pq.Text)
	if utf8.RuneCountInString(c.base.Text) > maxQueryLen {
		v.Add("q", "must be at most %d characters", maxQueryLen)
	}

	for _, af := range pq.Attributes {
		field := "attr." + af.Key
		a := attrs[af.Key]
		if a == nil || !a.IsFilterable {
			v.Add(field, "is not a filterable attribute here")
			continue
		}
		path, err := filterPath(a, af)
		if err != nil {
			v.Add(field, "%s", err)
			continue
		}
		c.paths = append(c.paths, attrPath{key: af.Key, path: path})
	}

	c.sort = pq.Sort
	if c.sort == "" {
		switch {
		case c.base.Text != "":
			c.sort = domain.SortRelevance
		case public:
			c.sort = domain.SortFeatured
		default:
			c.sort = domain.SortUpdated
		}
	}
	return c, v.Err()
}

// visibleCategoryBySlug loads a category; for the public audience it must be
// visible (it and every ancestor active), otherwise it does not exist.
func visibleCategoryBySlug(ctx context.Context, q *store.Queries, slug string, public bool) (*domain.Category, error) {
	row, err := q.GetCategoryBySlug(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("category %q: %w", slug, err)
	}
	if public {
		visible, err := q.CategoryIsVisible(ctx, row.ID)
		if err != nil {
			return nil, fmt.Errorf("check category visibility: %w", err)
		}
		if !visible {
			return nil, fmt.Errorf("category %q: %w", slug, domain.ErrNotFound)
		}
	}
	c := store.Category(gen.GetCategoryByIDRow(row))
	return &c, nil
}

// listProducts runs a compiled query and enriches the page for display.
func listProducts(ctx context.Context, q *store.Queries, c *compiledQuery, cursor string) (domain.ProductPage, error) {
	f := c.filter("")
	items, next, err := q.ListProducts(ctx, f, c.sort, cursor, c.limit)
	if err != nil {
		return domain.ProductPage{}, err
	}
	total, err := q.CountProducts(ctx, f)
	if err != nil {
		return domain.ProductPage{}, err
	}
	if err := enrichSummaries(ctx, q, items); err != nil {
		return domain.ProductPage{}, err
	}
	return domain.ProductPage{Items: items, Total: total, NextCursor: next}, nil
}

// availabilityRank orders stock statuses from most to least available; a
// card shows the best status across the product's variants.
var availabilityRank = map[domain.StockStatus]int{
	domain.StockIn:           0,
	domain.StockLow:          1,
	domain.StockSpecialOrder: 2,
	domain.StockUnknown:      3,
	domain.StockOut:          4,
}

// enrichSummaries fills the card aggregates (variant count, availability,
// public "from" price, cover image) for a page of products in a fixed number
// of batch queries, however many products the page holds.
func enrichSummaries(ctx context.Context, q *store.Queries, items []domain.ProductSummary) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(items))
	index := make(map[uuid.UUID]*domain.ProductSummary, len(items))
	for i := range items {
		ids[i] = items[i].Product.ID
		index[ids[i]] = &items[i]
		items[i].Availability = domain.StockUnknown
	}

	stock, err := q.ListVariantStockByProducts(ctx, ids)
	if err != nil {
		return fmt.Errorf("load variant stock: %w", err)
	}
	seen := map[uuid.UUID]bool{}
	for _, r := range stock {
		s := index[r.ProductID]
		st := domain.StockStatus(r.StockStatus)
		if !seen[r.ProductID] || availabilityRank[st] < availabilityRank[s.Availability] {
			s.Availability = st
		}
		seen[r.ProductID] = true
		s.VariantCount++
	}

	prices, err := q.ListCurrentRetailPricesByProducts(ctx, ids) // public-safe query
	if err != nil {
		return fmt.Errorf("load card prices: %w", err)
	}
	for _, r := range prices {
		s := index[r.ProductID]
		if s.FromPrice == nil || r.AmountMinor < s.FromPrice.AmountMinor() {
			m := platform.NewMoney(r.AmountMinor, r.Currency)
			s.FromPrice = &m
		}
	}

	covers, err := q.ListCoverMediaByProducts(ctx, ids)
	if err != nil {
		return fmt.Errorf("load cover images: %w", err)
	}
	assetIDs := make([]uuid.UUID, len(covers))
	for i, r := range covers {
		assetIDs[i] = r.AssetID
	}
	assets, err := loadAssets(ctx, q, assetIDs)
	if err != nil {
		return err
	}
	for _, r := range covers {
		pm := store.ProductMedia(r)
		pm.Asset = assets[r.AssetID]
		index[r.ProductID].Cover = &pm
	}
	return nil
}

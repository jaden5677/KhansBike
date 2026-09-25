package service

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/store"
	"github.com/khansbikezone/bikezone-api/internal/store/gen"
)

// Catalog serves the public, read-only catalogue. Everything it returns obeys
// the public visibility rules: active products in visible categories, the
// retail price only when the product opts in, processed images only.
type Catalog struct {
	store *store.Store
}

// NewCatalog builds the public catalogue service.
func NewCatalog(st *store.Store) *Catalog { return &Catalog{store: st} }

// fitmentAttribute is the shared attribute that answers "what fits my wheel":
// one global wheel_size, bound to every wheel-bearing category.
const fitmentAttribute = "wheel_size"

// Search result shaping.
const (
	searchPerGroup  = 4
	fitmentPerGroup = 100
	suggestLimit    = 8
	minSuggestLen   = 2
)

// CategoryTree is the visible category tree. Each ProductCount includes the
// products of the category's descendants. Images holds the hero images
// referenced by the tree, by asset id.
type CategoryTree struct {
	Roots  []domain.Category
	Images map[uuid.UUID]*domain.MediaAsset
}

// CategoryDetail is one visible category with its children and the trail of
// ancestors leading to it (root first).
type CategoryDetail struct {
	Category    domain.Category
	Breadcrumbs []domain.Category
	Images      map[uuid.UUID]*domain.MediaAsset
}

// catNode is a mutable tree node used while assembling the tree.
type catNode struct {
	cat      domain.Category
	parent   *catNode
	children []*catNode
}

// visibleTree builds the tree of active categories. A category whose parent
// is inactive is dropped along with its subtree: hiding a category hides
// everything under it.
func (c *Catalog) visibleTree(ctx context.Context) ([]*catNode, map[uuid.UUID]*catNode, error) {
	rows, err := c.store.ListActiveCategoriesWithCounts(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("load categories: %w", err)
	}
	nodes := make(map[uuid.UUID]*catNode, len(rows))
	var roots []*catNode
	for _, r := range rows { // ordered by path: parents precede children
		cat := store.Category(gen.GetCategoryByIDRow{
			ID: r.ID, ParentID: r.ParentID, Name: r.Name, Slug: r.Slug, Path: r.Path, Position: r.Position,
			Description: r.Description, HeroAssetID: r.HeroAssetID, IsActive: r.IsActive,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
		cat.ProductCount = int(r.ProductCount)
		n := &catNode{cat: cat}
		if r.ParentID == nil {
			roots = append(roots, n)
		} else if parent, ok := nodes[*r.ParentID]; ok {
			n.parent = parent
			parent.children = append(parent.children, n)
		} else {
			continue // parent hidden
		}
		nodes[r.ID] = n
	}
	for _, root := range roots {
		rollUp(root)
	}
	sortNodes(roots)
	return roots, nodes, nil
}

func rollUp(n *catNode) int {
	for _, ch := range n.children {
		n.cat.ProductCount += rollUp(ch)
	}
	return n.cat.ProductCount
}

func sortNodes(ns []*catNode) {
	slices.SortFunc(ns, func(a, b *catNode) int {
		return cmp.Or(cmp.Compare(a.cat.Position, b.cat.Position), cmp.Compare(a.cat.Name, b.cat.Name))
	})
	for _, n := range ns {
		sortNodes(n.children)
	}
}

// materialise converts a node into a domain.Category with Children filled.
func materialise(n *catNode) domain.Category {
	c := n.cat
	c.Children = make([]domain.Category, len(n.children))
	for i, ch := range n.children {
		c.Children[i] = materialise(ch)
	}
	return c
}

// CategoryTree returns the public category tree.
func (c *Catalog) CategoryTree(ctx context.Context) (*CategoryTree, error) {
	roots, nodes, err := c.visibleTree(ctx)
	if err != nil {
		return nil, err
	}
	t := &CategoryTree{Roots: make([]domain.Category, len(roots))}
	for i, r := range roots {
		t.Roots[i] = materialise(r)
	}
	if t.Images, err = c.heroImages(ctx, nodes); err != nil {
		return nil, err
	}
	return t, nil
}

// Category returns one visible category by slug.
func (c *Catalog) Category(ctx context.Context, slug string) (*CategoryDetail, error) {
	_, nodes, err := c.visibleTree(ctx)
	if err != nil {
		return nil, err
	}
	var node *catNode
	for _, n := range nodes {
		if n.cat.Slug == slug {
			node = n
			break
		}
	}
	if node == nil {
		return nil, fmt.Errorf("category %q: %w", slug, domain.ErrNotFound)
	}
	d := &CategoryDetail{Category: materialise(node)}
	for p := node.parent; p != nil; p = p.parent {
		d.Breadcrumbs = append([]domain.Category{p.cat}, d.Breadcrumbs...)
	}
	sub := map[uuid.UUID]*catNode{node.cat.ID: node}
	for _, ch := range node.children {
		sub[ch.cat.ID] = ch
	}
	if d.Images, err = c.heroImages(ctx, sub); err != nil {
		return nil, err
	}
	return d, nil
}

// heroImages loads the processed hero images of the given categories.
func (c *Catalog) heroImages(ctx context.Context, nodes map[uuid.UUID]*catNode) (map[uuid.UUID]*domain.MediaAsset, error) {
	var ids []uuid.UUID
	for _, n := range nodes {
		if n.cat.HeroAssetID != nil {
			ids = append(ids, *n.cat.HeroAssetID)
		}
	}
	return readyAssets(ctx, c.store.Queries, ids)
}

// readyAssets loads assets and drops any not yet processed.
func readyAssets(ctx context.Context, q *store.Queries, ids []uuid.UUID) (map[uuid.UUID]*domain.MediaAsset, error) {
	assets, err := loadAssets(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	for id, a := range assets {
		if a.Status != domain.AssetReady {
			delete(assets, id)
		}
	}
	return assets, nil
}

// ListProducts returns one page of a public product listing.
func (c *Catalog) ListProducts(ctx context.Context, pq domain.ProductQuery) (domain.ProductPage, error) {
	cq, err := compileQuery(ctx, c.store.Queries, pq, true)
	if err != nil {
		return domain.ProductPage{}, err
	}
	return listProducts(ctx, c.store.Queries, cq, pq.Cursor)
}

// Facets returns the filter options for a category listing under the current
// filters. Counts are disjunctive: each facet is counted with every filter
// except its own, so selecting "Black" still shows how many products come in
// "Red" instead of zeroing every other colour.
func (c *Catalog) Facets(ctx context.Context, pq domain.ProductQuery) ([]domain.Facet, error) {
	if pq.CategorySlug == "" {
		return nil, domain.Invalid("category", "facets are computed within a category")
	}
	cq, err := compileQuery(ctx, c.store.Queries, pq, true)
	if err != nil {
		return nil, err
	}
	own := map[string]bool{}
	for _, p := range cq.paths {
		own[p.key] = true
	}

	var enumKeys, numKeys []string
	var facetable []*domain.CategoryAttribute
	for i := range cq.schema.bindings {
		b := &cq.schema.bindings[i]
		if !b.Attribute.IsFilterable || b.Attribute.DataType == domain.DataTypeText {
			continue
		}
		facetable = append(facetable, b)
		if own[b.Attribute.Key] {
			continue // counted separately, below
		}
		if isNumeric(b.Attribute.DataType) {
			numKeys = append(numKeys, b.Attribute.Key)
		} else {
			enumKeys = append(enumKeys, b.Attribute.Key)
		}
	}
	counts, err := c.store.FacetCounts(ctx, cq.filter(""), enumKeys)
	if err != nil {
		return nil, err
	}
	ranges, err := c.store.FacetRanges(ctx, cq.filter(""), numKeys)
	if err != nil {
		return nil, err
	}
	for key := range own {
		b := cq.schema.byKey[key]
		if !slices.Contains(facetable, b) {
			continue
		}
		keys := []string{key}
		if isNumeric(b.Attribute.DataType) {
			r, err := c.store.FacetRanges(ctx, cq.filter(key), keys)
			if err != nil {
				return nil, err
			}
			ranges[key] = r[key]
		} else {
			cnt, err := c.store.FacetCounts(ctx, cq.filter(key), keys)
			if err != nil {
				return nil, err
			}
			counts[key] = cnt[key]
		}
	}

	facets := make([]domain.Facet, 0, len(facetable))
	for _, b := range facetable {
		a := &b.Attribute
		f := domain.Facet{Key: a.Key, Label: b.EffectiveLabel(), DataType: a.DataType}
		switch {
		case isNumeric(a.DataType):
			r, ok := ranges[a.Key]
			if !ok {
				continue // no product in the result set has a value
			}
			f.NumMin, f.NumMax = &r.Min, &r.Max
		case a.DataType == domain.DataTypeBoolean:
			for _, v := range []struct{ value, label string }{{"true", "Yes"}, {"false", "No"}} {
				if n := counts[a.Key][v.value]; n > 0 {
					f.Values = append(f.Values, domain.FacetValue{Value: v.value, Label: v.label, Count: n})
				}
			}
		default:
			for _, o := range a.Options {
				if n := counts[a.Key][o.Value]; n > 0 {
					f.Values = append(f.Values, domain.FacetValue{Value: o.Value, Label: o.Label, Count: n})
				}
			}
		}
		if f.NumMin != nil || len(f.Values) > 0 {
			facets = append(facets, f)
		}
	}
	return facets, nil
}

func isNumeric(dt domain.DataType) bool {
	return dt == domain.DataTypeNumber || dt == domain.DataTypeNumberRange
}

// Product returns a publicly visible product by slug. Anything not visible
// (draft, discontinued, hidden category) is reported as not found.
func (c *Catalog) Product(ctx context.Context, slug string) (*ProductView, error) {
	row, err := c.store.GetProductBySlug(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("product %q: %w", slug, err)
	}
	p := store.Product(gen.GetProductByIDRow(row))
	if !p.Status.IsPubliclyVisible() {
		return nil, fmt.Errorf("product %q: %w", slug, domain.ErrNotFound)
	}
	visible, err := c.store.CategoryIsVisible(ctx, p.CategoryID)
	if err != nil {
		return nil, fmt.Errorf("check category visibility: %w", err)
	}
	if !visible {
		return nil, fmt.Errorf("product %q: %w", slug, domain.ErrNotFound)
	}
	return loadProductView(ctx, c.store.Queries, p, publicAudience)
}

// Search runs a free-text search across every category and returns the best
// few matches per category, best categories first.
func (c *Catalog) Search(ctx context.Context, text string) ([]domain.SearchGroup, error) {
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > maxQueryLen {
		return nil, domain.Invalid("q", "must be 1 to %d characters", maxQueryLen)
	}
	return c.grouped(ctx, store.ProductFilter{PublicOnly: true, Text: text}, searchPerGroup)
}

// Suggest is the typeahead: a handful of product names for a partial query.
func (c *Catalog) Suggest(ctx context.Context, text string) ([]domain.Suggestion, error) {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) < minSuggestLen {
		return []domain.Suggestion{}, nil
	}
	if utf8.RuneCountInString(text) > maxQueryLen {
		return nil, domain.Invalid("q", "must be at most %d characters", maxQueryLen)
	}
	return c.store.SuggestProducts(ctx, text, suggestLimit)
}

// Fitment returns every visible product that fits a wheel size, across all
// categories (tubes, tyres, rims, forks, frames, bikes...), grouped by
// category in tree order.
func (c *Catalog) Fitment(ctx context.Context, wheelSize string) ([]domain.SearchGroup, error) {
	row, err := c.store.GetAttributeByKey(ctx, fitmentAttribute)
	if err != nil {
		return nil, fmt.Errorf("attribute %q: %w", fitmentAttribute, err)
	}
	a := store.Attribute(row)
	opts, err := loadOptions(ctx, c.store.Queries, []uuid.UUID{a.ID})
	if err != nil {
		return nil, err
	}
	a.Options = opts[a.ID]
	path, err := filterPath(&a, domain.AttributeFilter{Key: a.Key, Values: []string{wheelSize}})
	if err != nil {
		return nil, fmt.Errorf("wheel size %q: %w", wheelSize, domain.ErrNotFound)
	}
	return c.grouped(ctx, store.ProductFilter{PublicOnly: true, AttrPaths: []string{path}}, fitmentPerGroup)
}

func (c *Catalog) grouped(ctx context.Context, f store.ProductFilter, perGroup int) ([]domain.SearchGroup, error) {
	groups, err := c.store.ListProductsGrouped(ctx, f, perGroup)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		if err := enrichSummaries(ctx, c.store.Queries, groups[i].Products); err != nil {
			return nil, err
		}
	}
	return groups, nil
}

// BrandList is every brand with the processed logos they reference.
type BrandList struct {
	Brands []domain.Brand
	Images map[uuid.UUID]*domain.MediaAsset
}

// Brands lists all brands (brands are public; suppliers never are).
func (c *Catalog) Brands(ctx context.Context) (*BrandList, error) {
	rows, err := c.store.ListBrands(ctx)
	if err != nil {
		return nil, fmt.Errorf("list brands: %w", err)
	}
	out := &BrandList{Brands: make([]domain.Brand, len(rows))}
	var logoIDs []uuid.UUID
	for i, r := range rows {
		out.Brands[i] = store.Brand(r)
		if r.LogoAssetID != nil {
			logoIDs = append(logoIDs, *r.LogoAssetID)
		}
	}
	if out.Images, err = readyAssets(ctx, c.store.Queries, logoIDs); err != nil {
		return nil, err
	}
	return out, nil
}

// reindexBatch bounds how many products one statement refreshes, keeping row
// locks short while a full rebuild runs alongside admin edits.
const reindexBatch = 200

// ReindexSearch is the reindex_search job handler. It first discards every
// other queued reindex job (this run reads the current state, so it covers
// all of them), then rebuilds every product's search vector in batches.
func (c *Catalog) ReindexSearch(ctx context.Context, _ []byte) error {
	if _, err := c.store.DiscardQueuedJobsByKind(ctx, JobReindexSearch); err != nil {
		return fmt.Errorf("coalesce reindex jobs: %w", err)
	}
	after := uuid.Nil
	for {
		last, err := c.store.RefreshSearchVectors(ctx, after, reindexBatch)
		if err != nil {
			return err
		}
		if last == uuid.Nil {
			return nil
		}
		after = last
	}
}

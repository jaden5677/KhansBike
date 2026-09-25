package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/store"
	"github.com/khansbikezone/bikezone-api/internal/store/gen"
)

// ProductView is a product assembled for display: Product has its Category,
// Brand, Variants (with Prices and Attributes) and Media (with Asset and
// Renditions) populated, and its attribute values are resolved against the
// category schema for rendering.
type ProductView struct {
	Product           domain.Product
	Attributes        []AttributeView               // product-level, in schema order
	VariantAttributes map[uuid.UUID][]AttributeView // by variant id
}

// audience selects what a view may contain. The difference is a security
// boundary, not a formatting choice: only adminAudience ever loads non-retail
// price tiers, and publicAudience omits unprocessed images.
type audience int

const (
	publicAudience audience = iota
	adminAudience
)

// loadProductView assembles p (a bare product row) for the given audience.
func loadProductView(ctx context.Context, q *store.Queries, p domain.Product, aud audience) (*ProductView, error) {
	cat, err := q.GetCategoryByID(ctx, p.CategoryID)
	if err != nil {
		return nil, fmt.Errorf("load category: %w", err)
	}
	c := store.Category(cat)
	p.Category = &c
	if p.BrandID != nil {
		b, err := q.GetBrandByID(ctx, *p.BrandID)
		if err != nil {
			return nil, fmt.Errorf("load brand: %w", err)
		}
		brand := store.Brand(b)
		p.Brand = &brand
	}

	variantRows, err := q.ListVariantsByProduct(ctx, p.ID)
	if err != nil {
		return nil, fmt.Errorf("load variants: %w", err)
	}
	prices, err := loadPrices(ctx, q, p, aud)
	if err != nil {
		return nil, err
	}
	p.Variants = make([]domain.Variant, len(variantRows))
	for i, r := range variantRows {
		v := store.Variant(r)
		v.Prices = prices[v.ID]
		p.Variants[i] = v
	}

	schema, err := loadCategorySchema(ctx, q, p.CategoryID)
	if err != nil {
		return nil, err
	}
	valueRows, err := q.ListAttributeValuesByProduct(ctx, p.ID)
	if err != nil {
		return nil, fmt.Errorf("load attribute values: %w", err)
	}
	raw := make([]domain.AttributeValue, len(valueRows))
	for i, r := range valueRows {
		raw[i] = store.AttributeValue(r)
	}
	view := &ProductView{VariantAttributes: map[uuid.UUID][]AttributeView{}}
	perVariant := map[uuid.UUID][]domain.AttributeValue{}
	for _, v := range mergeValues(raw) {
		if v.VariantID == nil {
			p.Attributes = append(p.Attributes, v)
		} else {
			perVariant[*v.VariantID] = append(perVariant[*v.VariantID], v)
		}
	}
	view.Attributes = schema.views(p.Attributes)
	for i := range p.Variants {
		v := &p.Variants[i]
		v.Attributes = perVariant[v.ID]
		view.VariantAttributes[v.ID] = schema.views(v.Attributes)
	}

	if p.Media, err = loadProductMedia(ctx, q, p.ID, aud); err != nil {
		return nil, err
	}
	view.Product = p
	return view, nil
}

// loadPrices returns current prices by variant id. The public branch uses the
// public-safe query (retail tier only, only when the product opts in) and then
// filters again with domain.FilterPublicPrices, so a mistake in either layer
// alone cannot leak a cost or wholesale price.
func loadPrices(ctx context.Context, q *store.Queries, p domain.Product, aud audience) (map[uuid.UUID][]domain.Price, error) {
	var rows []gen.Price
	var err error
	if aud == adminAudience {
		rows, err = q.ListCurrentPricesByProduct(ctx, p.ID)
	} else {
		rows, err = q.ListCurrentRetailPricesByProduct(ctx, p.ID)
	}
	if err != nil {
		return nil, fmt.Errorf("load prices: %w", err)
	}
	out := map[uuid.UUID][]domain.Price{}
	for _, r := range rows {
		out[r.VariantID] = append(out[r.VariantID], store.Price(r))
	}
	if aud == publicAudience {
		for id, ps := range out {
			out[id] = domain.FilterPublicPrices(ps, p.RetailPriceIsPublic)
		}
	}
	return out, nil
}

// loadProductMedia returns a product's images with their assets and
// renditions. The public audience only sees processed (ready) images.
func loadProductMedia(ctx context.Context, q *store.Queries, productID uuid.UUID, aud audience) ([]domain.ProductMedia, error) {
	rows, err := q.ListProductMedia(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("load product media: %w", err)
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.AssetID
	}
	assets, err := loadAssets(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ProductMedia, 0, len(rows))
	for _, r := range rows {
		a, ok := assets[r.AssetID]
		if !ok || (aud == publicAudience && a.Status != domain.AssetReady) {
			continue
		}
		pm := store.ProductMedia(r)
		pm.Asset = a
		out = append(out, pm)
	}
	return out, nil
}

// loadAssets loads assets with their renditions, keyed by id.
func loadAssets(ctx context.Context, q *store.Queries, ids []uuid.UUID) (map[uuid.UUID]*domain.MediaAsset, error) {
	out := map[uuid.UUID]*domain.MediaAsset{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.ListAssetsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("load media assets: %w", err)
	}
	for _, r := range rows {
		a := store.MediaAsset(r)
		out[a.ID] = &a
	}
	rends, err := q.ListRenditionsByAssets(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("load renditions: %w", err)
	}
	for _, r := range rends {
		if a, ok := out[r.AssetID]; ok {
			a.Renditions = append(a.Renditions, store.Rendition(r))
		}
	}
	return out, nil
}

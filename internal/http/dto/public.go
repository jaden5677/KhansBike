// Package dto defines the JSON the API speaks and the mapping from domain
// types to it. Public and admin shapes are separate types on purpose: a
// public type has no field that could hold a supplier, a cost or a
// wholesale price, so leaking one is a compile error rather than a code
// review catch.
package dto

import (
	"time"

	"github.com/google/uuid"

	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/platform"
	"github.com/khansbikezone/bikezone-api/internal/service"
)

// URLFunc turns a media storage key into the URL a client fetches it from.
type URLFunc func(key string) string

// Page is one page of a list. Total is present when the list knows it;
// NextCursor is absent on the last page.
type Page[T any] struct {
	Items      []T    `json:"items"`
	Total      *int   `json:"total,omitempty"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// BrandRef and CategoryRef are compact references embedded in other objects.
type BrandRef struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Slug string    `json:"slug"`
}

type CategoryRef struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Slug string    `json:"slug"`
}

func brandRef(b *domain.Brand) *BrandRef {
	if b == nil {
		return nil
	}
	return &BrandRef{ID: b.ID, Name: b.Name, Slug: b.Slug}
}

func categoryRef(c *domain.Category) CategoryRef {
	if c == nil {
		return CategoryRef{}
	}
	return CategoryRef{ID: c.ID, Name: c.Name, Slug: c.Slug}
}

// ImageSource is one rendition of an image; clients build a srcset from them.
type ImageSource struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Format string `json:"format"`
}

// Image is a processed picture with its placeholders and renditions.
type Image struct {
	Width       int           `json:"width"`
	Height      int           `json:"height"`
	Blurhash    string        `json:"blurhash,omitempty"`
	DominantHex string        `json:"dominantHex,omitempty"`
	AltText     *string       `json:"altText,omitempty"`
	Sources     []ImageSource `json:"sources"`
}

// NewImage maps an asset, or returns nil when there is none.
func NewImage(a *domain.MediaAsset, alt *string, urls URLFunc) *Image {
	if a == nil {
		return nil
	}
	img := &Image{Width: a.Width, Height: a.Height, Blurhash: a.Blurhash, DominantHex: a.DominantHex, AltText: alt, Sources: []ImageSource{}}
	for _, r := range a.Renditions {
		img.Sources = append(img.Sources, ImageSource{URL: urls(r.StorageKey), Width: r.Width, Height: r.Height, Format: r.Format})
	}
	return img
}

// ProductCard is a product in a listing.
type ProductCard struct {
	ID           uuid.UUID          `json:"id"`
	Slug         string             `json:"slug"`
	Name         string             `json:"name"`
	Summary      *string            `json:"summary,omitempty"`
	Brand        *BrandRef          `json:"brand,omitempty"`
	Category     CategoryRef        `json:"category"`
	IsFeatured   bool               `json:"isFeatured"`
	FromPrice    *platform.Money    `json:"fromPrice"` // null when the price is not public
	Availability domain.StockStatus `json:"availability"`
	VariantCount int                `json:"variantCount"`
	Image        *Image             `json:"image"`
}

// NewProductCard maps a listing entry.
func NewProductCard(s domain.ProductSummary, urls URLFunc) ProductCard {
	p := s.Product
	c := ProductCard{
		ID: p.ID, Slug: p.Slug, Name: p.Name, Summary: p.Summary, Brand: brandRef(p.Brand),
		Category: categoryRef(p.Category), IsFeatured: p.IsFeatured, FromPrice: s.FromPrice,
		Availability: s.Availability, VariantCount: s.VariantCount,
	}
	if s.Cover != nil {
		c.Image = NewImage(s.Cover.Asset, s.Cover.AltText, urls)
	}
	return c
}

// NewProductPage maps a listing page.
func NewProductPage(pg domain.ProductPage, urls URLFunc) Page[ProductCard] {
	out := Page[ProductCard]{Items: make([]ProductCard, len(pg.Items)), Total: &pg.Total, NextCursor: pg.NextCursor}
	for i, s := range pg.Items {
		out.Items[i] = NewProductCard(s, urls)
	}
	return out
}

// AttributeValue is one attribute of a product or variant, ready to show.
type AttributeValue struct {
	Key       string          `json:"key"`
	Label     string          `json:"label"`
	DataType  domain.DataType `json:"dataType"`
	Unit      *string         `json:"unit,omitempty"`
	Value     any             `json:"value"`
	Display   string          `json:"display"`
	SwatchHex *string         `json:"swatchHex,omitempty"`
}

func attributeValues(views []service.AttributeView) []AttributeValue {
	out := make([]AttributeValue, len(views))
	for i, v := range views {
		out[i] = AttributeValue{Key: v.Key, Label: v.Label, DataType: v.DataType, Unit: v.Unit, Value: v.Value, Display: v.Display, SwatchHex: v.SwatchHex}
	}
	return out
}

// Variant is a public variant: no supplier fields, and only the retail price,
// only when the product publishes it.
type Variant struct {
	ID          uuid.UUID          `json:"id"`
	SKU         string             `json:"sku"`
	ModelNo     *string            `json:"modelNo,omitempty"`
	NameSuffix  *string            `json:"nameSuffix,omitempty"`
	StockStatus domain.StockStatus `json:"stockStatus"`
	IsDefault   bool               `json:"isDefault"`
	Attributes  []AttributeValue   `json:"attributes"`
	Price       *platform.Money    `json:"price"`
}

// ProductImage is an image attached to a product (optionally to one variant).
type ProductImage struct {
	ID        uuid.UUID        `json:"id"`
	Role      domain.MediaRole `json:"role"`
	VariantID *uuid.UUID       `json:"variantId,omitempty"`
	Image
}

// ProductDetail is the public product page.
type ProductDetail struct {
	ID          uuid.UUID        `json:"id"`
	Slug        string           `json:"slug"`
	Name        string           `json:"name"`
	Summary     *string          `json:"summary,omitempty"`
	Description *string          `json:"description,omitempty"`
	Brand       *BrandRef        `json:"brand,omitempty"`
	Category    CategoryRef      `json:"category"`
	IsFeatured  bool             `json:"isFeatured"`
	PublishedAt *time.Time       `json:"publishedAt,omitempty"`
	Attributes  []AttributeValue `json:"attributes"`
	Variants    []Variant        `json:"variants"`
	Images      []ProductImage   `json:"images"`
}

// NewProductDetail maps a public product view. The view's prices were loaded
// through the public-safe path; the retail tier is picked explicitly here too.
func NewProductDetail(v *service.ProductView, urls URLFunc) ProductDetail {
	p := v.Product
	d := ProductDetail{
		ID: p.ID, Slug: p.Slug, Name: p.Name, Summary: p.Summary, Description: p.Description,
		Brand: brandRef(p.Brand), Category: categoryRef(p.Category), IsFeatured: p.IsFeatured,
		PublishedAt: p.PublishedAt, Attributes: attributeValues(v.Attributes),
		Variants: make([]Variant, len(p.Variants)), Images: productImages(p.Media, urls),
	}
	for i, vr := range p.Variants {
		out := Variant{
			ID: vr.ID, SKU: vr.SKU, ModelNo: vr.ModelNo, NameSuffix: vr.NameSuffix, StockStatus: vr.StockStatus,
			IsDefault: vr.IsDefault, Attributes: attributeValues(v.VariantAttributes[vr.ID]),
		}
		for _, pr := range domain.FilterPublicPrices(vr.Prices, p.RetailPriceIsPublic) {
			m := pr.Amount
			out.Price = &m
		}
		d.Variants[i] = out
	}
	return d
}

func productImages(media []domain.ProductMedia, urls URLFunc) []ProductImage {
	out := make([]ProductImage, 0, len(media))
	for _, m := range media {
		if img := NewImage(m.Asset, m.AltText, urls); img != nil {
			out = append(out, ProductImage{ID: m.ID, Role: m.Role, VariantID: m.VariantID, Image: *img})
		}
	}
	return out
}

// CategoryNode is a category in the public tree. ProductCount includes the
// products of all descendants.
type CategoryNode struct {
	ID           uuid.UUID      `json:"id"`
	Name         string         `json:"name"`
	Slug         string         `json:"slug"`
	Description  *string        `json:"description,omitempty"`
	ProductCount int            `json:"productCount"`
	Image        *Image         `json:"image"`
	Children     []CategoryNode `json:"children"`
}

// NewCategoryNodes maps a category tree.
func NewCategoryNodes(cs []domain.Category, images map[uuid.UUID]*domain.MediaAsset, urls URLFunc) []CategoryNode {
	out := make([]CategoryNode, len(cs))
	for i, c := range cs {
		out[i] = CategoryNode{
			ID: c.ID, Name: c.Name, Slug: c.Slug, Description: c.Description, ProductCount: c.ProductCount,
			Children: NewCategoryNodes(c.Children, images, urls),
		}
		if c.HeroAssetID != nil {
			out[i].Image = NewImage(images[*c.HeroAssetID], nil, urls)
		}
	}
	return out
}

// CategoryPage is one category with its breadcrumb trail (root first).
type CategoryPage struct {
	CategoryNode
	Breadcrumbs []CategoryRef `json:"breadcrumbs"`
}

// NewCategoryPage maps a category detail.
func NewCategoryPage(d *service.CategoryDetail, urls URLFunc) CategoryPage {
	node := NewCategoryNodes([]domain.Category{d.Category}, d.Images, urls)[0]
	page := CategoryPage{CategoryNode: node, Breadcrumbs: make([]CategoryRef, len(d.Breadcrumbs))}
	for i := range d.Breadcrumbs {
		page.Breadcrumbs[i] = categoryRef(&d.Breadcrumbs[i])
	}
	return page
}

// SearchGroup is the matches within one category.
type SearchGroup struct {
	Category CategoryRef   `json:"category"`
	Total    int           `json:"total"`
	Products []ProductCard `json:"products"`
}

// NewSearchGroups maps grouped results.
func NewSearchGroups(groups []domain.SearchGroup, urls URLFunc) []SearchGroup {
	out := make([]SearchGroup, len(groups))
	for i, g := range groups {
		sg := SearchGroup{
			Category: CategoryRef{ID: g.CategoryID, Name: g.CategoryName, Slug: g.CategorySlug},
			Total:    g.Total, Products: make([]ProductCard, len(g.Products)),
		}
		for j, p := range g.Products {
			sg.Products[j] = NewProductCard(p, urls)
		}
		out[i] = sg
	}
	return out
}

// PublicBrand is a brand in the public brand list.
type PublicBrand struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Slug string    `json:"slug"`
	Logo *Image    `json:"logo"`
}

// NewPublicBrands maps the public brand list.
func NewPublicBrands(bl *service.BrandList, urls URLFunc) []PublicBrand {
	out := make([]PublicBrand, len(bl.Brands))
	for i, b := range bl.Brands {
		out[i] = PublicBrand{ID: b.ID, Name: b.Name, Slug: b.Slug}
		if b.LogoAssetID != nil {
			out[i].Logo = NewImage(bl.Images[*b.LogoAssetID], nil, urls)
		}
	}
	return out
}

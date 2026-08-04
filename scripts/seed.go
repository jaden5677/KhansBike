//go:build ignore

// Command seed populates a fresh database with the real-world taxonomy and a
// representative slice of catalogue data: the 33 workbook categories plus Bikes,
// the shared cross-category attributes (wheel_size, colour) with their canonical
// options, a handful of category-specific attributes, and ~30 products drawn
// from the actual workbook rows (pedals with thread/colour variants, tubes and
// tyres with wheel size and width, grips with colour variants, and so on).
//
// It is written against raw pgx rather than the generated queries so it stays a
// standalone, dependency-light tool. It builds the JSONB attribute projection in
// the same transaction as the EAV rows, exactly as the service layer will.
//
// Run with:  go run scripts/seed.go   (DATABASE_URL must be set)
//
// The //go:build ignore tag keeps it out of the normal package build; it is a
// script, not part of the server.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khansbikezone/bikezone-api/internal/platform"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("seed: %v", err)
	}
}

func run() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	s := &seeder{tx: tx, ctx: ctx, options: map[string]map[string]uuid.UUID{}}

	if err := s.seedCategories(); err != nil {
		return err
	}
	if err := s.seedAttributes(); err != nil {
		return err
	}
	if err := s.seedBrands(); err != nil {
		return err
	}
	if err := s.bindCategoryAttributes(); err != nil {
		return err
	}
	if err := s.seedProducts(); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	log.Printf("seed complete: %d categories, %d attributes, %d products",
		len(s.categoryID), s.attrCount, s.productCount)
	return nil
}

type seeder struct {
	ctx context.Context
	tx  pgx.Tx

	categoryID   map[string]uuid.UUID          // name -> id
	attrID       map[string]uuid.UUID          // key -> id
	attrType     map[string]string             // key -> data_type
	options      map[string]map[string]uuid.UUID // attr key -> option value -> id
	brandID      map[string]uuid.UUID          // name -> id
	attrCount    int
	productCount int
}

func newID() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.New()
	}
	return id
}

// ltreeLabel converts a slug to a valid ltree label (letters, digits, underscore
// only), since hyphens are not permitted in ltree.
func ltreeLabel(slug string) string { return strings.ReplaceAll(slug, "-", "_") }

// The 33 workbook categories, in workbook order, plus Bikes.
var categoryNames = []string{
	"Tubes", "Tyres", "Rims", "Tools", "Brakes and Related", "Hoppers/Axle Pegs",
	"Kickstands", "Shifters & Derailleurs", "Bearings and Related", "Hubs", "Spokes",
	"Bells & Horns", "Grips", "Tools/Grease/Lubes", "Lights", "Bottles & Cages",
	"Reflectors", "Cranksets & Plates", "Handle Bars", "Seat Poles", "Chains", "Pedals",
	"Frames", "Sprockets/Freewheels/Cassettes", "Pumps", "Valve Caps", "Seats",
	"Helmets and Safety Pads", "Forks", "E-Bikes & Parts", "Bike Locks", "Accessories",
	"Tricycles & Scooters",
	// Bikes has no workbook sheet but is a first-class category the owner wants.
	"Bikes",
}

func (s *seeder) seedCategories() error {
	s.categoryID = make(map[string]uuid.UUID, len(categoryNames))
	for i, name := range categoryNames {
		id := newID()
		slug := platform.Slugify(name)
		path := ltreeLabel(slug)
		_, err := s.tx.Exec(s.ctx,
			`INSERT INTO categories (id, parent_id, name, slug, path, position, is_active)
			 VALUES ($1, NULL, $2, $3, $4::ltree, $5, true)`,
			id, name, slug, path, i)
		if err != nil {
			return fmt.Errorf("insert category %q: %w", name, err)
		}
		s.categoryID[name] = id
	}
	return nil
}

// attrDef describes an attribute to create along with its canonical options.
type attrDef struct {
	key        string
	label      string
	dataType   string
	unit       string
	inputType  string
	filterable bool
	searchable bool
	options    []optDef
}

type optDef struct {
	value  string
	label  string
	swatch string
}

var attributeDefs = []attrDef{
	{
		key: "wheel_size", label: "Wheel Size", dataType: "enum", unit: "in", inputType: "select",
		filterable: true, searchable: true,
		options: []optDef{
			{"12", `12"`, ""}, {"12.5", `12.5"`, ""}, {"14", `14"`, ""}, {"16", `16"`, ""},
			{"18", `18"`, ""}, {"20", `20"`, ""}, {"24", `24"`, ""}, {"26", `26"`, ""},
			{"27", `27"`, ""}, {"27.5", `27.5"`, ""}, {"28", `28"`, ""}, {"29", `29"`, ""},
			{"700c", "700c", ""},
		},
	},
	{
		key: "colour", label: "Colour", dataType: "color", inputType: "swatch",
		filterable: true, searchable: false,
		options: []optDef{
			{"black", "Black", "#000000"}, {"silver", "Silver", "#c0c0c0"},
			{"red", "Red", "#d32f2f"}, {"blue", "Blue", "#1976d2"},
			{"orange", "Orange", "#f57c00"}, {"purple", "Purple", "#7b1fa2"},
			{"green", "Green", "#388e3c"}, {"white", "White", "#ffffff"},
			{"black-silver", "Black/Silver", "#4d4d4d"},
		},
	},
	{
		key: "valve_type", label: "Valve Type", dataType: "enum", inputType: "select",
		filterable: true,
		options: []optDef{{"av", "A/V (Schrader)", ""}, {"fv", "F/V (Presta)", ""}},
	},
	{
		key: "valve_length", label: "Valve Length", dataType: "enum", unit: "mm", inputType: "select",
		filterable: true,
		options: []optDef{{"regular", "Regular", ""}, {"48", "48mm", ""}, {"60", "60mm", ""}, {"80", "80mm", ""}},
	},
	{
		key: "thread", label: "Thread", dataType: "enum", inputType: "select", filterable: true,
		options: []optDef{{"half", `1/2"`, ""}, {"nine-sixteenths", `9/16"`, ""}},
	},
	{
		key: "tyre_type", label: "Type", dataType: "enum", inputType: "select", filterable: true, searchable: true,
		options: []optDef{
			{"black", "Black", ""}, {"freestyle", "Freestyle", ""}, {"k-rad", "K-Rad", ""},
			{"motocross", "Motocross", ""}, {"big-bead", "Big Bead", ""}, {"road", "Road", ""},
			{"gravel", "Gravel", ""}, {"tlr-folding", "TLR Folding", ""},
		},
	},
	{
		key: "width", label: "Width", dataType: "number_range", unit: "in", inputType: "range",
		filterable: true,
	},
	{
		key: "packaging", label: "Packaging", dataType: "enum", inputType: "select",
		options: []optDef{{"bag", "Bag", ""}, {"box", "Box", ""}, {"loose", "Loose", ""}},
	},
}

func (s *seeder) seedAttributes() error {
	s.attrID = make(map[string]uuid.UUID)
	s.attrType = make(map[string]string)
	for _, a := range attributeDefs {
		id := newID()
		var unit *string
		if a.unit != "" {
			unit = &a.unit
		}
		_, err := s.tx.Exec(s.ctx,
			`INSERT INTO attributes (id, key, label, data_type, unit, input_type, is_filterable, is_searchable)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			id, a.key, a.label, a.dataType, unit, a.inputType, a.filterable, a.searchable)
		if err != nil {
			return fmt.Errorf("insert attribute %q: %w", a.key, err)
		}
		s.attrID[a.key] = id
		s.attrType[a.key] = a.dataType
		s.attrCount++

		s.options[a.key] = make(map[string]uuid.UUID)
		for i, o := range a.options {
			oid := newID()
			var swatch *string
			if o.swatch != "" {
				swatch = &o.swatch
			}
			_, err := s.tx.Exec(s.ctx,
				`INSERT INTO attribute_options (id, attribute_id, value, label, swatch_hex, position)
				 VALUES ($1,$2,$3,$4,$5,$6)`,
				oid, id, o.value, o.label, swatch, i)
			if err != nil {
				return fmt.Errorf("insert option %s/%s: %w", a.key, o.value, err)
			}
			s.options[a.key][o.value] = oid
		}
	}
	return nil
}

var brandNames = []string{"Kenda", "CST", "Wanda", "Duro", "Generic"}

func (s *seeder) seedBrands() error {
	s.brandID = make(map[string]uuid.UUID)
	for i, name := range brandNames {
		id := newID()
		if _, err := s.tx.Exec(s.ctx,
			`INSERT INTO brands (id, name, slug, position) VALUES ($1,$2,$3,$4)`,
			id, name, platform.Slugify(name), i); err != nil {
			return fmt.Errorf("insert brand %q: %w", name, err)
		}
		s.brandID[name] = id
	}
	return nil
}

// binding attaches an attribute to a category with per-binding flags.
type binding struct {
	attr        string
	required    bool
	variantAxis bool
	position    int
}

// categoryBindings expresses which attributes apply to which categories. The
// wheel_size attribute is deliberately shared across every wheel-bearing
// category so "everything that fits a 20-inch wheel" is one query.
var categoryBindings = map[string][]binding{
	"Tubes":     {{"wheel_size", true, false, 0}, {"width", false, false, 1}, {"valve_type", true, true, 2}, {"valve_length", false, false, 3}, {"packaging", false, false, 4}},
	"Tyres":     {{"wheel_size", true, false, 0}, {"width", false, false, 1}, {"tyre_type", false, false, 2}},
	"Rims":      {{"wheel_size", true, false, 0}},
	"Frames":    {{"wheel_size", true, false, 0}},
	"Forks":     {{"wheel_size", true, false, 0}},
	"Bikes":     {{"wheel_size", true, false, 0}, {"colour", false, true, 1}},
	"Pedals":    {{"thread", true, true, 0}, {"colour", false, true, 1}},
	"Grips":     {{"colour", false, true, 0}},
	"Valve Caps": {{"colour", false, true, 0}},
	"Bike Locks": {{"colour", false, true, 0}},
	"Accessories": {{"colour", false, true, 0}},
}

func (s *seeder) bindCategoryAttributes() error {
	for catName, binds := range categoryBindings {
		catID, ok := s.categoryID[catName]
		if !ok {
			return fmt.Errorf("binding references unknown category %q", catName)
		}
		for _, b := range binds {
			attrID, ok := s.attrID[b.attr]
			if !ok {
				return fmt.Errorf("binding references unknown attribute %q", b.attr)
			}
			if _, err := s.tx.Exec(s.ctx,
				`INSERT INTO category_attributes (category_id, attribute_id, position, is_required, is_variant_axis)
				 VALUES ($1,$2,$3,$4,$5)`,
				catID, attrID, b.position, b.required, b.variantAxis); err != nil {
				return fmt.Errorf("bind %s/%s: %w", catName, b.attr, err)
			}
		}
	}
	return nil
}

// ---- product data ----------------------------------------------------------

type variantSpec struct {
	skuSuffix      string
	supplierItemNo string
	colour         string // option value, or "" if not a colour variant
	thread         string // option value, or ""
	stock          string
	retailTTD      int64 // minor units
	wholesaleTTD   int64
	landedTTD      int64
	costUSD        int64
}

type productSpec struct {
	category     string
	brand        string
	name         string
	summary      string
	retailPublic bool
	featured     bool
	wheelSize    string  // option value or ""
	tyreType     string  // option value or ""
	valveType    string  // option value or ""
	widthLow     float64 // 0 => none
	widthHigh    float64
	variants     []variantSpec
}

var productSpecs = []productSpec{
	{
		category: "Pedals", brand: "Generic", name: "Pedal Alloy Freestyle",
		summary: "Regular broad alloy freestyle pedal", retailPublic: true, featured: true,
		variants: []variantSpec{
			{"half-black", "PED-AF-12B", "black", "half", "in_stock", 8500, 6000, 4000, 550},
			{"nine-silver", "PED-AF-916S", "silver", "nine-sixteenths", "in_stock", 8500, 6000, 4000, 550},
			{"nine-black", "PED-AF-916B", "black", "nine-sixteenths", "in_stock", 8500, 6000, 4000, 550},
			{"nine-red", "PED-AF-916R", "red", "nine-sixteenths", "low", 8500, 6000, 4000, 550},
		},
	},
	{
		category: "Grips", brand: "Generic", name: "Star Grips",
		summary: "Star grips for BMX", retailPublic: true,
		variants: []variantSpec{
			{"black", "GRP-STAR-BK", "black", "", "in_stock", 4500, 3000, 2000, 250},
			{"blue", "GRP-STAR-BL", "blue", "", "in_stock", 4500, 3000, 2000, 250},
			{"green", "GRP-STAR-GN", "green", "", "in_stock", 4500, 3000, 2000, 250},
		},
	},
	{
		category: "Tubes", brand: "Kenda", name: `Tube 20 x 1.95/2.125 A/V`,
		summary: "20-inch inner tube, Schrader valve", retailPublic: true,
		wheelSize: "20", valveType: "av", widthLow: 1.95, widthHigh: 2.125,
		variants: []variantSpec{
			{"av-reg", "TUB-20-AV", "", "", "in_stock", 3500, 2200, 1500, 180},
		},
	},
	{
		category: "Tubes", brand: "CST", name: `Tube 26 x 1.75/2.125 A/V`,
		summary: "26-inch inner tube, Schrader valve", retailPublic: true,
		wheelSize: "26", valveType: "av", widthLow: 1.75, widthHigh: 2.125,
		variants: []variantSpec{
			{"av-reg", "TUB-26-AV", "", "", "in_stock", 3800, 2400, 1600, 190},
		},
	},
	{
		category: "Tyres", brand: "Kenda", name: "Kenda K-Rad 20 x 2.125",
		summary: "20-inch K-Rad BMX tyre", retailPublic: true, featured: true,
		wheelSize: "20", tyreType: "k-rad", widthLow: 2.125, widthHigh: 2.125,
		variants: []variantSpec{
			{"blk", "TYR-KRAD-20", "", "", "in_stock", 12000, 8500, 6000, 750},
		},
	},
	{
		category: "Tyres", brand: "CST", name: "CST Freestyle 20 x 1.95",
		summary: "20-inch freestyle tyre", retailPublic: true,
		wheelSize: "20", tyreType: "freestyle", widthLow: 1.95, widthHigh: 1.95,
		variants: []variantSpec{
			{"blk", "TYR-FS-20", "", "", "in_stock", 9500, 7000, 5000, 600},
		},
	},
	{
		category: "Tyres", brand: "Wanda", name: "Wanda Road 700c x 25",
		summary: "700c road tyre", retailPublic: false,
		wheelSize: "700c", tyreType: "road", widthLow: 0.98, widthHigh: 0.98,
		variants: []variantSpec{
			{"blk", "TYR-RD-700", "", "", "special_order", 15000, 11000, 8000, 950},
		},
	},
	{
		category: "Rims", brand: "Generic", name: "Alloy Rim 20 inch 36H",
		summary: "20-inch 36-hole alloy rim", retailPublic: true,
		wheelSize: "20",
		variants: []variantSpec{
			{"36h", "RIM-20-36", "", "", "in_stock", 18000, 13000, 9000, 1100},
		},
	},
	{
		category: "Rims", brand: "Generic", name: "Alloy Rim 26 inch 36H",
		summary: "26-inch 36-hole alloy rim", retailPublic: true,
		wheelSize: "26",
		variants: []variantSpec{
			{"36h", "RIM-26-36", "", "", "in_stock", 20000, 15000, 10000, 1250},
		},
	},
	{
		category: "Valve Caps", brand: "Generic", name: "Alloy Valve Caps",
		summary: "Anodised alloy valve caps, pair", retailPublic: true,
		variants: []variantSpec{
			{"red", "VC-AL-R", "red", "", "in_stock", 1500, 900, 600, 60},
			{"blue", "VC-AL-B", "blue", "", "in_stock", 1500, 900, 600, 60},
			{"black", "VC-AL-BK", "black", "", "in_stock", 1500, 900, 600, 60},
		},
	},
	{
		category: "Bike Locks", brand: "Generic", name: "Coil Cable Lock",
		summary: "1.2m coil cable lock with key", retailPublic: true,
		variants: []variantSpec{
			{"black", "LCK-COIL-BK", "black", "", "in_stock", 6500, 4500, 3000, 380},
			{"red", "LCK-COIL-R", "red", "", "low", 6500, 4500, 3000, 380},
		},
	},
	{
		category: "Bikes", brand: "Generic", name: "BMX Freestyle 20",
		summary: "20-inch freestyle BMX complete bike", retailPublic: true, featured: true,
		wheelSize: "20",
		variants: []variantSpec{
			{"black", "BMX-FS-20-BK", "black", "", "in_stock", 185000, 140000, 100000, 12500},
			{"blue", "BMX-FS-20-BL", "blue", "", "special_order", 185000, 140000, 100000, 12500},
		},
	},
	{
		category: "Bikes", brand: "Generic", name: "Mountain Bike 26 21-Speed",
		summary: "26-inch hardtail mountain bike, 21 speed", retailPublic: true,
		wheelSize: "26",
		variants: []variantSpec{
			{"black", "MTB-26-BK", "black", "", "in_stock", 265000, 200000, 145000, 18000},
		},
	},
	{
		// The ambiguous six-identical-helmets case: flagged needs_review, not
		// silently deduped or kept sixfold. Modelled as one product in review.
		category: "Helmets and Safety Pads", brand: "Generic",
		name: "Bicycle Helmet Kids / Multi Sport", summary: "Kids multi-sport helmet (CAS009) — colour unrecorded, pending review",
		retailPublic: false,
		variants: []variantSpec{
			{"cas009", "CAS009", "", "", "unknown", 9000, 6500, 4500, 550},
		},
	},
}

func (s *seeder) seedProducts() error {
	for _, ps := range productSpecs {
		if err := s.seedOneProduct(ps); err != nil {
			return fmt.Errorf("product %q: %w", ps.name, err)
		}
	}
	return nil
}

func (s *seeder) seedOneProduct(ps productSpec) error {
	catID, ok := s.categoryID[ps.category]
	if !ok {
		return fmt.Errorf("unknown category %q", ps.category)
	}
	var brandID *uuid.UUID
	if id, ok := s.brandID[ps.brand]; ok {
		brandID = &id
	}

	productID := newID()
	slug := platform.Slugify(ps.name)
	status := "active"
	if ps.category == "Helmets and Safety Pads" {
		status = "needs_review"
	}

	// Product-level JSONB projection.
	productAttrs := map[string]any{}
	if ps.wheelSize != "" {
		productAttrs["wheel_size"] = ps.wheelSize
	}
	if ps.tyreType != "" {
		productAttrs["tyre_type"] = ps.tyreType
	}
	if ps.valveType != "" {
		productAttrs["valve_type"] = ps.valveType
	}
	if ps.widthLow != 0 {
		productAttrs["width"] = map[string]any{"low": ps.widthLow, "high": ps.widthHigh}
	}
	attrsJSON, err := json.Marshal(productAttrs)
	if err != nil {
		return err
	}

	var publishedAt *time.Time
	if status == "active" {
		now := time.Now()
		publishedAt = &now
	}

	if _, err := s.tx.Exec(s.ctx,
		`INSERT INTO products (id, category_id, brand_id, name, slug, summary, status,
		     is_featured, retail_price_is_public, attrs, published_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11)`,
		productID, catID, brandID, ps.name, slug, ps.summary, status,
		ps.featured, ps.retailPublic, string(attrsJSON), publishedAt); err != nil {
		return err
	}

	// Product-level attribute values (EAV), mirroring the projection above.
	if ps.wheelSize != "" {
		if err := s.insertOptionValue(productID, nil, "wheel_size", ps.wheelSize); err != nil {
			return err
		}
	}
	if ps.tyreType != "" {
		if err := s.insertOptionValue(productID, nil, "tyre_type", ps.tyreType); err != nil {
			return err
		}
	}
	if ps.valveType != "" {
		if err := s.insertOptionValue(productID, nil, "valve_type", ps.valveType); err != nil {
			return err
		}
	}
	if ps.widthLow != 0 {
		if err := s.insertRangeValue(productID, nil, "width", ps.widthLow, ps.widthHigh); err != nil {
			return err
		}
	}

	for i, vs := range ps.variants {
		if err := s.seedVariant(productID, ps, vs, i); err != nil {
			return err
		}
	}
	s.productCount++
	return nil
}

func (s *seeder) seedVariant(productID uuid.UUID, ps productSpec, vs variantSpec, idx int) error {
	variantID := newID()
	sku := platform.Slugify(ps.name) + "-" + vs.skuSuffix

	variantAttrs := map[string]any{}
	if vs.colour != "" {
		variantAttrs["colour"] = vs.colour
	}
	if vs.thread != "" {
		variantAttrs["thread"] = vs.thread
	}
	vAttrsJSON, err := json.Marshal(variantAttrs)
	if err != nil {
		return err
	}

	var supplierItemNo *string
	if vs.supplierItemNo != "" {
		supplierItemNo = &vs.supplierItemNo
	}
	nameSuffix := buildNameSuffix(vs)

	stock := vs.stock
	if stock == "" {
		stock = "unknown"
	}

	if _, err := s.tx.Exec(s.ctx,
		`INSERT INTO product_variants (id, product_id, sku, supplier_item_no, name_suffix,
		     position, stock_status, attrs, is_default)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9)`,
		variantID, productID, sku, supplierItemNo, nameSuffix, idx, stock, string(vAttrsJSON), idx == 0); err != nil {
		return err
	}

	if vs.colour != "" {
		if err := s.insertOptionValue(productID, &variantID, "colour", vs.colour); err != nil {
			return err
		}
	}
	if vs.thread != "" {
		if err := s.insertOptionValue(productID, &variantID, "thread", vs.thread); err != nil {
			return err
		}
	}

	// Four price tiers per variant. Cost is USD; the TTD tiers are TTD.
	prices := []struct {
		tier     string
		amount   int64
		currency string
	}{
		{"cost_usd", vs.costUSD, "USD"},
		{"landed_ttd", vs.landedTTD, "TTD"},
		{"wholesale_ttd", vs.wholesaleTTD, "TTD"},
		{"retail_ttd", vs.retailTTD, "TTD"},
	}
	for _, p := range prices {
		if p.amount == 0 {
			continue
		}
		// Money round-trips through the platform type to prove the value is exact.
		m := platform.NewMoney(p.amount, p.currency)
		if _, err := s.tx.Exec(s.ctx,
			`INSERT INTO prices (id, variant_id, tier, amount_minor, currency)
			 VALUES ($1,$2,$3,$4,$5)`,
			newID(), variantID, p.tier, m.AmountMinor(), m.Currency()); err != nil {
			return err
		}
	}
	return nil
}

func buildNameSuffix(vs variantSpec) *string {
	var parts []string
	if vs.colour != "" {
		parts = append(parts, vs.colour)
	}
	if vs.thread != "" {
		parts = append(parts, vs.thread)
	}
	if len(parts) == 0 {
		return nil
	}
	s := strings.Join(parts, " / ")
	return &s
}

// insertOptionValue writes an enum/color EAV row by resolving the option id.
func (s *seeder) insertOptionValue(productID uuid.UUID, variantID *uuid.UUID, attrKey, optValue string) error {
	attrID, ok := s.attrID[attrKey]
	if !ok {
		return fmt.Errorf("unknown attribute %q", attrKey)
	}
	optID, ok := s.options[attrKey][optValue]
	if !ok {
		return fmt.Errorf("unknown option %s/%s", attrKey, optValue)
	}
	_, err := s.tx.Exec(s.ctx,
		`INSERT INTO product_attribute_values (id, product_id, variant_id, attribute_id, option_id)
		 VALUES ($1,$2,$3,$4,$5)`,
		newID(), productID, variantID, attrID, optID)
	return err
}

// insertRangeValue writes a number_range EAV row.
func (s *seeder) insertRangeValue(productID uuid.UUID, variantID *uuid.UUID, attrKey string, low, high float64) error {
	attrID, ok := s.attrID[attrKey]
	if !ok {
		return fmt.Errorf("unknown attribute %q", attrKey)
	}
	_, err := s.tx.Exec(s.ctx,
		`INSERT INTO product_attribute_values (id, product_id, variant_id, attribute_id, value_num_low, value_num_high)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		newID(), productID, variantID, attrID, low, high)
	return err
}

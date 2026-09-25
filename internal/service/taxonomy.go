package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/khansbikezone/bikezone-api/internal/domain"
	"github.com/khansbikezone/bikezone-api/internal/store"
	"github.com/khansbikezone/bikezone-api/internal/store/gen"
)

// Taxonomy administers the catalogue's structure: the category tree, the
// attribute registry and its options, which attributes apply to which
// category, and brands and suppliers. Editing these is how the owner reshapes
// the catalogue (a category gaining new fields, say) without a code change.
type Taxonomy struct {
	store *store.Store
}

// NewTaxonomy builds the taxonomy service.
func NewTaxonomy(st *store.Store) *Taxonomy { return &Taxonomy{store: st} }

// Field limits.
const (
	maxNameLen        = 100
	maxDescriptionLen = 2000
	maxLabelLen       = 100
	maxHelpTextLen    = 500
	maxUnitLen        = 20
	maxOptionValueLen = 100
	maxOptionLabelLen = 200
)

// ---- categories -------------------------------------------------------------

// CategoryInput is the writable part of a category. On update it replaces
// the category, except that an empty Slug or a nil IsActive keeps the
// current value.
type CategoryInput struct {
	ParentID    *uuid.UUID `json:"parentId"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	Position    int        `json:"position"`
	Description *string    `json:"description"`
	HeroAssetID *uuid.UUID `json:"heroAssetId"`
	IsActive    *bool      `json:"isActive"`
}

// ltreeLabel turns a slug into a valid ltree label (hyphens are not allowed).
func ltreeLabel(slug string) string { return strings.ReplaceAll(slug, "-", "_") }

// ListCategories returns every category, active or not, in tree (path) order.
func (t *Taxonomy) ListCategories(ctx context.Context) ([]domain.Category, error) {
	rows, err := t.store.ListCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	out := make([]domain.Category, len(rows))
	for i, r := range rows {
		out[i] = store.Category(gen.GetCategoryByIDRow(r))
	}
	return out, nil
}

// GetCategory returns one category by id.
func (t *Taxonomy) GetCategory(ctx context.Context, id uuid.UUID) (domain.Category, error) {
	row, err := t.store.GetCategoryByID(ctx, id)
	if err != nil {
		return domain.Category{}, fmt.Errorf("category %s: %w", id, err)
	}
	return store.Category(row), nil
}

func (in *CategoryInput) validate(v *domain.ValidationError) {
	in.Name = requireText(v, "name", in.Name, maxNameLen)
	in.Description = optionalText(v, "description", in.Description, maxDescriptionLen)
	if in.Slug = strings.TrimSpace(in.Slug); in.Slug != "" {
		validateSlug(v, "slug", in.Slug)
	}
	if in.Position < 0 {
		v.Add("position", "must not be negative")
	}
}

// parentPath resolves the path a child of parentID would hang under ("" for
// a root category).
func parentPath(ctx context.Context, q *store.Queries, parentID *uuid.UUID) (string, error) {
	if parentID == nil {
		return "", nil
	}
	p, err := q.GetCategoryByID(ctx, *parentID)
	if errors.Is(err, domain.ErrNotFound) {
		return "", domain.Invalid("parentId", "does not exist")
	}
	if err != nil {
		return "", fmt.Errorf("load parent category: %w", err)
	}
	return p.Path + ".", nil
}

// CreateCategory adds a category, deriving the slug from the name when none
// is given.
func (t *Taxonomy) CreateCategory(ctx context.Context, in CategoryInput) (domain.Category, error) {
	v := &domain.ValidationError{}
	in.validate(v)
	if err := v.Err(); err != nil {
		return domain.Category{}, err
	}
	var out domain.Category
	err := t.store.InTx(ctx, func(q *store.Queries) error {
		prefix, err := parentPath(ctx, q, in.ParentID)
		if err != nil {
			return err
		}
		if in.Slug == "" {
			if in.Slug, err = uniqueSlug(ctx, in.Name, q.CategorySlugExists); err != nil {
				return err
			}
		}
		row, err := q.CreateCategory(ctx, gen.CreateCategoryParams{
			ID:          domain.NewID(),
			ParentID:    in.ParentID,
			Name:        in.Name,
			Slug:        in.Slug,
			Path:        prefix + ltreeLabel(in.Slug),
			Position:    int32(in.Position),
			Description: store.Text(in.Description),
			HeroAssetID: in.HeroAssetID,
			IsActive:    in.IsActive == nil || *in.IsActive,
		})
		if err != nil {
			return err
		}
		out = store.Category(gen.GetCategoryByIDRow(row))
		return q.Audit(ctx, "category.create", "category", &out.ID, nil, out)
	})
	return out, err
}

// UpdateCategory replaces a category. Changing its slug or parent moves its
// whole subtree (the ltree paths are rewritten in the same transaction).
func (t *Taxonomy) UpdateCategory(ctx context.Context, id uuid.UUID, in CategoryInput, ifMatch string) (domain.Category, error) {
	v := &domain.ValidationError{}
	in.validate(v)
	if err := v.Err(); err != nil {
		return domain.Category{}, err
	}
	var out domain.Category
	err := t.store.InTx(ctx, func(q *store.Queries) error {
		cur, err := q.GetCategoryByID(ctx, id)
		if err != nil {
			return fmt.Errorf("category %s: %w", id, err)
		}
		before := store.Category(cur)
		if err := checkIfMatch(ifMatch, before.UpdatedAt); err != nil {
			return err
		}
		if in.Slug == "" {
			in.Slug = before.Slug
		}
		if in.IsActive == nil {
			in.IsActive = &before.IsActive
		}
		prefix, err := parentPath(ctx, q, in.ParentID)
		if err != nil {
			return err
		}
		newPath := prefix + ltreeLabel(in.Slug)
		// A category cannot become its own ancestor.
		if in.ParentID != nil && (*in.ParentID == id || strings.HasPrefix(prefix, before.Path+".")) {
			return domain.Invalid("parentId", "cannot be the category itself or one of its descendants")
		}
		_, err = q.UpdateCategory(ctx, gen.UpdateCategoryParams{
			ID:                id,
			ParentID:          in.ParentID,
			Name:              in.Name,
			Slug:              in.Slug,
			Position:          int32(in.Position),
			Description:       store.Text(in.Description),
			HeroAssetID:       in.HeroAssetID,
			IsActive:          *in.IsActive,
			ExpectedUpdatedAt: store.Timestamptz(before.UpdatedAt),
		})
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrVersionMismatch // it existed a moment ago: someone else changed it
		}
		if err != nil {
			return err
		}
		if newPath != before.Path {
			if err := q.MoveCategorySubtree(ctx, gen.MoveCategorySubtreeParams{OldPath: before.Path, NewPath: newPath}); err != nil {
				return err
			}
		}
		if in.Name != before.Name {
			// Category names are part of every product's search vector.
			if err := q.Enqueue(ctx, JobReindexSearch, struct{}{}); err != nil {
				return err
			}
		}
		row, err := q.GetCategoryByID(ctx, id)
		if err != nil {
			return err
		}
		out = store.Category(row)
		return q.Audit(ctx, "category.update", "category", &id, before, out)
	})
	return out, err
}

// DeleteCategory removes an empty category. One that still has products or
// subcategories is refused with a conflict.
func (t *Taxonomy) DeleteCategory(ctx context.Context, id uuid.UUID) error {
	return t.store.InTx(ctx, func(q *store.Queries) error {
		cur, err := q.GetCategoryByID(ctx, id)
		if err != nil {
			return fmt.Errorf("category %s: %w", id, err)
		}
		if _, err := q.DeleteCategory(ctx, id); err != nil {
			return fmt.Errorf("delete category: %w", err)
		}
		return q.Audit(ctx, "category.delete", "category", &id, store.Category(cur), nil)
	})
}

// ---- category attribute bindings -----------------------------------------------

// BindingInput configures how an attribute applies within one category.
type BindingInput struct {
	Position      int     `json:"position"`
	IsRequired    bool    `json:"isRequired"`
	IsVariantAxis bool    `json:"isVariantAxis"`
	LabelOverride *string `json:"labelOverride"`
}

// CategoryAttributes lists the attributes bound to a category, with options.
func (t *Taxonomy) CategoryAttributes(ctx context.Context, categoryID uuid.UUID) ([]domain.CategoryAttribute, error) {
	if _, err := t.GetCategory(ctx, categoryID); err != nil {
		return nil, err
	}
	s, err := loadCategorySchema(ctx, t.store.Queries, categoryID)
	if err != nil {
		return nil, err
	}
	return s.bindings, nil
}

// BindAttribute makes an attribute apply to a category, or updates how it
// applies. Moving an attribute between product level and variant level is
// refused while products still hold values for it, since those values would
// sit at the wrong level.
func (t *Taxonomy) BindAttribute(ctx context.Context, categoryID, attributeID uuid.UUID, in BindingInput) error {
	v := &domain.ValidationError{}
	in.LabelOverride = optionalText(v, "labelOverride", in.LabelOverride, maxLabelLen)
	if in.Position < 0 {
		v.Add("position", "must not be negative")
	}
	if err := v.Err(); err != nil {
		return err
	}
	return t.store.InTx(ctx, func(q *store.Queries) error {
		if _, err := q.GetCategoryByID(ctx, categoryID); err != nil {
			return fmt.Errorf("category %s: %w", categoryID, err)
		}
		if _, err := q.GetAttributeByID(ctx, attributeID); err != nil {
			return fmt.Errorf("attribute %s: %w", attributeID, err)
		}
		s, err := loadCategorySchema(ctx, q, categoryID)
		if err != nil {
			return err
		}
		if cur, ok := s.byID[attributeID]; ok && cur.IsVariantAxis != in.IsVariantAxis {
			has, err := q.CategoryAttributeHasValues(ctx, gen.CategoryAttributeHasValuesParams{CategoryID: categoryID, AttributeID: attributeID})
			if err != nil {
				return err
			}
			if has {
				return fmt.Errorf("%w: products in this category already store values for this attribute; clear them before changing isVariantAxis", domain.ErrConflict)
			}
		}
		if err := q.BindCategoryAttribute(ctx, gen.BindCategoryAttributeParams{
			CategoryID:    categoryID,
			AttributeID:   attributeID,
			Position:      int32(in.Position),
			IsRequired:    in.IsRequired,
			IsVariantAxis: in.IsVariantAxis,
			LabelOverride: store.Text(in.LabelOverride),
		}); err != nil {
			return err
		}
		return q.Audit(ctx, "category.attribute.bind", "category", &categoryID, nil,
			map[string]any{"attributeId": attributeID, "binding": in})
	})
}

// UnbindAttribute removes an attribute from a category, deleting the values
// products in that category held for it and rebuilding their filter
// projections, all in one transaction.
func (t *Taxonomy) UnbindAttribute(ctx context.Context, categoryID, attributeID uuid.UUID) error {
	return t.store.InTx(ctx, func(q *store.Queries) error {
		n, err := q.UnbindCategoryAttribute(ctx, gen.UnbindCategoryAttributeParams{CategoryID: categoryID, AttributeID: attributeID})
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("attribute %s is not bound to category %s: %w", attributeID, categoryID, domain.ErrNotFound)
		}
		affected, err := q.DeleteCategoryAttributeValues(ctx, gen.DeleteCategoryAttributeValuesParams{CategoryID: categoryID, AttributeID: attributeID})
		if err != nil {
			return err
		}
		s, err := loadCategorySchema(ctx, q, categoryID)
		if err != nil {
			return err
		}
		rebuilt := map[uuid.UUID]bool{} // a product has one row per deleted value
		for _, pid := range affected {
			if rebuilt[pid] {
				continue
			}
			rebuilt[pid] = true
			if err := rebuildProjections(ctx, q, pid, s); err != nil {
				return err
			}
		}
		return q.Audit(ctx, "category.attribute.unbind", "category", &categoryID, map[string]any{"attributeId": attributeID}, nil)
	})
}

// FormSchema is the server-driven product form for a category. Version is a
// hash of the fields, so it changes exactly when the form does.
func (t *Taxonomy) FormSchema(ctx context.Context, categoryID uuid.UUID) (domain.FormSchema, error) {
	bindings, err := t.CategoryAttributes(ctx, categoryID)
	if err != nil {
		return domain.FormSchema{}, err
	}
	fs := domain.FormSchema{CategoryID: categoryID, Fields: make([]domain.FormField, len(bindings))}
	for i := range bindings {
		b := &bindings[i]
		f := domain.FormField{
			Key:           b.Attribute.Key,
			Label:         b.EffectiveLabel(),
			InputType:     b.Attribute.InputType,
			DataType:      b.Attribute.DataType,
			Unit:          b.Attribute.Unit,
			Required:      b.IsRequired,
			IsVariantAxis: b.IsVariantAxis,
			HelpText:      b.Attribute.HelpText,
			Position:      b.Position,
		}
		for _, o := range b.Attribute.Options {
			f.Options = append(f.Options, domain.FormOption{Value: o.Value, Label: o.Label, SwatchHex: o.SwatchHex, Position: o.Position})
		}
		fs.Fields[i] = f
	}
	b, err := json.Marshal(fs.Fields)
	if err != nil {
		return domain.FormSchema{}, fmt.Errorf("encode form fields: %w", err)
	}
	h := fnv.New32a()
	_, _ = h.Write(b)
	fs.Version = int(h.Sum32())
	return fs, nil
}

// ---- attributes and options ---------------------------------------------------

// AttributeInput is the writable part of an attribute. Key, DataType and
// Options are only used on create: the key and type are immutable (stored
// values and filter URLs depend on them) and options have their own calls.
type AttributeInput struct {
	Key          string          `json:"key"`
	Label        string          `json:"label"`
	DataType     domain.DataType `json:"dataType"`
	Unit         *string         `json:"unit"`
	InputType    string          `json:"inputType"`
	IsFilterable *bool           `json:"isFilterable"` // default true
	IsSearchable bool            `json:"isSearchable"`
	HelpText     *string         `json:"helpText"`
	Options      []OptionInput   `json:"options"`
}

// OptionInput is the writable part of an attribute option. Value is only used
// on create: it is the stable token stored in projections and filter URLs.
type OptionInput struct {
	Value     string  `json:"value"`
	Label     string  `json:"label"`
	SwatchHex *string `json:"swatchHex"`
	Position  int     `json:"position"`
}

// Attribute keys are snake_case identifiers. They may not end in _min or _max:
// those suffixes are how list URLs express numeric bounds (attr.width_min).
var attributeKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

var swatchPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// inputTypes are the UI hints clients know how to render, with the default
// hint for each data type.
var (
	inputTypes       = []string{"select", "radio", "swatch", "range", "checkbox", "text"}
	defaultInputType = map[domain.DataType]string{
		domain.DataTypeText: "text", domain.DataTypeNumber: "text", domain.DataTypeNumberRange: "range",
		domain.DataTypeBoolean: "checkbox", domain.DataTypeEnum: "select", domain.DataTypeMultiEnum: "checkbox",
		domain.DataTypeColor: "swatch",
	}
)

func (in *AttributeInput) validateCommon(v *domain.ValidationError) {
	in.Label = requireText(v, "label", in.Label, maxLabelLen)
	in.Unit = optionalText(v, "unit", in.Unit, maxUnitLen)
	in.HelpText = optionalText(v, "helpText", in.HelpText, maxHelpTextLen)
	if in.InputType = strings.TrimSpace(in.InputType); in.InputType == "" {
		in.InputType = defaultInputType[in.DataType]
	} else if !slices.Contains(inputTypes, in.InputType) {
		v.Add("inputType", "must be one of %s", strings.Join(inputTypes, ", "))
	}
}

func (in *OptionInput) validate(v *domain.ValidationError, field string, withValue bool) {
	if withValue {
		in.Value = requireText(v, field+".value", in.Value, maxOptionValueLen)
	}
	in.Label = requireText(v, field+".label", in.Label, maxOptionLabelLen)
	if in.SwatchHex = optionalText(v, field+".swatchHex", in.SwatchHex, 7); in.SwatchHex != nil && !swatchPattern.MatchString(*in.SwatchHex) {
		v.Add(field+".swatchHex", "must be a #rrggbb colour")
	}
	if in.Position < 0 {
		v.Add(field+".position", "must not be negative")
	}
}

// ListAttributes returns every attribute with its options.
func (t *Taxonomy) ListAttributes(ctx context.Context) ([]domain.Attribute, error) {
	byKey, err := loadAttributes(ctx, t.store.Queries)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Attribute, 0, len(byKey))
	for _, a := range byKey {
		out = append(out, *a)
	}
	slices.SortFunc(out, func(a, b domain.Attribute) int { return strings.Compare(a.Key, b.Key) })
	return out, nil
}

// GetAttribute returns one attribute with its options.
func (t *Taxonomy) GetAttribute(ctx context.Context, id uuid.UUID) (domain.Attribute, error) {
	return getAttribute(ctx, t.store.Queries, id)
}

func getAttribute(ctx context.Context, q *store.Queries, id uuid.UUID) (domain.Attribute, error) {
	row, err := q.GetAttributeByID(ctx, id)
	if err != nil {
		return domain.Attribute{}, fmt.Errorf("attribute %s: %w", id, err)
	}
	a := store.Attribute(row)
	opts, err := loadOptions(ctx, q, []uuid.UUID{id})
	if err != nil {
		return domain.Attribute{}, err
	}
	a.Options = opts[id]
	return a, nil
}

// CreateAttribute registers an attribute, with its options if it uses them.
func (t *Taxonomy) CreateAttribute(ctx context.Context, in AttributeInput) (domain.Attribute, error) {
	v := &domain.ValidationError{}
	in.Key = strings.TrimSpace(in.Key)
	if !attributeKeyPattern.MatchString(in.Key) || strings.HasSuffix(in.Key, "_min") || strings.HasSuffix(in.Key, "_max") {
		v.Add("key", "must be snake_case (a-z, 0-9, _), start with a letter, be at most 63 characters, and not end in _min or _max")
	}
	if !in.DataType.Valid() {
		v.Add("dataType", "must be one of text, number, number_range, boolean, enum, multi_enum, color")
	}
	in.validateCommon(v)
	if !in.DataType.UsesOptions() && len(in.Options) > 0 {
		v.Add("options", "only enum, multi_enum and color attributes have options")
	}
	seen := map[string]bool{}
	for i := range in.Options {
		field := fmt.Sprintf("options[%d]", i)
		in.Options[i].validate(v, field, true)
		if seen[in.Options[i].Value] {
			v.Add(field+".value", "duplicates another option")
		}
		seen[in.Options[i].Value] = true
	}
	if err := v.Err(); err != nil {
		return domain.Attribute{}, err
	}
	var out domain.Attribute
	err := t.store.InTx(ctx, func(q *store.Queries) error {
		row, err := q.CreateAttribute(ctx, gen.CreateAttributeParams{
			ID:           domain.NewID(),
			Key:          in.Key,
			Label:        in.Label,
			DataType:     gen.AttrDataType(in.DataType),
			Unit:         store.Text(in.Unit),
			InputType:    in.InputType,
			IsFilterable: in.IsFilterable == nil || *in.IsFilterable,
			IsSearchable: in.IsSearchable,
			HelpText:     store.Text(in.HelpText),
		})
		if err != nil {
			return err
		}
		out = store.Attribute(row)
		for _, o := range in.Options {
			opt, err := q.CreateAttributeOption(ctx, gen.CreateAttributeOptionParams{
				ID: domain.NewID(), AttributeID: out.ID, Value: o.Value, Label: o.Label,
				SwatchHex: store.Text(o.SwatchHex), Position: int32(o.Position),
			})
			if err != nil {
				return err
			}
			out.Options = append(out.Options, store.AttributeOption(opt))
		}
		return q.Audit(ctx, "attribute.create", "attribute", &out.ID, nil, out)
	})
	return out, err
}

// UpdateAttribute changes an attribute's presentation and flags.
func (t *Taxonomy) UpdateAttribute(ctx context.Context, id uuid.UUID, in AttributeInput, ifMatch string) (domain.Attribute, error) {
	var out domain.Attribute
	err := t.store.InTx(ctx, func(q *store.Queries) error {
		before, err := getAttribute(ctx, q, id)
		if err != nil {
			return err
		}
		if err := checkIfMatch(ifMatch, before.UpdatedAt); err != nil {
			return err
		}
		v := &domain.ValidationError{}
		if k := strings.TrimSpace(in.Key); k != "" && k != before.Key {
			v.Add("key", "cannot be changed")
		}
		if in.DataType != "" && in.DataType != before.DataType {
			v.Add("dataType", "cannot be changed")
		}
		if len(in.Options) > 0 {
			v.Add("options", "are managed through the attribute's options endpoints")
		}
		in.DataType = before.DataType
		in.validateCommon(v)
		if err := v.Err(); err != nil {
			return err
		}
		_, err = q.UpdateAttribute(ctx, gen.UpdateAttributeParams{
			ID:                id,
			Label:             in.Label,
			Unit:              store.Text(in.Unit),
			InputType:         in.InputType,
			IsFilterable:      in.IsFilterable == nil || *in.IsFilterable,
			IsSearchable:      in.IsSearchable,
			HelpText:          store.Text(in.HelpText),
			ExpectedUpdatedAt: store.Timestamptz(before.UpdatedAt),
		})
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrVersionMismatch
		}
		if err != nil {
			return err
		}
		if out, err = getAttribute(ctx, q, id); err != nil {
			return err
		}
		return q.Audit(ctx, "attribute.update", "attribute", &id, before, out)
	})
	return out, err
}

// DeleteAttribute removes an attribute that no category binds and no product
// uses; otherwise it is refused with a conflict.
func (t *Taxonomy) DeleteAttribute(ctx context.Context, id uuid.UUID) error {
	return t.store.InTx(ctx, func(q *store.Queries) error {
		before, err := getAttribute(ctx, q, id)
		if err != nil {
			return err
		}
		if _, err := q.DeleteAttribute(ctx, id); err != nil {
			return fmt.Errorf("delete attribute: %w", err)
		}
		return q.Audit(ctx, "attribute.delete", "attribute", &id, before, nil)
	})
}

// CreateOption adds an option to an enum, multi_enum or color attribute.
func (t *Taxonomy) CreateOption(ctx context.Context, attributeID uuid.UUID, in OptionInput) (domain.AttributeOption, error) {
	v := &domain.ValidationError{}
	in.validate(v, "option", true)
	if err := v.Err(); err != nil {
		return domain.AttributeOption{}, err
	}
	var out domain.AttributeOption
	err := t.store.InTx(ctx, func(q *store.Queries) error {
		a, err := getAttribute(ctx, q, attributeID)
		if err != nil {
			return err
		}
		if !a.DataType.UsesOptions() {
			return domain.Invalid("option", "%s attributes do not have options", a.DataType)
		}
		row, err := q.CreateAttributeOption(ctx, gen.CreateAttributeOptionParams{
			ID: domain.NewID(), AttributeID: attributeID, Value: in.Value, Label: in.Label,
			SwatchHex: store.Text(in.SwatchHex), Position: int32(in.Position),
		})
		if err != nil {
			return err
		}
		out = store.AttributeOption(row)
		if err := q.TouchAttribute(ctx, attributeID); err != nil {
			return err
		}
		return q.Audit(ctx, "attribute.option.create", "attribute", &attributeID, nil, out)
	})
	return out, err
}

// UpdateOption changes an option's label, swatch or position. Its value
// token is immutable.
func (t *Taxonomy) UpdateOption(ctx context.Context, attributeID, optionID uuid.UUID, in OptionInput) (domain.AttributeOption, error) {
	v := &domain.ValidationError{}
	in.validate(v, "option", false)
	if err := v.Err(); err != nil {
		return domain.AttributeOption{}, err
	}
	var out domain.AttributeOption
	err := t.store.InTx(ctx, func(q *store.Queries) error {
		a, err := getAttribute(ctx, q, attributeID)
		if err != nil {
			return err
		}
		before := optionByID(&a, optionID)
		if before == nil {
			return fmt.Errorf("option %s: %w", optionID, domain.ErrNotFound)
		}
		if val := strings.TrimSpace(in.Value); val != "" && val != before.Value {
			return domain.Invalid("option.value", "cannot be changed")
		}
		row, err := q.UpdateAttributeOption(ctx, gen.UpdateAttributeOptionParams{
			ID: optionID, AttributeID: attributeID, Label: in.Label, SwatchHex: store.Text(in.SwatchHex), Position: int32(in.Position),
		})
		if err != nil {
			return err
		}
		out = store.AttributeOption(row)
		if err := q.TouchAttribute(ctx, attributeID); err != nil {
			return err
		}
		if out.Label != before.Label {
			// Option labels are indexed for search.
			if err := q.Enqueue(ctx, JobReindexSearch, struct{}{}); err != nil {
				return err
			}
		}
		return q.Audit(ctx, "attribute.option.update", "attribute", &attributeID, *before, out)
	})
	return out, err
}

// DeleteOption removes an option no product uses; otherwise it is refused
// with a conflict.
func (t *Taxonomy) DeleteOption(ctx context.Context, attributeID, optionID uuid.UUID) error {
	return t.store.InTx(ctx, func(q *store.Queries) error {
		n, err := q.DeleteAttributeOption(ctx, gen.DeleteAttributeOptionParams{ID: optionID, AttributeID: attributeID})
		if err != nil {
			return fmt.Errorf("delete option: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("option %s: %w", optionID, domain.ErrNotFound)
		}
		if err := q.TouchAttribute(ctx, attributeID); err != nil {
			return err
		}
		return q.Audit(ctx, "attribute.option.delete", "attribute", &attributeID, map[string]any{"optionId": optionID}, nil)
	})
}

// ---- brands and suppliers ------------------------------------------------------

// BrandInput is the writable part of a brand. An empty Slug on create derives
// one from the name; on update it keeps the current slug.
type BrandInput struct {
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	LogoAssetID *uuid.UUID `json:"logoAssetId"`
	Position    int        `json:"position"`
}

func (in *BrandInput) validate(v *domain.ValidationError) {
	in.Name = requireText(v, "name", in.Name, maxNameLen)
	if in.Slug = strings.TrimSpace(in.Slug); in.Slug != "" {
		validateSlug(v, "slug", in.Slug)
	}
	if in.Position < 0 {
		v.Add("position", "must not be negative")
	}
}

// ListBrands returns every brand.
func (t *Taxonomy) ListBrands(ctx context.Context) ([]domain.Brand, error) {
	rows, err := t.store.ListBrands(ctx)
	if err != nil {
		return nil, fmt.Errorf("list brands: %w", err)
	}
	out := make([]domain.Brand, len(rows))
	for i, r := range rows {
		out[i] = store.Brand(r)
	}
	return out, nil
}

// CreateBrand adds a brand.
func (t *Taxonomy) CreateBrand(ctx context.Context, in BrandInput) (domain.Brand, error) {
	v := &domain.ValidationError{}
	in.validate(v)
	if err := v.Err(); err != nil {
		return domain.Brand{}, err
	}
	var out domain.Brand
	err := t.store.InTx(ctx, func(q *store.Queries) error {
		b, err := t.CreateBrandInTx(ctx, q, in)
		out = b
		return err
	})
	return out, err
}

// CreateBrandInTx inserts a validated brand inside the caller's transaction
// (the importer creates brands it meets for the first time this way).
func (t *Taxonomy) CreateBrandInTx(ctx context.Context, q *store.Queries, in BrandInput) (domain.Brand, error) {
	if in.Slug == "" {
		slug, err := uniqueSlug(ctx, in.Name, q.BrandSlugExists)
		if err != nil {
			return domain.Brand{}, err
		}
		in.Slug = slug
	}
	row, err := q.CreateBrand(ctx, gen.CreateBrandParams{
		ID: domain.NewID(), Name: in.Name, Slug: in.Slug, LogoAssetID: in.LogoAssetID, Position: int32(in.Position),
	})
	if err != nil {
		return domain.Brand{}, err
	}
	b := store.Brand(row)
	return b, q.Audit(ctx, "brand.create", "brand", &b.ID, nil, b)
}

// UpdateBrand replaces a brand.
func (t *Taxonomy) UpdateBrand(ctx context.Context, id uuid.UUID, in BrandInput) (domain.Brand, error) {
	v := &domain.ValidationError{}
	in.validate(v)
	if err := v.Err(); err != nil {
		return domain.Brand{}, err
	}
	var out domain.Brand
	err := t.store.InTx(ctx, func(q *store.Queries) error {
		cur, err := q.GetBrandByID(ctx, id)
		if err != nil {
			return fmt.Errorf("brand %s: %w", id, err)
		}
		before := store.Brand(cur)
		if in.Slug == "" {
			in.Slug = before.Slug
		}
		row, err := q.UpdateBrand(ctx, gen.UpdateBrandParams{
			ID: id, Name: in.Name, Slug: in.Slug, LogoAssetID: in.LogoAssetID, Position: int32(in.Position),
		})
		if err != nil {
			return err
		}
		out = store.Brand(row)
		if out.Name != before.Name {
			// Brand names are part of every product's search vector.
			if err := q.Enqueue(ctx, JobReindexSearch, struct{}{}); err != nil {
				return err
			}
		}
		return q.Audit(ctx, "brand.update", "brand", &id, before, out)
	})
	return out, err
}

// DeleteBrand removes a brand; its products keep existing without one.
func (t *Taxonomy) DeleteBrand(ctx context.Context, id uuid.UUID) error {
	return t.store.InTx(ctx, func(q *store.Queries) error {
		cur, err := q.GetBrandByID(ctx, id)
		if err != nil {
			return fmt.Errorf("brand %s: %w", id, err)
		}
		if _, err := q.DeleteBrand(ctx, id); err != nil {
			return err
		}
		if err := q.Enqueue(ctx, JobReindexSearch, struct{}{}); err != nil {
			return err
		}
		return q.Audit(ctx, "brand.delete", "brand", &id, store.Brand(cur), nil)
	})
}

// SupplierInput is the writable part of a supplier. ADMIN ONLY data.
type SupplierInput struct {
	Name  string  `json:"name"`
	Code  *string `json:"code"`
	Notes *string `json:"notes"`
}

func (in *SupplierInput) validate(v *domain.ValidationError) {
	in.Name = requireText(v, "name", in.Name, 200)
	in.Code = optionalText(v, "code", in.Code, 50)
	in.Notes = optionalText(v, "notes", in.Notes, maxDescriptionLen)
}

// ListSuppliers returns every supplier.
func (t *Taxonomy) ListSuppliers(ctx context.Context) ([]domain.Supplier, error) {
	rows, err := t.store.ListSuppliers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list suppliers: %w", err)
	}
	out := make([]domain.Supplier, len(rows))
	for i, r := range rows {
		out[i] = store.Supplier(r)
	}
	return out, nil
}

// CreateSupplier adds a supplier.
func (t *Taxonomy) CreateSupplier(ctx context.Context, in SupplierInput) (domain.Supplier, error) {
	v := &domain.ValidationError{}
	in.validate(v)
	if err := v.Err(); err != nil {
		return domain.Supplier{}, err
	}
	var out domain.Supplier
	err := t.store.InTx(ctx, func(q *store.Queries) error {
		s, err := t.CreateSupplierInTx(ctx, q, in)
		out = s
		return err
	})
	return out, err
}

// CreateSupplierInTx inserts a validated supplier inside the caller's
// transaction.
func (t *Taxonomy) CreateSupplierInTx(ctx context.Context, q *store.Queries, in SupplierInput) (domain.Supplier, error) {
	row, err := q.CreateSupplier(ctx, gen.CreateSupplierParams{
		ID: domain.NewID(), Name: in.Name, Code: store.Text(in.Code), Notes: store.Text(in.Notes),
	})
	if err != nil {
		return domain.Supplier{}, err
	}
	s := store.Supplier(row)
	return s, q.Audit(ctx, "supplier.create", "supplier", &s.ID, nil, s)
}

// UpdateSupplier replaces a supplier.
func (t *Taxonomy) UpdateSupplier(ctx context.Context, id uuid.UUID, in SupplierInput) (domain.Supplier, error) {
	v := &domain.ValidationError{}
	in.validate(v)
	if err := v.Err(); err != nil {
		return domain.Supplier{}, err
	}
	var out domain.Supplier
	err := t.store.InTx(ctx, func(q *store.Queries) error {
		cur, err := q.GetSupplierByID(ctx, id)
		if err != nil {
			return fmt.Errorf("supplier %s: %w", id, err)
		}
		row, err := q.UpdateSupplier(ctx, gen.UpdateSupplierParams{
			ID: id, Name: in.Name, Code: store.Text(in.Code), Notes: store.Text(in.Notes),
		})
		if err != nil {
			return err
		}
		out = store.Supplier(row)
		return q.Audit(ctx, "supplier.update", "supplier", &id, store.Supplier(cur), out)
	})
	return out, err
}

// DeleteSupplier removes a supplier; variants keep existing without one.
func (t *Taxonomy) DeleteSupplier(ctx context.Context, id uuid.UUID) error {
	return t.store.InTx(ctx, func(q *store.Queries) error {
		cur, err := q.GetSupplierByID(ctx, id)
		if err != nil {
			return fmt.Errorf("supplier %s: %w", id, err)
		}
		if _, err := q.DeleteSupplier(ctx, id); err != nil {
			return err
		}
		return q.Audit(ctx, "supplier.delete", "supplier", &id, store.Supplier(cur), nil)
	})
}

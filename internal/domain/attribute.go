package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// DataType controls which value column in product_attribute_values is populated
// and which filter operators the search layer will accept. It is the linchpin of
// the dynamic attribute system: the same table stores every attribute, and the
// data type decides how a value is written, indexed, and filtered.
type DataType string

const (
	DataTypeText        DataType = "text"
	DataTypeNumber      DataType = "number"
	DataTypeNumberRange DataType = "number_range" // e.g. tyre width 1.95/2.125
	DataTypeBoolean     DataType = "boolean"
	DataTypeEnum        DataType = "enum"
	DataTypeMultiEnum   DataType = "multi_enum"
	DataTypeColor       DataType = "color"
)

// Valid reports whether d is a known data type; used to reject bad admin input
// before it reaches the database enum.
func (d DataType) Valid() bool {
	switch d {
	case DataTypeText, DataTypeNumber, DataTypeNumberRange, DataTypeBoolean,
		DataTypeEnum, DataTypeMultiEnum, DataTypeColor:
		return true
	default:
		return false
	}
}

// UsesOptions reports whether values of this type reference attribute_options
// rows rather than storing an inline scalar.
func (d DataType) UsesOptions() bool {
	return d == DataTypeEnum || d == DataTypeMultiEnum || d == DataTypeColor
}

// Attribute is a globally-registered field that categories can bind. Key is the
// stable machine identifier (immutable after creation) so renaming the human
// Label never breaks stored values or query params; a single global wheel_size
// attribute, for instance, is shared by every wheel-bearing category.
type Attribute struct {
	ID           uuid.UUID
	Key          string
	Label        string
	DataType     DataType
	Unit         *string // "in", "mm", "T", "speed"
	InputType    string  // UI hint: select|radio|swatch|range|checkbox|text
	IsFilterable bool
	IsSearchable bool
	HelpText     *string
	CreatedAt    time.Time
	UpdatedAt    time.Time

	// Options is populated for enum/multi_enum/color attributes.
	Options []AttributeOption
}

// AttributeOption is one allowed value of an enum-like attribute. Value is the
// canonical, filter-stable token ("20"); Label is what the UI shows (`20"`).
// SwatchHex renders a colour chip for color attributes.
type AttributeOption struct {
	ID          uuid.UUID
	AttributeID uuid.UUID
	Value       string
	Label       string
	SwatchHex   *string
	Position    int
}

// CategoryAttribute binds an Attribute to a Category, which is what makes the
// attribute apply there. IsVariantAxis marks attributes that differentiate
// variants (colour, thread, size) from those describing the whole product
// (material, brand, discipline). Editing these bindings from the admin UI is how
// a new category like Bikes gains a dozen fields with zero migrations.
type CategoryAttribute struct {
	CategoryID    uuid.UUID
	Attribute     Attribute
	Position      int
	IsRequired    bool
	IsVariantAxis bool
	LabelOverride *string
}

// EffectiveLabel returns LabelOverride when set, else the attribute's own Label,
// so a category can rename a shared attribute for its own context.
func (ca *CategoryAttribute) EffectiveLabel() string {
	if ca.LabelOverride != nil && *ca.LabelOverride != "" {
		return *ca.LabelOverride
	}
	return ca.Attribute.Label
}

// AttributeValue is one typed value attached to a product or a specific variant.
// Exactly one logical value slot is populated, determined by the attribute's
// DataType. Construct it via NewAttributeValue so the invariant is enforced once,
// centrally, rather than at every call site.
type AttributeValue struct {
	AttributeID uuid.UUID
	VariantID   *uuid.UUID // nil => applies to the whole product
	OptionIDs   []uuid.UUID
	Text        *string
	Num         *float64
	NumLow      *float64
	NumHigh     *float64
	Bool        *bool
	ETRTO       *string // reserved; only meaningful on size attributes
}

// NewAttributeValue validates that the provided fields match dt and returns a
// populated AttributeValue, or ErrValidation (wrapped with detail) if the shape
// is wrong. This is the single constructor the service and importer must use.
func NewAttributeValue(attributeID uuid.UUID, dt DataType, variantID *uuid.UUID, av AttributeValue) (AttributeValue, error) {
	av.AttributeID = attributeID
	av.VariantID = variantID

	fail := func(msg string) (AttributeValue, error) {
		return AttributeValue{}, fmt.Errorf("%w: attribute %s (%s): %s", ErrValidation, attributeID, dt, msg)
	}

	switch dt {
	case DataTypeText:
		if av.Text == nil {
			return fail("text value required")
		}
	case DataTypeNumber:
		if av.Num == nil {
			return fail("numeric value required")
		}
	case DataTypeNumberRange:
		if av.NumLow == nil || av.NumHigh == nil {
			return fail("both low and high bounds required")
		}
		if *av.NumLow > *av.NumHigh {
			return fail("low bound exceeds high bound")
		}
	case DataTypeBoolean:
		if av.Bool == nil {
			return fail("boolean value required")
		}
	case DataTypeEnum, DataTypeColor:
		if len(av.OptionIDs) != 1 {
			return fail("exactly one option required")
		}
	case DataTypeMultiEnum:
		if len(av.OptionIDs) == 0 {
			return fail("at least one option required")
		}
	default:
		return fail("unknown data type")
	}
	return av, nil
}

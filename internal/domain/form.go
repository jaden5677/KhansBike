package domain

import "github.com/google/uuid"

// FormSchema is the server-driven contract that every admin client renders its
// product form against. Because the schema is served, not compiled into clients,
// adding an attribute to a category changes the form on the web admin and the
// phone app simultaneously with no client release — the single most important
// requirement of the system. Clients cache by Version and refetch when it bumps.
type FormSchema struct {
	CategoryID uuid.UUID   `json:"categoryId"`
	Version    int         `json:"version"`
	Fields     []FormField `json:"fields"`
}

// FormField describes one input in the generated form. It is derived from an
// Attribute joined with its CategoryAttribute binding (required/variant-axis/
// label override), so the ordering and requiredness are per-category.
type FormField struct {
	Key           string       `json:"key"`
	Label         string       `json:"label"`
	InputType     string       `json:"inputType"`
	DataType      DataType     `json:"dataType"`
	Unit          *string      `json:"unit,omitempty"`
	Required      bool         `json:"required"`
	IsVariantAxis bool         `json:"isVariantAxis"`
	HelpText      *string      `json:"helpText,omitempty"`
	Options       []FormOption `json:"options,omitempty"`
	Min           *float64     `json:"min,omitempty"`
	Max           *float64     `json:"max,omitempty"`
	Position      int          `json:"position"`
}

// FormOption is a selectable choice for enum-like fields, carrying the swatch so
// colour pickers render chips without a second request.
type FormOption struct {
	Value     string  `json:"value"`
	Label     string  `json:"label"`
	SwatchHex *string `json:"swatchHex,omitempty"`
	Position  int     `json:"position"`
}

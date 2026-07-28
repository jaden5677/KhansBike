package domain

import "github.com/google/uuid"

// Brand is a manufacturer/marque a product belongs to (Kenda, CST, ...). Brand
// names in the source data carry trailing-whitespace noise; the importer
// normalises them so "Kenda " and "Kenda" collapse to one brand. Brand is
// public information, unlike Supplier.
type Brand struct {
	ID          uuid.UUID
	Name        string
	Slug        string
	LogoAssetID *uuid.UUID
	Position    int
}

// Supplier is the trade source of a variant. It is ADMIN-ONLY and competitively
// sensitive: no public response type may ever carry a Supplier or its fields.
// It lives in the domain purely so admin flows and the importer can reference it.
type Supplier struct {
	ID    uuid.UUID
	Name  string
	Code  *string
	Notes *string
}

package models

// Trait interfaces allow models to opt-in to common behaviors.
// Models implementing these interfaces can use generic helper functions
// and be handled uniformly by repository and hook implementations.

// Sluggable indicates a model has a URL slug field.
type Sluggable interface {
	GetSlug() string
}

// GeoLocatable indicates a model has latitude/longitude coordinates.
type GeoLocatable interface {
	GetLatitude() *float64
	GetLongitude() *float64
}

// Nameable indicates a model has a searchable name field.
type Nameable interface {
	GetName() string
}

// Auditable indicates a model supports audit logging.
// Models implementing this interface will have their lifecycle events logged.
type Auditable interface {
	GetID() string
	TableName() string
}

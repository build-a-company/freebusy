package types

import (
	"github.com/oh-tarnished/runtime-go/ulid"
	"github.com/the-protobuf-project/resourcename"
)

// Resource-name helpers (Google AIP-122). The freebusy database stores bare ids
// as primary keys, while the protobuf API addresses records by hierarchical
// resource names such as "promoCodes/{promo_code}". These helpers convert between
// the two forms using github.com/the-protobuf-project/resourcename, so callers
// never hand-assemble or hand-parse name strings.
//
// Each helper struct carries its template on a blank field (detected by the
// braces in its tag) and exposes the id segment(s) as tagged value fields.

type promoCodeName struct {
	_  struct{} `resource:"promoCodes/{promo_code}"`
	ID string   `resource:"promo_code"`
}

type propertyName struct {
	_  struct{} `resource:"properties/{property}"`
	ID string   `resource:"property"`
}

type unitName struct {
	_        struct{} `resource:"properties/{property}/units/{unit}"`
	Property string   `resource:"property"`
	Unit     string   `resource:"unit"`
}

// resourceName is the RFC 9073 VRESOURCE name, "resources/{resource}".
//
// Flat, because the RFC resource is flat: section 7.3 defines the component's
// properties and says nothing about where it sits, so the catalogue is not
// nested under a site the way freebusy's own Unit was under a Property.
type resourceName struct {
	_        struct{} `resource:"resources/{resource}"`
	Resource string   `resource:"resource"`
}

// ParseResource extracts the id from an RFC 9073 resource name,
// "resources/{resource}".
//
// It replaces ParseUnitParent on every booking path: a booking now names a
// resource another service owns, so there is no parent property segment to
// return and nothing local to resolve it against.
func ParseResource(name string) (resourceID string, err error) {
	var n resourceName
	if err = resourcename.UnmarshalResource(name, &n); err != nil {
		return "", err
	}
	return n.Resource, nil
}

// PromoCodeName builds the resource name "promoCodes/{id}" from a bare id.
func PromoCodeName(id string) (string, error) {
	return resourcename.MarshalResource(&promoCodeName{ID: id})
}

// PromoCodeID extracts the bare id from a "promoCodes/{id}" resource name.
func PromoCodeID(name string) (string, error) {
	var n promoCodeName
	if err := resourcename.UnmarshalResource(name, &n); err != nil {
		return "", err
	}
	return n.ID, nil
}

// ResolvePromoCodeName returns the bare id and full resource name for a write.
// When name is empty a fresh ULID id is generated and formatted into a name;
// otherwise the id is parsed out of the supplied name. Both adapters use it so
// id generation and name parsing stay identical across providers.
func ResolvePromoCodeName(name string) (id, full string, err error) {
	if name == "" {
		id = ulid.GenerateString()
		full, err = PromoCodeName(id)
		return id, full, err
	}
	if id, err = PromoCodeID(name); err != nil {
		return "", "", err
	}
	return id, name, nil
}

// PropertyName builds the resource name "properties/{id}" from a bare id.
func PropertyName(id string) (string, error) {
	return resourcename.MarshalResource(&propertyName{ID: id})
}

// PropertyID extracts the bare id from a "properties/{id}" resource name.
func PropertyID(name string) (string, error) {
	var n propertyName
	if err := resourcename.UnmarshalResource(name, &n); err != nil {
		return "", err
	}
	return n.ID, nil
}

// ResolvePropertyName returns the bare id and full resource name for a write.
// When name is empty a fresh ULID id is generated; otherwise the id is parsed
// out of the supplied name.
func ResolvePropertyName(name string) (id, full string, err error) {
	if name == "" {
		id = ulid.GenerateString()
		full, err = PropertyName(id)
		return id, full, err
	}
	if id, err = PropertyID(name); err != nil {
		return "", "", err
	}
	return id, name, nil
}

// UnitName builds "properties/{property}/units/{unit}" from bare ids.
func UnitName(propertyID, unitID string) (string, error) {
	return resourcename.MarshalResource(&unitName{Property: propertyID, Unit: unitID})
}

// UnitID extracts the unit id segment from a
// "properties/{property}/units/{unit}" resource name.
func UnitID(name string) (string, error) {
	var n unitName
	if err := resourcename.UnmarshalResource(name, &n); err != nil {
		return "", err
	}
	return n.Unit, nil
}

// ParseUnitParent extracts both the property and unit ids from a unit resource
// name ("properties/{property}/units/{unit}").
func ParseUnitParent(name string) (propertyID, unitID string, err error) {
	var n unitName
	if err = resourcename.UnmarshalResource(name, &n); err != nil {
		return "", "", err
	}
	return n.Property, n.Unit, nil
}

// ResolveUnitName returns the parent property id, unit id, and full unit
// resource name for a write. When name is set it is parsed; otherwise a fresh
// ULID unit id is minted under the property parsed from parent
// ("properties/{property}"). Both adapters use it so id generation and name
// parsing stay identical across providers.
func ResolveUnitName(parent, name string) (propertyID, unitID, full string, err error) {
	if name != "" {
		var n unitName
		if err = resourcename.UnmarshalResource(name, &n); err != nil {
			return "", "", "", err
		}
		return n.Property, n.Unit, name, nil
	}
	if propertyID, err = PropertyID(parent); err != nil {
		return "", "", "", err
	}
	unitID = ulid.GenerateString()
	full, err = UnitName(propertyID, unitID)
	return propertyID, unitID, full, err
}

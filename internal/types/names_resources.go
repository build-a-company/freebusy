// Name codecs for licences, organisations, bookings, users, schedules, and availability exceptions.
package types

import (
	"github.com/oh-tarnished/runtime-go/ulid"
	"github.com/the-protobuf-project/resourcename"
)

type licenceName struct {
	_        struct{} `resource:"properties/{property}/licences/{licence}"`
	Property string   `resource:"property"`
	Licence  string   `resource:"licence"`
}

type organisationName struct {
	_  struct{} `resource:"organisations/{organisation}"`
	ID string   `resource:"organisation"`
}

type bookingName struct {
	_  struct{} `resource:"bookings/{booking}"`
	ID string   `resource:"booking"`
}

type userName struct {
	_  struct{} `resource:"users/{user}"`
	ID string   `resource:"user"`
}

type scheduleName struct {
	_        struct{} `resource:"resources/{resource}/schedule"`
	Resource string   `resource:"resource"`
}

type availabilityExceptionName struct {
	_         struct{} `resource:"resources/{resource}/availabilityExceptions/{availability_exception}"`
	Resource  string   `resource:"resource"`
	Exception string   `resource:"availability_exception"`
}

// LicenceName builds "properties/{property}/licences/{licence}" from bare ids.
func LicenceName(propertyID, licenceID string) (string, error) {
	return resourcename.MarshalResource(&licenceName{Property: propertyID, Licence: licenceID})
}

// LicenceID extracts the licence id segment from a
// "properties/{property}/licences/{licence}" resource name.
func LicenceID(name string) (string, error) {
	var n licenceName
	if err := resourcename.UnmarshalResource(name, &n); err != nil {
		return "", err
	}
	return n.Licence, nil
}

// ResolveLicenceName returns the parent property id, licence id, and full
// licence resource name for a write. When name is set it is parsed; otherwise
// a fresh ULID licence id is minted under the property parsed from parent
// ("properties/{property}").
func ResolveLicenceName(parent, name string) (propertyID, licenceID, full string, err error) {
	if name != "" {
		var n licenceName
		if err = resourcename.UnmarshalResource(name, &n); err != nil {
			return "", "", "", err
		}
		return n.Property, n.Licence, name, nil
	}
	if propertyID, err = PropertyID(parent); err != nil {
		return "", "", "", err
	}
	licenceID = ulid.GenerateString()
	full, err = LicenceName(propertyID, licenceID)
	return propertyID, licenceID, full, err
}

// OrganisationName builds the resource name "organisations/{id}" from a bare id.
func OrganisationName(id string) (string, error) {
	return resourcename.MarshalResource(&organisationName{ID: id})
}

// OrganisationID extracts the bare id from an "organisations/{id}" resource name.
func OrganisationID(name string) (string, error) {
	var n organisationName
	if err := resourcename.UnmarshalResource(name, &n); err != nil {
		return "", err
	}
	return n.ID, nil
}

// BookingName builds the resource name "bookings/{id}" from a bare id.
func BookingName(id string) (string, error) {
	return resourcename.MarshalResource(&bookingName{ID: id})
}

// BookingID extracts the bare id from a "bookings/{id}" resource name.
func BookingID(name string) (string, error) {
	var n bookingName
	if err := resourcename.UnmarshalResource(name, &n); err != nil {
		return "", err
	}
	return n.ID, nil
}

// ResolveBookingName returns the bare id and full resource name for a write.
func ResolveBookingName(name string) (id, full string, err error) {
	if name == "" {
		id = ulid.GenerateString()
		full, err = BookingName(id)
		return id, full, err
	}
	if id, err = BookingID(name); err != nil {
		return "", "", err
	}
	return id, name, nil
}

// UserName builds the resource name "users/{id}" from a bare id.
func UserName(id string) (string, error) {
	return resourcename.MarshalResource(&userName{ID: id})
}

// ScheduleName builds "resources/{resource}/schedule".
//
// A schedule hangs off the bookable resource directly now that Unit is an RFC
// 9073 VRESOURCE: there is no property segment left to nest it under, and
// AIP-123 requires every parent segment name a resource that actually exists.
func ScheduleName(resourceID string) (string, error) {
	return resourcename.MarshalResource(&scheduleName{Resource: resourceID})
}

// ParseScheduleName extracts the parent resource id from a schedule name.
func ParseScheduleName(name string) (resourceID string, err error) {
	var n scheduleName
	if err = resourcename.UnmarshalResource(name, &n); err != nil {
		return "", err
	}
	return n.Resource, nil
}

// AvailabilityExceptionName builds the full exception resource name from bare ids.
func AvailabilityExceptionName(resourceID, exceptionID string) (string, error) {
	return resourcename.MarshalResource(&availabilityExceptionName{Resource: resourceID, Exception: exceptionID})
}

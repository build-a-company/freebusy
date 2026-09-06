package rfc

import (
	"context"
	"testing"

	resourcev1 "github.com/oh-tarnished/freebusy/protobuf/generated/go/protobuf/rfc9073/resource/v1"
)

// A resource carrying freebusy's STRUCTURED-DATA yields the profile in it.
func TestDecodeProfile(t *testing.T) {
	res := &resourcev1.Resource{
		Name: "resources/deluxe-king",
		StructuredData: []*resourcev1.StructuredData{
			{Schema: "https://example.com/other", Value: &resourcev1.StructuredData_Text{Text: `{"max_occupancy":99}`}},
			{Schema: ProfileSchemaURI, Value: &resourcev1.StructuredData_Text{Text: `{"max_occupancy":4}`}},
		},
	}
	p, err := DecodeProfile(res)
	if err != nil {
		t.Fatalf("DecodeProfile: %v", err)
	}
	if p.MaxOccupancy != 4 {
		t.Fatalf("MaxOccupancy = %d, want 4 (the entry matching our schema, not the first one)", p.MaxOccupancy)
	}
}

// A resource with no structured data at all is ordinary, not an error: most
// resources will never carry a booking profile.
func TestDecodeProfileAbsent(t *testing.T) {
	p, err := DecodeProfile(&resourcev1.Resource{Name: "resources/bay-1"})
	if err != nil {
		t.Fatalf("DecodeProfile on a bare resource: %v", err)
	}
	if p.MaxOccupancy != 0 {
		t.Fatalf("MaxOccupancy = %d, want 0 (unbounded)", p.MaxOccupancy)
	}
}

// Malformed JSON under our own schema URI is a real error — it means someone
// wrote a profile and got it wrong, which is worth reporting rather than
// silently reading as unbounded.
func TestDecodeProfileMalformed(t *testing.T) {
	res := &resourcev1.Resource{
		Name:           "resources/bay-2",
		StructuredData: []*resourcev1.StructuredData{{Schema: ProfileSchemaURI, Value: &resourcev1.StructuredData_Text{Text: `{"max_occupancy":`}}},
	}
	if _, err := DecodeProfile(res); err == nil {
		t.Fatal("DecodeProfile on malformed JSON = nil error, want a decode failure")
	}
}

// A uri-valued payload is deliberately not followed: fetching an arbitrary URL
// on the booking hot path is not something a decode should do silently.
func TestDecodeProfileIgnoresURIForm(t *testing.T) {
	res := &resourcev1.Resource{
		Name:           "resources/bay-3",
		StructuredData: []*resourcev1.StructuredData{{Schema: ProfileSchemaURI, Value: &resourcev1.StructuredData_Uri{Uri: "https://example.com/profile.json"}}},
	}
	p, err := DecodeProfile(res)
	if err != nil {
		t.Fatalf("DecodeProfile: %v", err)
	}
	if p.MaxOccupancy != 0 {
		t.Fatalf("MaxOccupancy = %d, want 0 — a uri payload must not be followed", p.MaxOccupancy)
	}
}

// A nil Client is a supported configuration, not a crash: it is what "no
// catalogue configured" resolves to, and the booking path relies on it.
func TestNilClientIsUnavailable(t *testing.T) {
	var c *Client
	if _, err := c.Resource(context.Background(), "resources/x"); err != ErrUnavailable {
		t.Fatalf("Resource on nil client = %v, want ErrUnavailable", err)
	}
	if _, err := c.TimeZone(context.Background(), &resourcev1.Resource{}); err != ErrUnavailable {
		t.Fatalf("TimeZone on nil client = %v, want ErrUnavailable", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close on nil client = %v, want nil", err)
	}
}

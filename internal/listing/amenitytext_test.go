package listing

import "testing"

func TestAmenitiesFromText(t *testing.T) {
	cases := []struct {
		text string
		want []Amenity
	}{
		{"Spacious kitchen with gas stove, Dishwasher and plenty of cabinets.", []Amenity{AmenityDishwasher}},
		{"In-unit laundry, central air, and one off-street parking space.", []Amenity{AmenityInUnitLaundry, AmenityAirConditioning, AmenityParking}},
		{"Sorry, no dishwasher. Laundry in basement. Street parking only.", nil},
		{"Unit is without air conditioning but has a dishwasher.", []Amenity{AmenityDishwasher}},
		{"Washer/dryer in unit!", []Amenity{AmenityInUnitLaundry}},
	}
	for _, c := range cases {
		got := AmenitiesFromText(c.text)
		if len(got) != len(c.want) {
			t.Errorf("%q: got %v, want %v", c.text, got, c.want)
			continue
		}
		for _, a := range c.want {
			if !got[a] {
				t.Errorf("%q: missing %s in %v", c.text, a, got)
			}
		}
	}
}

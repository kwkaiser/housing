package listing

import "testing"

func TestShortLeaseText(t *testing.T) {
	cases := map[string]bool{
		"12 Months":         false,
		"1 Month":           true,
		"6 months":          true,
		"Short term":        true,
		"2 weeks":           true,
		"":                  false,
		"Flexible":          false,
		"Month-to-month":    true,
		"12 mo lease":       false,
		"3 mo minimum stay": true,
	}
	for in, want := range cases {
		if got := ShortLeaseText(in); got != want {
			t.Errorf("ShortLeaseText(%q) = %v, want %v", in, got, want)
		}
	}
}

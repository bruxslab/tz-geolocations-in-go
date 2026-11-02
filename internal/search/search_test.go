package search

import "testing"

func TestSafeInputs(t *testing.T) {
	if Sanitize("\ufeff--\ufeff") != "" {
		t.Fatal("JavaScript whitespace must trim BOM")
	}
	for _, tc := range []struct{ in, want string }{{"  Buguruni! ", "Buguruni"}, {"' OR 1=1; --", "OR 11"}, {"Ng'ombe", "Ngombe"}, {"é", ""}, {"dar-es-salaam", "daressalaam"}} {
		if got := Sanitize(tc.in); got != tc.want {
			t.Errorf("%q: %q != %q", tc.in, got, tc.want)
		}
	}
	if got := Prefix(Sanitize("Dar es Sa")); got != "'Dar' <-> 'es' <-> 'Sa':*" {
		t.Fatal(got)
	}
	if UTF16Length("😀") != 2 {
		t.Fatal("UTF-16 compatibility")
	}
}

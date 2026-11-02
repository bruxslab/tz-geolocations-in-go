package database

import "testing"

func TestTestDatabaseGuard(t *testing.T) {
	for _, url := range []string{"postgres://u:p@localhost/tz_locations", "postgres://u:p@localhost/postgres", "postgres://u:p@localhost/tz_locations_test?search_path=public", "%%%"} {
		if RequireTestDatabase(url) == nil {
			t.Errorf("unsafe URL accepted: %s", url)
		}
	}
	if err := RequireTestDatabase("postgres://u:p@localhost/tz_locations_test"); err != nil {
		t.Fatal(err)
	}
}

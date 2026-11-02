package service

import (
	"errors"
	"testing"
)

func TestPagination(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
	}{{"", 10}, {"0", 10}, {"garbage", 10}, {"-3", 1}, {"999", 50}, {"1e1", 10}, {"0x2", 2}, {"Infinity", 50}} {
		got, err := Pagination(tc.in, 10, 50)
		if err != nil || got != tc.want {
			t.Errorf("%q: %d %v", tc.in, got, err)
		}
	}
	if _, err := Pagination("1.5", 10, 50); !errors.Is(err, ErrFractional) {
		t.Fatal(err)
	}
	if _, err := Offset("Infinity"); err == nil {
		t.Fatal("unbounded offset")
	}
}

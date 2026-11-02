package service

import (
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var SlugPattern = regexp.MustCompile(`^[a-z0-9-]{1,100}$`)
var PostcodePattern = regexp.MustCompile(`^[0-9]{5}$`)
var ErrFractional = errors.New("fractional pagination is not supported by the reference database")

// Number follows JavaScript Number for the common HTTP numeric forms, including
// zero/NaN defaulting, exponent notation, and unsigned hex/binary/octal literals.
func Number(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if len(raw) > 2 && raw[0] == '0' && strings.ContainsAny(raw[1:2], "xXbBoO") {
		v, e := strconv.ParseUint(raw, 0, 64)
		if e == nil {
			return float64(v)
		}
		return math.NaN()
	}
	v, e := strconv.ParseFloat(raw, 64)
	if e != nil && !math.IsInf(v, 0) {
		return math.NaN()
	}
	return v
}
func Pagination(raw string, def, max int) (int, error) {
	n := Number(raw)
	if math.IsNaN(n) || n == 0 {
		n = float64(def)
	}
	if n < 1 {
		n = 1
	}
	if n > float64(max) {
		n = float64(max)
	}
	if math.Trunc(n) != n {
		return 0, ErrFractional
	}
	return int(n), nil
}
func Offset(raw string) (int, error) {
	n := Number(raw)
	if math.IsNaN(n) || n < 0 {
		n = 0
	}
	if math.IsInf(n, 0) || n > 9007199254740991 {
		return 0, errors.New("offset exceeds safe integer range")
	}
	if math.Trunc(n) != n {
		return 0, ErrFractional
	}
	return int(n), nil
}

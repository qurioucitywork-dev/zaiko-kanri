package persistence

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

var ErrInvalidProductWeight = errors.New("invalid product weight")

func normalizeProductWeight(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		return "", ErrInvalidProductWeight
	}
	return strconv.FormatFloat(n, 'f', -1, 64), nil
}

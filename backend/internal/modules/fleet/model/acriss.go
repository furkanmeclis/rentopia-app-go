package model

import (
	"fmt"
	"strings"
)

// ACRISS (SIPP) code matrix: four letters, one per position. A group's ACRISS
// code is a classification hint for channels; it is never a guarantee.
var (
	// ACRISSCategories is position 1 (vehicle category).
	ACRISSCategories = "MNEHCDIJSRFGPULWOX"
	// ACRISSTypes is position 2 (vehicle type / body).
	ACRISSTypes = "BCDWVLSTFJXPQZEMRHYNGK"
	// ACRISSTransmissions is position 3 (transmission / drive).
	ACRISSTransmissions = "MNCABD"
	// ACRISSFuelAir is position 4 (fuel / air conditioning).
	ACRISSFuelAir = "RNDQHIECLSABMFVZUX"
)

// NormalizeACRISS upper-cases and trims a code.
func NormalizeACRISS(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// ValidateACRISS checks a (normalized) ACRISS code against the matrix and
// returns which position is invalid.
func ValidateACRISS(code string) error {
	if len(code) != 4 {
		return fmt.Errorf("acriss code must have 4 letters")
	}
	positions := []struct {
		name, allowed string
	}{
		{"category", ACRISSCategories},
		{"type", ACRISSTypes},
		{"transmission", ACRISSTransmissions},
		{"fuel/air", ACRISSFuelAir},
	}
	for i, p := range positions {
		c := code[i]
		if c < 'A' || c > 'Z' || !strings.ContainsRune(p.allowed, rune(c)) {
			return fmt.Errorf("acriss letter %d (%s) %q is not valid", i+1, p.name, string(c))
		}
	}
	return nil
}

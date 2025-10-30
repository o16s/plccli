package main

import (
	"fmt"
)

// BitValue represents a single bit extracted from a value
type BitValue struct {
	BitNum int    // Bit position (0-31)
	Value  int    // Bit value (0 or 1)
	Name   string // Human-readable name for this bit
}

// getBitValue extracts a single bit from a uint32 value
// bitNum: 0 (LSB) to 31 (MSB)
// Returns: 0 or 1
func getBitValue(value uint32, bitNum int) int {
	if bitNum < 0 || bitNum > 31 {
		return 0 // Invalid bit number
	}
	return int((value >> bitNum) & 1)
}

// validateBitNames validates that bit names are either:
// - nil or empty (will use defaults)
// - exactly 32 names
//
// Returns error if not exactly 32 names (when provided)
func validateBitNames(names []string) error {
	if names == nil || len(names) == 0 {
		return nil // Will use default names
	}

	if len(names) != 32 {
		return fmt.Errorf("bit names must be exactly 32 (got %d). Provide all 32 bit names or none at all", len(names))
	}

	return nil
}

// extractBits extracts all 32 bits (0-31) from a uint32 value
// value: the uint32 value to extract bits from
// bitNames: optional slice of exactly 32 bit names (or nil for defaults)
//
// Returns: slice of 32 BitValue structs, one for each bit
func extractBits(value uint32, bitNames []string) ([]BitValue, error) {
	// Validate bit names first
	if err := validateBitNames(bitNames); err != nil {
		return nil, err
	}

	// Extract all 32 bits (0-31)
	results := make([]BitValue, 32)
	for bitNum := 0; bitNum < 32; bitNum++ {
		bitValue := getBitValue(value, bitNum)

		// Determine bit name
		var bitName string
		if bitNames != nil && len(bitNames) == 32 {
			bitName = bitNames[bitNum]
		} else {
			bitName = fmt.Sprintf("bit_%d", bitNum)
		}

		results[bitNum] = BitValue{
			BitNum: bitNum,
			Value:  bitValue,
			Name:   bitName,
		}
	}

	return results, nil
}

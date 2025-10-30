package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetBitValue tests extracting a single bit from a uint32 value
// This is the CORE function - must be 100% correct
func TestGetBitValue(t *testing.T) {
	tests := []struct {
		name     string
		value    uint32
		bitNum   int
		expected int
	}{
		// All zeros
		{name: "all zeros - bit 0", value: 0x00000000, bitNum: 0, expected: 0},
		{name: "all zeros - bit 15", value: 0x00000000, bitNum: 15, expected: 0},
		{name: "all zeros - bit 31", value: 0x00000000, bitNum: 31, expected: 0},

		// All ones
		{name: "all ones - bit 0", value: 0xFFFFFFFF, bitNum: 0, expected: 1},
		{name: "all ones - bit 15", value: 0xFFFFFFFF, bitNum: 15, expected: 1},
		{name: "all ones - bit 31", value: 0xFFFFFFFF, bitNum: 31, expected: 1},

		// Single bit set
		{name: "bit 0 only", value: 0x00000001, bitNum: 0, expected: 1},
		{name: "bit 0 only - check bit 1", value: 0x00000001, bitNum: 1, expected: 0},
		{name: "bit 1 only", value: 0x00000002, bitNum: 1, expected: 1},
		{name: "bit 7 only", value: 0x00000080, bitNum: 7, expected: 1},
		{name: "bit 31 only", value: 0x80000000, bitNum: 31, expected: 1},
		{name: "bit 31 only - check bit 30", value: 0x80000000, bitNum: 30, expected: 0},

		// PRODUCTION VALUE: 134217856 = 0x08000080 = bits 7 and 27 set
		{name: "production value - bit 0", value: 134217856, bitNum: 0, expected: 0},
		{name: "production value - bit 7", value: 134217856, bitNum: 7, expected: 1},
		{name: "production value - bit 27", value: 134217856, bitNum: 27, expected: 1},
		{name: "production value - bit 31", value: 134217856, bitNum: 31, expected: 0},
		{name: "production value - bit 8", value: 134217856, bitNum: 8, expected: 0},
		{name: "production value - bit 26", value: 134217856, bitNum: 26, expected: 0},

		// Alternating patterns
		{name: "0xAAAAAAAA - even bit 0", value: 0xAAAAAAAA, bitNum: 0, expected: 0},
		{name: "0xAAAAAAAA - odd bit 1", value: 0xAAAAAAAA, bitNum: 1, expected: 1},
		{name: "0xAAAAAAAA - even bit 30", value: 0xAAAAAAAA, bitNum: 30, expected: 0},
		{name: "0xAAAAAAAA - odd bit 31", value: 0xAAAAAAAA, bitNum: 31, expected: 1},

		{name: "0x55555555 - even bit 0", value: 0x55555555, bitNum: 0, expected: 1},
		{name: "0x55555555 - odd bit 1", value: 0x55555555, bitNum: 1, expected: 0},
		{name: "0x55555555 - even bit 30", value: 0x55555555, bitNum: 30, expected: 1},
		{name: "0x55555555 - odd bit 31", value: 0x55555555, bitNum: 31, expected: 0},

		// Multiple bits
		{name: "0x00000003 - bit 0", value: 0x00000003, bitNum: 0, expected: 1},
		{name: "0x00000003 - bit 1", value: 0x00000003, bitNum: 1, expected: 1},
		{name: "0x00000003 - bit 2", value: 0x00000003, bitNum: 2, expected: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getBitValue(tt.value, tt.bitNum)
			assert.Equal(t, tt.expected, result,
				"getBitValue(0x%08X, %d) should return %d, got %d",
				tt.value, tt.bitNum, tt.expected, result)
		})
	}
}

// TestGetBitValue_EdgeCases tests invalid bit numbers for 100% coverage
func TestGetBitValue_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		value    uint32
		bitNum   int
		expected int
	}{
		{name: "bit number -1 (too low)", value: 0xFFFFFFFF, bitNum: -1, expected: 0},
		{name: "bit number 32 (too high)", value: 0xFFFFFFFF, bitNum: 32, expected: 0},
		{name: "bit number 100 (way too high)", value: 0xFFFFFFFF, bitNum: 100, expected: 0},
		{name: "bit number -100 (way too low)", value: 0xFFFFFFFF, bitNum: -100, expected: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getBitValue(tt.value, tt.bitNum)
			assert.Equal(t, tt.expected, result,
				"getBitValue with invalid bit number %d should return 0", tt.bitNum)
		})
	}
}

// TestValidateBitNames tests validation of bit names
// CRITICAL: Must be exactly 32 names or error
func TestValidateBitNames(t *testing.T) {
	tests := []struct {
		name    string
		names   []string
		wantErr bool
	}{
		{
			name: "exactly 32 names - PASS",
			names: []string{
				"bit0", "bit1", "bit2", "bit3", "bit4", "bit5", "bit6", "bit7",
				"bit8", "bit9", "bit10", "bit11", "bit12", "bit13", "bit14", "bit15",
				"bit16", "bit17", "bit18", "bit19", "bit20", "bit21", "bit22", "bit23",
				"bit24", "bit25", "bit26", "bit27", "bit28", "bit29", "bit30", "bit31",
			},
			wantErr: false,
		},
		{
			name:    "nil names (will use defaults) - PASS",
			names:   nil,
			wantErr: false,
		},
		{
			name:    "empty slice (will use defaults) - PASS",
			names:   []string{},
			wantErr: false,
		},
		{
			name: "31 names - FAIL",
			names: []string{
				"bit0", "bit1", "bit2", "bit3", "bit4", "bit5", "bit6", "bit7",
				"bit8", "bit9", "bit10", "bit11", "bit12", "bit13", "bit14", "bit15",
				"bit16", "bit17", "bit18", "bit19", "bit20", "bit21", "bit22", "bit23",
				"bit24", "bit25", "bit26", "bit27", "bit28", "bit29", "bit30",
			},
			wantErr: true,
		},
		{
			name: "33 names - FAIL",
			names: []string{
				"bit0", "bit1", "bit2", "bit3", "bit4", "bit5", "bit6", "bit7",
				"bit8", "bit9", "bit10", "bit11", "bit12", "bit13", "bit14", "bit15",
				"bit16", "bit17", "bit18", "bit19", "bit20", "bit21", "bit22", "bit23",
				"bit24", "bit25", "bit26", "bit27", "bit28", "bit29", "bit30", "bit31",
				"bit32",
			},
			wantErr: true,
		},
		{
			name:    "1 name - FAIL",
			names:   []string{"motor_fault"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBitNames(tt.names)

			if tt.wantErr {
				assert.Error(t, err,
					"validateBitNames with %d names should return error", len(tt.names))
			} else {
				assert.NoError(t, err,
					"validateBitNames with %d names should not return error", len(tt.names))
			}
		})
	}
}

// TestExtractBits tests the full bit extraction pipeline
func TestExtractBits(t *testing.T) {
	tests := []struct {
		name     string
		value    uint32
		bitNames []string
		wantErr  bool
		validate func(t *testing.T, results []BitValue)
	}{
		{
			name:     "production value - all 32 bits",
			value:    134217856, // 0x08000080
			bitNames: nil,
			wantErr:  false,
			validate: func(t *testing.T, results []BitValue) {
				require.Len(t, results, 32, "should have 32 results")

				// Check bit 7 is HIGH
				assert.Equal(t, 7, results[7].BitNum)
				assert.Equal(t, 1, results[7].Value)
				assert.Equal(t, "bit_7", results[7].Name)

				// Check bit 27 is HIGH
				assert.Equal(t, 27, results[27].BitNum)
				assert.Equal(t, 1, results[27].Value)
				assert.Equal(t, "bit_27", results[27].Name)

				// Check bit 0 is LOW
				assert.Equal(t, 0, results[0].BitNum)
				assert.Equal(t, 0, results[0].Value)

				// Check bit 31 is LOW
				assert.Equal(t, 31, results[31].BitNum)
				assert.Equal(t, 0, results[31].Value)
			},
		},
		{
			name:     "all zeros - check all bits",
			value:    0x00000000,
			bitNames: nil,
			wantErr:  false,
			validate: func(t *testing.T, results []BitValue) {
				require.Len(t, results, 32, "should have 32 results")
				for i, result := range results {
					assert.Equal(t, i, result.BitNum, "bit number should match index")
					assert.Equal(t, 0, result.Value, "all bits should be 0")
				}
			},
		},
		{
			name:     "all ones - check all bits",
			value:    0xFFFFFFFF,
			bitNames: nil,
			wantErr:  false,
			validate: func(t *testing.T, results []BitValue) {
				require.Len(t, results, 32, "should have 32 results")
				for i, result := range results {
					assert.Equal(t, i, result.BitNum, "bit number should match index")
					assert.Equal(t, 1, result.Value, "all bits should be 1")
				}
			},
		},
		{
			name:  "with custom bit names",
			value: 0x00000080, // bit 7 set
			bitNames: []string{
				"bit0", "bit1", "bit2", "bit3", "bit4", "bit5", "bit6", "motor_fault",
				"bit8", "bit9", "bit10", "bit11", "bit12", "bit13", "bit14", "bit15",
				"bit16", "bit17", "bit18", "bit19", "bit20", "bit21", "bit22", "bit23",
				"bit24", "bit25", "bit26", "bit27", "bit28", "bit29", "bit30", "bit31",
			},
			wantErr: false,
			validate: func(t *testing.T, results []BitValue) {
				require.Len(t, results, 32, "should have 32 results")

				// Check bit 7 has custom name
				assert.Equal(t, 7, results[7].BitNum)
				assert.Equal(t, 1, results[7].Value)
				assert.Equal(t, "motor_fault", results[7].Name)

				// Check other bits have custom names too
				assert.Equal(t, "bit0", results[0].Name)
				assert.Equal(t, "bit31", results[31].Name)
			},
		},
		{
			name:     "wrong number of bit names",
			value:    0x00000001,
			bitNames: []string{"only_one_name"},
			wantErr:  true,
			validate: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := extractBits(tt.value, tt.bitNames)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, results)
				if tt.validate != nil {
					tt.validate(t, results)
				}
			}
		})
	}
}

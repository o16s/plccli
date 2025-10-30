package main

import (
	"strconv"
	"testing"

	"github.com/gopcua/opcua/ua"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBooleanParsing tests the strconv.ParseBool function that's used in service.go:744
// This tests the first step of the boolean write path
func TestBooleanParsing(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantBool  bool
		wantError bool
	}{
		{
			name:      "lowercase true",
			value:     "true",
			wantBool:  true,
			wantError: false,
		},
		{
			name:      "lowercase false",
			value:     "false",
			wantBool:  false,
			wantError: false,
		},
		{
			name:      "uppercase TRUE",
			value:     "TRUE",
			wantBool:  true,
			wantError: false,
		},
		{
			name:      "uppercase FALSE",
			value:     "FALSE",
			wantBool:  false,
			wantError: false,
		},
		{
			name:      "1 as true",
			value:     "1",
			wantBool:  true,
			wantError: false,
		},
		{
			name:      "0 as false",
			value:     "0",
			wantBool:  false,
			wantError: false,
		},
		{
			name:      "invalid value",
			value:     "not-a-bool",
			wantError: true,
		},
		{
			name:      "empty string",
			value:     "",
			wantError: true,
		},
		{
			name:      "number 2",
			value:     "2",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This replicates the exact logic from service.go:744
			boolValue, err := strconv.ParseBool(tt.value)

			if tt.wantError {
				assert.Error(t, err, "Expected ParseBool to return an error")
			} else {
				require.NoError(t, err, "ParseBool should not return an error")
				assert.Equal(t, tt.wantBool, boolValue, "Boolean value mismatch")
			}
		})
	}
}

// TestBooleanVariantCreation tests creating a UA variant with a boolean value
// This tests the second step of the boolean write path (service.go:752)
func TestBooleanVariantCreation(t *testing.T) {
	tests := []struct {
		name      string
		boolValue bool
	}{
		{
			name:      "true value",
			boolValue: true,
		},
		{
			name:      "false value",
			boolValue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This replicates the exact logic from service.go:752
			variant, err := ua.NewVariant(tt.boolValue)

			require.NoError(t, err, "NewVariant should not fail for boolean")
			require.NotNil(t, variant, "Variant should not be nil")

			// Verify the variant contains the expected boolean value
			assert.Equal(t, tt.boolValue, variant.Value(), "Variant should contain the correct boolean value")

			// Verify the variant type is correct
			assert.IsType(t, true, variant.Value(), "Variant should contain a bool type")

			// Log the variant type for debugging
			t.Logf("Variant type: %T, value: %v", variant.Value(), variant.Value())
		})
	}
}

// TestBooleanWriteValueStructure tests that a boolean variant can be properly
// used in a WriteValue structure as done in service.go:884-894
func TestBooleanWriteValueStructure(t *testing.T) {
	tests := []struct {
		name      string
		boolValue bool
	}{
		{
			name:      "write true",
			boolValue: true,
		},
		{
			name:      "write false",
			boolValue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a boolean variant
			variant, err := ua.NewVariant(tt.boolValue)
			require.NoError(t, err)

			// Create a WriteValue structure as done in service.go:884-894
			nodeID := ua.NewNumericNodeID(3, 1000)
			writeValue := &ua.WriteValue{
				NodeID:      nodeID,
				AttributeID: ua.AttributeIDValue,
				Value: &ua.DataValue{
					EncodingMask: ua.DataValueValue,
					Value:        variant,
				},
			}

			// Verify structure is created correctly
			assert.NotNil(t, writeValue, "WriteValue should not be nil")
			assert.NotNil(t, writeValue.Value, "DataValue should not be nil")
			assert.NotNil(t, writeValue.Value.Value, "Variant should not be nil")
			assert.Equal(t, tt.boolValue, writeValue.Value.Value.Value(), "WriteValue should contain correct boolean")
			assert.Equal(t, ua.AttributeIDValue, writeValue.AttributeID, "AttributeID should be Value")

			// Log for debugging
			t.Logf("WriteValue structure: NodeID=%v, Value=%v, Type=%T",
				writeValue.NodeID,
				writeValue.Value.Value.Value(),
				writeValue.Value.Value.Value())
		})
	}
}

// TestBooleanEndToEndParsing tests the complete parsing chain for boolean writes
func TestBooleanEndToEndParsing(t *testing.T) {
	tests := []struct {
		name       string
		inputValue string
		wantBool   bool
	}{
		{
			name:       "string true to boolean true",
			inputValue: "true",
			wantBool:   true,
		},
		{
			name:       "string false to boolean false",
			inputValue: "false",
			wantBool:   false,
		},
		{
			name:       "string 1 to boolean true",
			inputValue: "1",
			wantBool:   true,
		},
		{
			name:       "string 0 to boolean false",
			inputValue: "0",
			wantBool:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Step 1: Parse the string (service.go:744)
			boolValue, err := strconv.ParseBool(tt.inputValue)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBool, boolValue)

			// Step 2: Create variant (service.go:752)
			variant, err := ua.NewVariant(boolValue)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBool, variant.Value())

			// Step 3: Create WriteValue structure (service.go:884-894)
			nodeID := ua.NewNumericNodeID(3, 1000)
			writeValue := &ua.WriteValue{
				NodeID:      nodeID,
				AttributeID: ua.AttributeIDValue,
				Value: &ua.DataValue{
					EncodingMask: ua.DataValueValue,
					Value:        variant,
				},
			}

			// Verify the complete chain
			assert.Equal(t, tt.wantBool, writeValue.Value.Value.Value())

			t.Logf("Successfully processed '%s' -> %v -> variant -> WriteValue",
				tt.inputValue, boolValue)
		})
	}
}

package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseNodeID(t *testing.T) {
	tests := []struct {
		name           string
		nodeID         string
		wantNamespace  string
		wantType       string
		wantIdentifier string
		wantErr        bool
	}{
		{
			name:           "semicolon format numeric",
			nodeID:         "ns=0;i=2258",
			wantNamespace:  "0",
			wantType:       "i",
			wantIdentifier: "2258",
			wantErr:        false,
		},
		{
			name:           "comma format numeric",
			nodeID:         "ns=0,i=2258",
			wantNamespace:  "0",
			wantType:       "i",
			wantIdentifier: "2258",
			wantErr:        false,
		},
		{
			name:           "semicolon format string",
			nodeID:         "ns=3;s=Temperature",
			wantNamespace:  "3",
			wantType:       "s",
			wantIdentifier: "Temperature",
			wantErr:        false,
		},
		{
			name:           "comma format string",
			nodeID:         "ns=3,s=Temperature",
			wantNamespace:  "3",
			wantType:       "s",
			wantIdentifier: "Temperature",
			wantErr:        false,
		},
		{
			name:           "complex string identifier",
			nodeID:         `ns=5;s="Root"."Objects"."Temperature"`,
			wantNamespace:  "5",
			wantType:       "s",
			wantIdentifier: `"Root"."Objects"."Temperature"`,
			wantErr:        false,
		},
		{
			name:    "invalid format - no separator",
			nodeID:  "invalid",
			wantErr: true,
		},
		{
			name:    "invalid format - missing namespace",
			nodeID:  "i=2258",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namespace, idType, identifier, err := parseNodeID(tt.nodeID)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantNamespace, namespace, "namespace mismatch")
				assert.Equal(t, tt.wantType, idType, "type mismatch")
				assert.Equal(t, tt.wantIdentifier, identifier, "identifier mismatch")
			}
		})
	}
}

func TestFormatInfluxOutput(t *testing.T) {
	tests := []struct {
		name        string
		measurement string
		nodeID      string
		value       interface{}
		dataType    string
		endpoint    string
		wantContain []string // Strings that should be in the output
	}{
		{
			name:        "boolean true",
			measurement: "opcua_node",
			nodeID:      "ns=3;s=BoolValue",
			value:       true,
			dataType:    "boolean",
			endpoint:    "opc.tcp://192.168.1.100:4840",
			wantContain: []string{
				"opcua_node",
				"node_id=ns\\=3;s\\=BoolValue", // semicolon is not escaped in tag values
				"value=1", // boolean true should be 1
				"endpoint=opc.tcp://192.168.1.100:4840",
			},
		},
		{
			name:        "boolean false",
			measurement: "opcua_node",
			nodeID:      "ns=3;s=BoolValue",
			value:       false,
			dataType:    "boolean",
			endpoint:    "opc.tcp://192.168.1.100:4840",
			wantContain: []string{
				"opcua_node",
				"value=0", // boolean false should be 0
			},
		},
		{
			name:        "integer value",
			measurement: "temperature",
			nodeID:      "ns=3;s=Temp",
			value:       42,
			dataType:    "int32",
			endpoint:    "opc.tcp://localhost:4840",
			wantContain: []string{
				"temperature",
				"value=42",
			},
		},
		{
			name:        "float value",
			measurement: "pressure",
			nodeID:      "ns=3;s=Press",
			value:       2.5,
			dataType:    "float",
			endpoint:    "opc.tcp://localhost:4840",
			wantContain: []string{
				"pressure",
				"value=2.5",
			},
		},
		{
			name:        "string value",
			measurement: "opcua_node",
			nodeID:      "ns=3;s=StringValue",
			value:       "test string",
			dataType:    "string",
			endpoint:    "opc.tcp://localhost:4840",
			wantContain: []string{
				"opcua_node",
				"value=1", // non-timestamp strings get value=1
				`string_value="test string"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := formatInfluxOutput(tt.measurement, tt.nodeID, tt.value, tt.dataType, tt.endpoint)

			// Check that all expected strings are present
			for _, want := range tt.wantContain {
				assert.Contains(t, output, want, "output should contain: %s", want)
			}

			// Verify it's in InfluxDB line protocol format (measurement,tags fields timestamp)
			assert.Contains(t, output, ",", "should have tags separated by comma")
			assert.Contains(t, output, " ", "should have space before fields")
		})
	}
}

// TestFormatInfluxOutputWithBits_ProductionValue tests the critical production use case
// This test validates the EXACT output format for the factory alarm monitoring system
func TestFormatInfluxOutputWithBits_ProductionValue(t *testing.T) {
	// Production value: 134217856 = 0x08000080 (bits 7 and 27 are HIGH)
	value := uint32(134217856)
	measurement := "event_rack"
	nodeID := `ns=5;s="Root"."Objects"."event_rack"`
	endpoint := "opc.tcp://172.18.11.10:4840"

	lines, err := formatInfluxOutputWithBits(measurement, nodeID, value, endpoint, nil)
	require.NoError(t, err, "should not error with valid uint32 value")
	require.Len(t, lines, 32, "should return exactly 32 lines (one per bit)")

	// Verify bit 7 is HIGH
	bit7Line := lines[7]
	assert.Contains(t, bit7Line, "event_rack,")
	assert.Contains(t, bit7Line, "bit=7,")
	assert.Contains(t, bit7Line, "bit_name=bit_7")
	assert.Contains(t, bit7Line, " value=1 ") // HIGH
	assert.Contains(t, bit7Line, `node_id=ns\=5;s\=\"Root\".\"Objects\".\"event_rack\"`)
	assert.Contains(t, bit7Line, "endpoint=opc.tcp://172.18.11.10:4840")

	// Verify bit 27 is HIGH
	bit27Line := lines[27]
	assert.Contains(t, bit27Line, "bit=27,")
	assert.Contains(t, bit27Line, "bit_name=bit_27")
	assert.Contains(t, bit27Line, " value=1 ") // HIGH

	// Verify bit 0 is LOW
	bit0Line := lines[0]
	assert.Contains(t, bit0Line, "bit=0,")
	assert.Contains(t, bit0Line, "bit_name=bit_0")
	assert.Contains(t, bit0Line, " value=0 ") // LOW

	// Verify bit 31 is LOW
	bit31Line := lines[31]
	assert.Contains(t, bit31Line, "bit=31,")
	assert.Contains(t, bit31Line, "bit_name=bit_31")
	assert.Contains(t, bit31Line, " value=0 ") // LOW

	// Verify all lines have the correct measurement and tags
	for i, line := range lines {
		assert.True(t, strings.HasPrefix(line, "event_rack,"), "line %d should start with measurement", i)
		assert.Contains(t, line, "endpoint=opc.tcp://172.18.11.10:4840", "line %d should have endpoint tag", i)
		assert.Regexp(t, ` value=[01] \d+$`, line, "line %d should end with 'value=X timestamp'", i)
	}
}

// TestFormatInfluxOutputWithBits_CustomBitNames tests semantic alarm names
func TestFormatInfluxOutputWithBits_CustomBitNames(t *testing.T) {
	value := uint32(0x00000080) // bit 7 only
	measurement := "event_rack"
	nodeID := "ns=5;s=alarms"
	endpoint := "opc.tcp://localhost:4840"

	// All 32 custom names (semantic alarm names)
	bitNames := []string{
		"motor_fault", "temp_high", "pressure_low", "estop_active",
		"guard_open", "hydraulic_fail", "encoder_error", "drive_fault",
		"overload", "undervoltage", "phase_loss", "contactor_fail",
		"brake_fail", "cooling_fail", "lubrication_low", "vibration_high",
		"bearing_temp", "winding_temp", "current_limit", "torque_limit",
		"position_error", "speed_limit", "cable_break", "sensor_fail",
		"power_supply", "network_lost", "safety_relay", "light_curtain",
		"interlock", "maintenance", "reserved_30", "reserved_31",
	}

	lines, err := formatInfluxOutputWithBits(measurement, nodeID, value, endpoint, bitNames)
	require.NoError(t, err)
	require.Len(t, lines, 32)

	// Verify bit 7 has custom name "drive_fault" and is HIGH
	bit7Line := lines[7]
	assert.Contains(t, bit7Line, "bit=7,")
	assert.Contains(t, bit7Line, "bit_name=drive_fault")
	assert.Contains(t, bit7Line, " value=1 ")

	// Verify bit 0 has custom name "motor_fault" and is LOW
	bit0Line := lines[0]
	assert.Contains(t, bit0Line, "bit=0,")
	assert.Contains(t, bit0Line, "bit_name=motor_fault")
	assert.Contains(t, bit0Line, " value=0 ")

	// Verify bit 31 has custom name "reserved_31"
	bit31Line := lines[31]
	assert.Contains(t, bit31Line, "bit=31,")
	assert.Contains(t, bit31Line, "bit_name=reserved_31")
}

// TestFormatInfluxOutputWithBits_TypeConversions tests all numeric type conversions
func TestFormatInfluxOutputWithBits_TypeConversions(t *testing.T) {
	measurement := "test"
	nodeID := "ns=0;i=1"
	endpoint := "opc.tcp://localhost:4840"

	// Test value: 0x00000003 (bits 0 and 1 HIGH)
	tests := []struct {
		name  string
		value interface{}
	}{
		{"float64", float64(3)},
		{"float32", float32(3)},
		{"int", int(3)},
		{"int32", int32(3)},
		{"int64", int64(3)},
		{"uint", uint(3)},
		{"uint32", uint32(3)},
		{"uint64", uint64(3)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines, err := formatInfluxOutputWithBits(measurement, nodeID, tt.value, endpoint, nil)
			require.NoError(t, err, "type %T should be convertible to uint32", tt.value)
			require.Len(t, lines, 32)

			// Verify bits 0 and 1 are HIGH, bit 2 is LOW
			assert.Contains(t, lines[0], " value=1 ", "bit 0 should be HIGH for value 3")
			assert.Contains(t, lines[1], " value=1 ", "bit 1 should be HIGH for value 3")
			assert.Contains(t, lines[2], " value=0 ", "bit 2 should be LOW for value 3")
		})
	}
}

// TestFormatInfluxOutputWithBits_InvalidTypes tests error handling for non-numeric types
func TestFormatInfluxOutputWithBits_InvalidTypes(t *testing.T) {
	measurement := "test"
	nodeID := "ns=0;i=1"
	endpoint := "opc.tcp://localhost:4840"

	tests := []struct {
		name  string
		value interface{}
	}{
		{"string", "not a number"},
		{"bool", true},
		{"nil", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines, err := formatInfluxOutputWithBits(measurement, nodeID, tt.value, endpoint, nil)
			assert.Error(t, err, "should error for non-numeric type %T", tt.value)
			assert.Nil(t, lines, "should return nil lines on error")
			assert.Contains(t, err.Error(), "cannot be converted to uint32", "error should mention conversion failure")
		})
	}
}

// TestFormatInfluxOutputWithBits_TagEscaping tests InfluxDB tag escaping for special characters
func TestFormatInfluxOutputWithBits_TagEscaping(t *testing.T) {
	value := uint32(0x00000001) // bit 0 only
	measurement := "test"
	nodeID := "ns=0;i=1"
	endpoint := "opc.tcp://localhost:4840"

	// Bit names with special characters that need escaping
	bitNames := []string{
		"motor,fault",    // comma needs escaping
		"temp=high",      // equals needs escaping
		"status ok",      // space needs escaping
		"name\"quoted\"", // quotes need escaping
		"normal", "bit5", "bit6", "bit7",
		"bit8", "bit9", "bit10", "bit11", "bit12", "bit13", "bit14", "bit15",
		"bit16", "bit17", "bit18", "bit19", "bit20", "bit21", "bit22", "bit23",
		"bit24", "bit25", "bit26", "bit27", "bit28", "bit29", "bit30", "bit31",
	}

	lines, err := formatInfluxOutputWithBits(measurement, nodeID, value, endpoint, bitNames)
	require.NoError(t, err)
	require.Len(t, lines, 32)

	// Verify tag escaping in the output
	assert.Contains(t, lines[0], `bit_name=motor\,fault`, "comma should be escaped")
	assert.Contains(t, lines[1], `bit_name=temp\=high`, "equals should be escaped")
	assert.Contains(t, lines[2], `bit_name=status\ ok`, "space should be escaped")
	assert.Contains(t, lines[3], `bit_name=name\"quoted\"`, "quotes should be escaped")
	assert.Contains(t, lines[4], "bit_name=normal", "normal name should not be escaped")
}

// TestFormatInfluxOutputWithBits_WrongNumberOfNames tests validation of bit names count
func TestFormatInfluxOutputWithBits_WrongNumberOfNames(t *testing.T) {
	value := uint32(0)
	measurement := "test"
	nodeID := "ns=0;i=1"
	endpoint := "opc.tcp://localhost:4840"

	tests := []struct {
		name     string
		bitNames []string
	}{
		{"31 names", make([]string, 31)},
		{"33 names", make([]string, 33)},
		{"1 name", []string{"only_one"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines, err := formatInfluxOutputWithBits(measurement, nodeID, value, endpoint, tt.bitNames)
			assert.Error(t, err, "should error with %d bit names", len(tt.bitNames))
			assert.Nil(t, lines)
			assert.Contains(t, err.Error(), "must be exactly 32")
		})
	}
}

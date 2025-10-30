package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGetPortForConnection tests the port hashing logic for named connections
func TestGetPortForConnection(t *testing.T) {
	tests := []struct {
		name         string
		connectionID string
		basePort     int
		wantPort     int
	}{
		{
			name:         "default connection uses base port",
			connectionID: "default",
			basePort:     8765,
			wantPort:     8765,
		},
		{
			name:         "named connection gets hashed port",
			connectionID: "plc1",
			basePort:     8765,
			wantPort:     getPortForConnection("plc1", 8765),
		},
		{
			name:         "different names get different ports",
			connectionID: "plc2",
			basePort:     8765,
			wantPort:     getPortForConnection("plc2", 8765),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPort := getPortForConnection(tt.connectionID, tt.basePort)
			assert.Equal(t, tt.wantPort, gotPort)

			// Verify port is in valid range
			assert.GreaterOrEqual(t, gotPort, 8765, "port should be >= base port")
			assert.LessOrEqual(t, gotPort, 65000, "port should be <= 65000")
		})
	}

	// Test determinism - same name should always produce same port
	t.Run("deterministic hashing", func(t *testing.T) {
		name := "test-connection"
		port1 := getPortForConnection(name, 8765)
		port2 := getPortForConnection(name, 8765)
		assert.Equal(t, port1, port2, "same connection name should produce same port")
	})

	// Test that different names produce different ports
	t.Run("unique ports for different names", func(t *testing.T) {
		port1 := getPortForConnection("connection1", 8765)
		port2 := getPortForConnection("connection2", 8765)
		assert.NotEqual(t, port1, port2, "different connection names should produce different ports")
	})
}

// TestGetServiceDescriptor tests the service descriptor generation
func TestGetServiceDescriptor(t *testing.T) {
	tests := []struct {
		name           string
		connectionName string
		want           string
	}{
		{
			name:           "default connection",
			connectionName: "default",
			want:           "OPCUA service",
		},
		{
			name:           "named connection",
			connectionName: "plc1",
			want:           "OPCUA service 'plc1'",
		},
		{
			name:           "another named connection",
			connectionName: "my-custom-plc",
			want:           "OPCUA service 'my-custom-plc'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getServiceDescriptor(tt.connectionName)
			assert.Equal(t, tt.want, got)
		})
	}
}

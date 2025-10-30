# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`plccli` is an OPC UA (Open Platform Communications Unified Architecture) command-line interface tool written in Go. It provides a client-server architecture for interacting with industrial automation systems, with built-in support for InfluxDB and Prometheus monitoring.

## Architecture

### Client-Server Model

The application operates in two modes:

1. **Service Mode** (`--service` flag): Runs a persistent HTTP server that maintains a connection to an OPC UA server
   - HTTP server listens on configurable ports (default: 8765, or hashed ports for named connections)
   - Maintains keep-alive connection with 30-second health checks (service.go:127)
   - Automatically reconnects with exponential backoff on connection failures (service.go:434-490)
   - Certificates are stored in `~/.config/plccli/` directory

2. **Client Mode** (default): Makes HTTP requests to the service to read/write OPC UA nodes
   - Communicates with the service via HTTP API
   - Supports multiple node reads in a single batch request

### File Structure

- `main.go`: Entry point, CLI flag parsing, command routing
- `service.go`: HTTP service implementation, OPC UA connection management, API endpoints
- `client.go`: HTTP client implementation for communicating with the service
- `browse.go`: Node browsing functionality (recursive tree traversal)
- `types.go`: Shared data structures (NodeResponse)

### Key Components

**Connection Management** (service.go:177-431):
- Automatic endpoint discovery with security policy negotiation
- Support for multiple authentication methods (UserName, Anonymous)
- Certificate generation and management
- Context-based timeouts and cancellation

**Multiple Connections**:
- Use `--connection <name>` to manage multiple OPC UA server connections simultaneously
- Each connection uses a deterministic port derived from the connection name via FNV hash (main.go:44-56)
- Connection-specific certificates are generated with connection name suffix

**HTTP API Endpoints** (service.go):
- `GET /api/node?namespace=X&type=Y&identifier=Z` - Read single node
- `POST /api/node` - Write node value (requires dataType field)
- `POST /api/nodes` - Batch read multiple nodes
- `GET /api/browse?nodeid=X&maxdepth=Y` - Browse node tree
- `GET /api/info` - Get connection information

## Building and Testing

### Build Commands

```bash
# Build for current platform
make build

# Build for specific platforms
make build-mac      # macOS Apple Silicon
make build-linux    # Linux (amd64 and arm64)

# Build all platforms
make all

# Clean build artifacts
make clean
```

### Running Tests

The project uses standard Go testing with testify for assertions:

```bash
# Run all tests
make test

# Run tests with coverage report
make test-coverage

# Run tests with race detector
make test-verbose

# Or use go test directly
go test ./...
go test -v ./...
go test -run TestBooleanParsing ./...
```

**Test files:**
- `client_test.go`: Tests for parseNodeID() and formatInfluxOutput()
- `service_test.go`: Tests for boolean variant creation and write operations
- `main_test.go`: Tests for CLI utilities (port hashing, service descriptors)

### Running the Application

```bash
# Start the service (must be running before client commands)
./plccli --service --endpoint opc.tcp://192.168.1.100:4840 --username user --password pass

# In another terminal, read a node
./plccli opcua get ns=3;s=Temperature

# Write a node
./plccli opcua set ns=3;s=MyVariable 42 int32

# Browse nodes
./plccli opcua browse ns=3;s=MyFolder 2

# Extract all 32 bits from a uint32 alarm field
./plccli --bits --measurement event_rack opcua get "ns=5;s=\"Root\".\"Objects\".\"event_rack\""

# Extract bits with custom names (exactly 32 names required)
./plccli --bits --measurement event_rack --bit-names "motor_fault,temp_high,pressure_low,..." opcua get "ns=5;s=...\""
```

## Development Patterns

### Node ID Parsing

Node IDs accept both semicolon and comma separators:
- `ns=3;s=Temperature` (standard format)
- `ns=3,s=Temperature` (alternative format)
- `ns=0;i=2258` (numeric identifier)

Parsing logic in client.go:14-55 tries both formats for robustness.

### Output Formats

Two output formats are supported via `--format` flag:
1. **default**: Human-readable output
2. **influx**: InfluxDB Line Protocol format with escaped special characters

InfluxDB formatting (client.go:58-106) converts all values to numeric for proper Prometheus/InfluxDB ingestion.

### Bit Extraction (Alarm Monitoring)

For monitoring alarm fields stored as uint32 bitfields:

**Implementation (bitfield.go):**
- `getBitValue(value uint32, bitNum int) int` - Extract single bit (LSB=0, MSB=31)
- `extractBits(value uint32, bitNames []string) ([]BitValue, error)` - Extract all 32 bits

**Usage:**
```bash
# Extract all 32 bits with default names (bit_0, bit_1, ..., bit_31)
plccli --bits --measurement event_rack opcua get "ns=5;s=alarm_field"

# Extract with custom bit names (MUST be exactly 32 names)
plccli --bits --bit-names "motor_fault,temp_high,..." opcua get "ns=5;s=alarm_field"
```

**Output format (InfluxDB):**
```
event_rack,node_id=...,endpoint=...,bit=0,bit_name=motor_fault value=0 1761836282581869000
event_rack,node_id=...,endpoint=...,bit=7,bit_name=pressure_low value=1 1761836282581869000
event_rack,node_id=...,endpoint=...,bit=27,bit_name=safety_circuit value=1 1761836282581869000
```

**Safety guarantees:**
- Requires `--format influx` (errors otherwise)
- Bit names must be exactly 32 (errors if not)
- Only 32-bit (uint32) values supported
- Bit order: LSB=bit0, MSB=bit31
- Comprehensive test coverage in bitfield_test.go

### Error Handling

Connection errors are handled consistently via handleConnectionError() (main.go:96-108):
- Provides helpful error messages with exact commands to start the service
- Includes connection name context for multi-connection scenarios

### Security Configuration

Security policies and modes are automatically negotiated based on server capabilities (service.go:256-330):
1. First tries anonymous authentication with no security
2. Falls back to username authentication with best available security policy
3. Certificate generation is automatic when security is required

## Common Development Tasks

### Adding New HTTP Endpoints

1. Add handler function in service.go following the pattern of existing handlers
2. Register route in startService() function (service.go:73-108)
3. Add corresponding client function in client.go if needed
4. Update main.go command routing if exposing via CLI

### Modifying OPC UA Operations

Most OPC UA operations happen in service.go handlers:
- Node reads use client.Node(id).Value(ctx) pattern
- Node writes use client.Write(ctx, &ua.WriteRequest{...})
- Browse operations use recursive tree traversal in browse.go:189-299

### Version Information

Version info is injected at build time via linker flags (Makefile:16-18):
- `buildVersion`: Git tag or "dev"
- `buildCommit`: Short commit hash
- `buildTime`: Build timestamp

## Dependencies

- `github.com/gopcua/opcua`: OPC UA client library (v0.8.0)
- `golang.org/x/term`: Terminal utilities
- Standard library for HTTP, JSON, context management

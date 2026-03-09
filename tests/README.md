# plccli Integration Tests

Comprehensive integration test suite for plccli on Siemens S7-1200 PLC.

## Test Coverage

- **15 DTL tests**: Siemens DTL (Date Time Long) data type
- **12 Regression tests**: All standard data types (boolean, int32, uint16, etc.)
- **5 Service tests**: Health checks and connection stability
- **3 Format tests**: Default and InfluxDB output formats
- **2 Browse tests**: Node browsing functionality

**Total: 37 tests**

## Setup

### 1. Configure Test Environment

```bash
# Copy example configuration
cp tests/.env.test.example tests/.env.test

# Edit configuration for your PLC
nano tests/.env.test
```

Configure the following in `.env.test`:
- `OPCUA_ENDPOINT`: Your PLC's OPC UA endpoint
- `DTL_NODE`: Node ID of a DTL field (e.g., ns=4,i=38)
- `INT32_NODE`: Node ID of an int32 field
- `BOOL_NODE`: Node ID of a boolean field
- `UINT16_NODE`: Node ID of a uint16 field

### 2. Build plccli

```bash
make build
```

### 3. Start Service

In a separate terminal, start the plccli service:

```bash
./plccli --service --endpoint opc.tcp://YOUR_PLC_IP:4840 --username "" --password ""
```

## Running Tests

### Run All Tests

```bash
./tests/test_dtl_s7_1200.sh
```

### Run from Project Root

```bash
cd /path/to/plccli
./tests/test_dtl_s7_1200.sh
```

### Verbose Mode

For detailed command output:

```bash
VERBOSE=true ./tests/test_dtl_s7_1200.sh
```

## Test Output

### Successful Run

```
================================
plccli Integration Test Suite
Siemens S7-1200
================================

Configuration:
  Endpoint: opc.tcp://192.168.123.252:4840
  DTL Node: ns=4,i=38
  Binary: ./plccli
  Service: ✓ Running on port 8765

════════════════════════════════
DTL TESTS (15)
════════════════════════════════
[1/37] DTL: Write ISO 8601........................ ✓ PASS (0.12s)
[2/37] DTL: Read ISO 8601......................... ✓ PASS (0.08s)
...

════════════════════════════════
RESULTS
════════════════════════════════
Total:    37 tests
Passed:   37 (100.0%)
Failed:   0 (0.0%)
Duration: 5s

Detailed log: tests/test_results_20260309_202145.log
════════════════════════════════
```

### Failed Test Example

```
[21/37] Regression: string........................ ✗ FAIL (0.10s)
    Expected pattern 'test_string' not found in output: Error: service reported error: ...

════════════════════════════════
RESULTS
════════════════════════════════
Total:    37 tests
Passed:   36 (97.3%)
Failed:   1 (2.7%)
Duration: 5s

Failed Tests:
  [21] Regression: string

Detailed log: tests/test_results_20260309_202145.log
════════════════════════════════
```

## Test Logs

Each test run creates a detailed log file in `tests/`:
- Filename format: `test_results_YYYYMMDD_HHMMSS.log`
- Contains timestamps, commands, outputs, and error details
- Useful for debugging failed tests

## Troubleshooting

### Service Not Running

```
Error: plccli service not running on port 8765
```

**Solution**: Start the service in another terminal:
```bash
./plccli --service --endpoint opc.tcp://YOUR_PLC_IP:4840
```

### Binary Not Found

```
Error: plccli binary not found at ./plccli
```

**Solution**: Build the binary:
```bash
make build
```

### Configuration Not Found

```
Error: tests/.env.test not found
```

**Solution**: Create configuration from example:
```bash
cp tests/.env.test.example tests/.env.test
# Edit tests/.env.test with your PLC details
```

### Node Not Writable

Some tests may fail if the PLC nodes are configured as read-only. Check your PLC program (TIA Portal) and ensure test nodes have write access enabled.

## CI/CD Integration

The test script returns appropriate exit codes for CI/CD:
- Exit 0: All tests passed
- Exit 1: One or more tests failed

Example GitHub Actions:

```yaml
name: Integration Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Build plccli
        run: make build
      - name: Start service
        run: |
          ./plccli --service --endpoint opc.tcp://test-plc:4840 &
          sleep 2
      - name: Run tests
        run: ./tests/test_dtl_s7_1200.sh
```

## Test Categories

### DTL Tests
Tests specific to Siemens DTL (Date Time Long) data type:
- ISO 8601 format parsing
- Space-separated format
- RFC3339 with timezone
- Edge cases (leap year, end of year, midnight)
- Weekday auto-calculation
- Child field verification

### Regression Tests
Ensures existing functionality still works after DTL implementation:
- All basic data types (boolean, integers, floats, strings)
- Read and write operations
- Multiple node operations

### Service Tests
Health and stability checks:
- Service connectivity
- API endpoints
- Rapid operation handling
- Large value handling

### Format Tests
Output formatting validation:
- Default format
- InfluxDB Line Protocol
- Custom measurements

### Browse Tests
Node browsing functionality:
- DTL node structure (8 children)
- General browse operations

## Development

### Adding New Tests

1. Edit `tests/test_dtl_s7_1200.sh`
2. Add test to appropriate category function
3. Increment total test count in documentation
4. Use `run_test` helper function:

```bash
run_test "Test name" \
    "command to run" \
    "expected pattern in output" \
    error_ok_flag  # optional: true if error expected
```

### Test Naming Convention

Format: `Category: Description`

Examples:
- `DTL: Write ISO 8601`
- `Regression: int32 positive`
- `Service: Health check`
- `Format: InfluxDB output`

## Support

For issues or questions about the test suite:
1. Check test logs in `tests/test_results_*.log`
2. Run with `VERBOSE=true` for detailed output
3. Open an issue on GitHub with test output

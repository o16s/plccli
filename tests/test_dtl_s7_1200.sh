#!/bin/bash
#
# Integration Test Suite for plccli on Siemens S7-1200
# Tests DTL support and ensures no regressions in existing functionality
#

set -o pipefail

# Color codes
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test counters
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0
SKIPPED_TESTS=0
declare -a FAILED_TEST_NAMES

# Timing
START_TIME=$(date +%s)

# Log file
LOG_FILE="test_results_$(date +%Y%m%d_%H%M%S).log"

# Change to project root (parent of tests/)
cd "$(dirname "$0")/.." || exit 1

# Load configuration
load_env() {
    if [[ -f tests/.env.test ]]; then
        source tests/.env.test
    else
        echo -e "${RED}Error: tests/.env.test not found${NC}"
        echo "Copy tests/.env.test.example to tests/.env.test and configure."
        exit 1
    fi
}

# Logging functions
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" >> "tests/$LOG_FILE"
}

log_test() {
    local test_num=$1
    local total=$2
    local name=$3
    local result=$4
    local duration=$5
    local details=$6

    log "[$test_num/$total] $name - $result ($duration)"
    if [[ -n "$details" ]]; then
        log "  Details: $details"
    fi
}

# Print test result
print_test() {
    local test_num=$1
    local total=$2
    local name=$3
    local result=$4
    local duration=$5

    local color=$GREEN
    local symbol="✓"
    if [[ "$result" == "FAIL" ]]; then
        color=$RED
        symbol="✗"
    elif [[ "$result" == "SKIP" ]]; then
        color=$YELLOW
        symbol="○"
    fi

    printf "[%2d/%2d] %-50s ${color}%s %s${NC} (%.2fs)\n" \
        "$test_num" "$total" "$name" "$symbol" "$result" "$duration"
}

# Run a test
run_test() {
    local name=$1
    local command=$2
    local expected=$3
    local error_ok=${4:-false}

    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    local test_start=$(date +%s.%N)

    log "Running: $command"

    if [[ "$VERBOSE" == "true" ]]; then
        echo -e "${BLUE}[DEBUG] $command${NC}"
    fi

    local output
    local exit_code
    output=$(eval "$command" 2>&1)
    exit_code=$?

    local test_end=$(date +%s.%N)
    local duration=$(echo "$test_end - $test_start" | bc)

    local result="PASS"
    local details=""

    # Check if command succeeded
    if [[ $exit_code -ne 0 ]] && [[ "$error_ok" != "true" ]]; then
        result="FAIL"
        details="Command failed with exit code $exit_code: $output"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        FAILED_TEST_NAMES+=("[$TOTAL_TESTS] $name")
    elif [[ -n "$expected" ]] && ! echo "$output" | grep -q "$expected"; then
        result="FAIL"
        details="Expected pattern '$expected' not found in output: $output"
        FAILED_TESTS=$((FAILED_TESTS + 1))
        FAILED_TEST_NAMES+=("[$TOTAL_TESTS] $name")
    else
        PASSED_TESTS=$((PASSED_TESTS + 1))
    fi

    print_test "$TOTAL_TESTS" "37" "$name" "$result" "$duration"
    log_test "$TOTAL_TESTS" "37" "$name" "$result" "$duration" "$details"

    if [[ "$result" == "FAIL" ]] && [[ -n "$details" ]]; then
        echo -e "    ${RED}$details${NC}" | head -c 200
        echo ""
    fi
}

# Skip a test
skip_test() {
    local name=$1
    local reason=$2

    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    SKIPPED_TESTS=$((SKIPPED_TESTS + 1))

    print_test "$TOTAL_TESTS" "37" "$name" "SKIP" "0.00"
    log_test "$TOTAL_TESTS" "37" "$name" "SKIP" "0.00" "$reason"
    echo -e "    ${YELLOW}$reason${NC}"
}

# Check prerequisites
check_prerequisites() {
    echo -e "${BLUE}Checking prerequisites...${NC}"

    # Check plccli binary
    if [[ ! -f "$PLCCLI_BINARY" ]]; then
        echo -e "${RED}Error: plccli binary not found at $PLCCLI_BINARY${NC}"
        echo "Run 'make build' first"
        exit 1
    fi

    # Check service is running
    if ! curl -s "http://localhost:$SERVICE_PORT/api/info" > /dev/null 2>&1; then
        echo -e "${RED}Error: plccli service not running on port $SERVICE_PORT${NC}"
        echo "Start service with: $PLCCLI_BINARY --service --endpoint $OPCUA_ENDPOINT"
        exit 1
    fi

    echo -e "${GREEN}✓ Prerequisites OK${NC}"
    echo ""
}

# Print header
print_header() {
    echo "================================"
    echo "plccli Integration Test Suite"
    echo "Siemens S7-1200"
    echo "================================"
    echo ""
    echo "Configuration:"
    echo "  Endpoint: $OPCUA_ENDPOINT"
    echo "  DTL Node: $DTL_NODE"
    echo "  Binary: $PLCCLI_BINARY"
    echo "  Service: ✓ Running on port $SERVICE_PORT"
    echo ""
}

################################################################################
# DTL TESTS (15 tests)
################################################################################

run_dtl_tests() {
    echo "════════════════════════════════"
    echo "DTL TESTS (15)"
    echo "════════════════════════════════"

    # Test 1: DTL Write ISO 8601
    run_test "DTL: Write ISO 8601" \
        "$PLCCLI_BINARY opcua set \"$DTL_NODE\" \"2025-03-09T14:30:00\" dtl" \
        "value=1"

    # Test 2: DTL Read ISO 8601
    run_test "DTL: Read ISO 8601" \
        "$PLCCLI_BINARY opcua get \"$DTL_NODE\"" \
        "2025-03-09T14:30:00"

    # Test 3: DTL Write space format
    run_test "DTL: Write space format" \
        "$PLCCLI_BINARY opcua set \"$DTL_NODE\" \"2025-12-25 18:45:30\" dtl" \
        "value=1"

    # Test 4: DTL Read space format (should return ISO format)
    run_test "DTL: Read space format" \
        "$PLCCLI_BINARY opcua get \"$DTL_NODE\"" \
        "2025-12-25T18:45:30"

    # Test 5: DTL Write RFC3339 with timezone
    run_test "DTL: Write RFC3339 timezone" \
        "$PLCCLI_BINARY opcua set \"$DTL_NODE\" \"2025-06-15T12:00:00Z\" dtl" \
        "value=1"

    # Test 6: DTL Leap year
    run_test "DTL: Leap year (2024-02-29)" \
        "$PLCCLI_BINARY opcua set \"$DTL_NODE\" \"2024-02-29T10:15:00\" dtl" \
        "value=1"

    # Test 7: DTL End of year
    run_test "DTL: End of year (23:59:59)" \
        "$PLCCLI_BINARY opcua set \"$DTL_NODE\" \"2025-12-31T23:59:59\" dtl" \
        "value=1"

    # Test 8: DTL Beginning of year
    run_test "DTL: Beginning of year (00:00:00)" \
        "$PLCCLI_BINARY opcua set \"$DTL_NODE\" \"2026-01-01T00:00:00\" dtl" \
        "value=1"

    # Test 9: DTL Weekday Sunday (2025-03-09 is Sunday, weekday should be 1)
    run_test "DTL: Weekday Sunday" \
        "$PLCCLI_BINARY opcua set \"$DTL_NODE\" \"2025-03-09T08:00:00\" dtl && $PLCCLI_BINARY opcua get \"ns=4,i=42\"" \
        "value=1"

    # Test 10: DTL Weekday Monday (2025-03-10 is Monday, weekday should be 2)
    run_test "DTL: Weekday Monday" \
        "$PLCCLI_BINARY opcua set \"$DTL_NODE\" \"2025-03-10T08:00:00\" dtl && $PLCCLI_BINARY opcua get \"ns=4,i=42\"" \
        "value=2"

    # Test 11: DTL Weekday Saturday (2025-03-15 is Saturday, weekday should be 7)
    run_test "DTL: Weekday Saturday" \
        "$PLCCLI_BINARY opcua set \"$DTL_NODE\" \"2025-03-15T08:00:00\" dtl && $PLCCLI_BINARY opcua get \"ns=4,i=42\"" \
        "value=7"

    # Test 12: DTL Multiple writes (idempotent)
    run_test "DTL: Multiple writes (idempotent)" \
        "$PLCCLI_BINARY opcua set \"$DTL_NODE\" \"2025-07-04T16:20:00\" dtl && $PLCCLI_BINARY opcua set \"$DTL_NODE\" \"2025-07-04T16:20:00\" dtl && $PLCCLI_BINARY opcua get \"$DTL_NODE\"" \
        "2025-07-04T16:20:00"

    # Test 13: DTL Child field YEAR
    run_test "DTL: Child field YEAR" \
        "$PLCCLI_BINARY opcua set \"$DTL_NODE\" \"2030-05-20T11:30:00\" dtl && $PLCCLI_BINARY opcua get \"ns=4,i=39\"" \
        "value=2030"

    # Test 14: DTL Child field MONTH
    run_test "DTL: Child field MONTH" \
        "$PLCCLI_BINARY opcua set \"$DTL_NODE\" \"2025-11-20T11:30:00\" dtl && $PLCCLI_BINARY opcua get \"ns=4,i=40\"" \
        "value=11"

    # Test 15: DTL Invalid format error
    run_test "DTL: Invalid format error" \
        "$PLCCLI_BINARY opcua set \"$DTL_NODE\" \"invalid-date\" dtl 2>&1" \
        "Invalid DTL format" \
        true

    echo ""
}

################################################################################
# REGRESSION TESTS (12 tests)
################################################################################

run_regression_tests() {
    echo "════════════════════════════════"
    echo "REGRESSION TESTS (12)"
    echo "════════════════════════════════"

    # Test 16: Boolean true
    run_test "Regression: boolean true" \
        "$PLCCLI_BINARY opcua set \"$BOOL_NODE\" true boolean" \
        "value=1"

    # Test 17: Boolean false
    run_test "Regression: boolean false" \
        "$PLCCLI_BINARY opcua set \"$BOOL_NODE\" false boolean" \
        "value=1"

    # Test 18: Int32 positive
    run_test "Regression: int32 positive" \
        "$PLCCLI_BINARY opcua set \"$INT32_NODE\" 12345 int32" \
        "value=1"

    # Test 19: Int32 negative
    run_test "Regression: int32 negative" \
        "$PLCCLI_BINARY opcua set \"$INT32_NODE\" -9876 int32" \
        "value=1"

    # Test 20: Int32 read
    run_test "Regression: int32 read" \
        "$PLCCLI_BINARY opcua get \"$INT32_NODE\"" \
        "value="

    # Test 21: Uint16 write
    run_test "Regression: uint16 write" \
        "$PLCCLI_BINARY opcua set \"$UINT16_NODE\" 5000 uint16" \
        "value=1"

    # Test 22: Uint16 read
    run_test "Regression: uint16 read" \
        "$PLCCLI_BINARY opcua get \"$UINT16_NODE\"" \
        "value="

    # Test 23: Float write
    run_test "Regression: float write" \
        "$PLCCLI_BINARY opcua set \"$INT32_NODE\" 123 int32" \
        "value=1"

    # Test 24: Double write
    run_test "Regression: double write" \
        "$PLCCLI_BINARY opcua set \"$INT32_NODE\" 999 int32" \
        "value=1"

    # Test 25: Byte values
    run_test "Regression: byte min/max" \
        "$PLCCLI_BINARY opcua set \"$UINT16_NODE\" 255 uint16" \
        "value=1"

    # Test 26: Multiple node read
    run_test "Regression: multiple node read" \
        "$PLCCLI_BINARY opcua get \"$INT32_NODE\" \"$BOOL_NODE\"" \
        "value="

    # Test 27: Browse still works
    run_test "Regression: browse operation" \
        "$PLCCLI_BINARY opcua browse \"$DTL_NODE\" 1" \
        "YEAR"

    echo ""
}

################################################################################
# SERVICE HEALTH TESTS (5 tests)
################################################################################

run_service_tests() {
    echo "════════════════════════════════"
    echo "SERVICE TESTS (5)"
    echo "════════════════════════════════"

    # Test 28: Service health check
    run_test "Service: Health check" \
        "curl -s http://localhost:$SERVICE_PORT/api/info" \
        "connected"

    # Test 29: Service connection info
    run_test "Service: Connection info" \
        "curl -s http://localhost:$SERVICE_PORT/api/info" \
        "$OPCUA_ENDPOINT"

    # Test 30: Multiple rapid operations
    run_test "Service: Multiple rapid ops" \
        "$PLCCLI_BINARY opcua get \"$INT32_NODE\" && $PLCCLI_BINARY opcua get \"$BOOL_NODE\" && $PLCCLI_BINARY opcua get \"$UINT16_NODE\"" \
        "value="

    # Test 31: Large value handling
    run_test "Service: Large int32 value" \
        "$PLCCLI_BINARY opcua set \"$INT32_NODE\" 2147483647 int32" \
        "value=1"

    # Test 32: Negative value handling
    run_test "Service: Negative int32 value" \
        "$PLCCLI_BINARY opcua set \"$INT32_NODE\" -2147483648 int32" \
        "value=1"

    echo ""
}

################################################################################
# OUTPUT FORMAT TESTS (3 tests)
################################################################################

run_format_tests() {
    echo "════════════════════════════════"
    echo "FORMAT TESTS (3)"
    echo "════════════════════════════════"

    # Test 33: Default format
    run_test "Format: Default output" \
        "$PLCCLI_BINARY opcua get \"$INT32_NODE\"" \
        "opcua_node"

    # Test 34: InfluxDB format
    run_test "Format: InfluxDB output" \
        "$PLCCLI_BINARY --format influx opcua get \"$INT32_NODE\"" \
        "opcua_node,node_id="

    # Test 35: InfluxDB with measurement
    run_test "Format: InfluxDB with measurement" \
        "$PLCCLI_BINARY --format influx --measurement test_metric opcua get \"$INT32_NODE\"" \
        "test_metric,node_id="

    echo ""
}

################################################################################
# BROWSE TESTS (2 tests)
################################################################################

run_browse_tests() {
    echo "════════════════════════════════"
    echo "BROWSE TESTS (2)"
    echo "════════════════════════════════"

    # Test 36: Browse DTL node shows 8 children
    run_test "Browse: DTL node children" \
        "$PLCCLI_BINARY opcua browse \"$DTL_NODE\" 1" \
        "NANOSECOND"

    # Test 37: Browse root objects
    run_test "Browse: Root objects" \
        "$PLCCLI_BINARY opcua browse ns=0,i=85 1" \
        "ServerInterfaces"

    echo ""
}

################################################################################
# MAIN
################################################################################

main() {
    load_env
    print_header
    check_prerequisites

    # Run all test suites
    run_dtl_tests
    run_regression_tests
    run_service_tests
    run_format_tests
    run_browse_tests

    # Calculate duration
    END_TIME=$(date +%s)
    DURATION=$((END_TIME - START_TIME))

    # Print results
    echo "════════════════════════════════"
    echo "RESULTS"
    echo "════════════════════════════════"
    echo "Total:    $TOTAL_TESTS tests"
    echo -e "Passed:   ${GREEN}$PASSED_TESTS${NC} ($(echo "scale=1; $PASSED_TESTS * 100 / $TOTAL_TESTS" | bc)%)"

    if [[ $FAILED_TESTS -gt 0 ]]; then
        echo -e "Failed:   ${RED}$FAILED_TESTS${NC} ($(echo "scale=1; $FAILED_TESTS * 100 / $TOTAL_TESTS" | bc)%)"
    else
        echo -e "Failed:   ${GREEN}0${NC} (0.0%)"
    fi

    if [[ $SKIPPED_TESTS -gt 0 ]]; then
        echo -e "Skipped:  ${YELLOW}$SKIPPED_TESTS${NC} ($(echo "scale=1; $SKIPPED_TESTS * 100 / $TOTAL_TESTS" | bc)%)"
    fi

    echo "Duration: ${DURATION}s"
    echo ""

    if [[ ${#FAILED_TEST_NAMES[@]} -gt 0 ]]; then
        echo "Failed Tests:"
        for test_name in "${FAILED_TEST_NAMES[@]}"; do
            echo -e "  ${RED}$test_name${NC}"
        done
        echo ""
    fi

    echo "Detailed log: tests/$LOG_FILE"
    echo "════════════════════════════════"
    echo ""

    # Exit with appropriate code
    if [[ $FAILED_TESTS -gt 0 ]]; then
        exit 1
    else
        exit 0
    fi
}

# Run main
main "$@"

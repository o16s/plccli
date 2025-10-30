#!/bin/bash

# Health check timeout (must be shorter than Docker's healthcheck timeout of 10s)
HEALTH_TIMEOUT=5

# Exit codes
EXIT_HEALTHY=0
EXIT_UNHEALTHY=1

echo "=== PLCCLI Health Check $(date +%Y-%m-%d\ %H:%M:%S) ==="

# 1. Check if the PLCCLI service process is running
if ! pgrep -f "plccli --service" > /dev/null; then
    echo "FAIL: PLCCLI service process is not running"
    exit $EXIT_UNHEALTHY
fi
echo "✓ Process is running"

# 2. Test OPC UA connectivity with timeout using the universal server time node
# ns=0;i=2258 = Server.ServerStatus.CurrentTime (exists on all OPC UA servers)
echo -n "Testing OPC UA read (ns=0;i=2258 Server.CurrentTime) ... "

START=$(date +%s%N 2>/dev/null || date +%s)
if ! timeout $HEALTH_TIMEOUT plccli --measurement opcua_server_time --format influx opcua get ns=0,i=2258 > /dev/null 2>&1; then
    if [ $? -eq 124 ]; then
        echo "FAIL: Timed out after ${HEALTH_TIMEOUT}s"
        echo "This indicates the OPC UA server is not responding or network latency is too high"
    else
        echo "FAIL: Read failed"
        echo "This indicates OPC UA connection is broken or server is down"
    fi
    exit $EXIT_UNHEALTHY
fi
END=$(date +%s%N 2>/dev/null || date +%s)

# Calculate duration in milliseconds (if nanosecond precision available)
if [[ $START =~ [0-9]{19} ]]; then
    DURATION_MS=$(( (END - START) / 1000000 ))
else
    DURATION_MS=$(( (END - START) * 1000 ))
fi

# Check if response time is acceptable
if [ $DURATION_MS -gt 3000 ]; then
    echo "WARN: Slow response (${DURATION_MS}ms) - Check network/PLC performance"
elif [ $DURATION_MS -gt 1000 ]; then
    echo "OK (${DURATION_MS}ms - slightly slow)"
else
    echo "OK (${DURATION_MS}ms)"
fi

# 3. Verify service HTTP endpoint is responding
echo -n "Testing service HTTP endpoint ... "
if ! timeout 2 curl -sf http://localhost:8765/api/info > /dev/null 2>&1; then
    echo "WARN: Service info endpoint not responding (non-critical)"
    # Don't fail on this, just warn
else
    echo "OK"
fi

echo "=== Health Check PASSED ==="
exit $EXIT_HEALTHY
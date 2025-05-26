#!/bin/bash

# Check if the PLCCLI service process is running
if ! pgrep -f "plccli --service" > /dev/null; then
    echo "PLCCLI service is not running"
    exit 1
fi

# Try to get server time to verify service is responsive
# Using ns=0;i=2258 which is the CurrentTime node in the Server object
if ! plccli opcua get ns=0;i=2258 > /dev/null 2>&1; then
    echo "PLCCLI service is not responding"
    exit 1
fi

# If we get here, service is healthy
echo "PLCCLI service is healthy"
exit 0
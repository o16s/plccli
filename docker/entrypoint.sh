#!/bin/bash
set -e

# Debug information
echo "Checking environment:"
echo "PATH: $PATH"
echo "plccli location: $(which plccli 2>/dev/null || echo 'Not found')"
echo "telegraf location: $(which telegraf 2>/dev/null || echo 'Not found')"

# Start the plccli service in the background if OPCUA variables are provided
if [ ! -z "$OPCUA_ENDPOINT" ] && [ ! -z "$OPCUA_USERNAME" ] && [ ! -z "$OPCUA_PASSWORD" ]; then
    echo "Starting PLCCLI service..."
    /usr/bin/plccli --service --endpoint "$OPCUA_ENDPOINT" --username "$OPCUA_USERNAME" --password "$OPCUA_PASSWORD" &
    
    # Give the service a moment to start
    sleep 2
    echo "PLCCLI service started"
else
    echo "OPCUA environment variables not set. PLCCLI service will not start."
fi

# Execute the original telegraf command with all arguments
exec /usr/bin/telegraf "$@"
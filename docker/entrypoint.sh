#!/bin/bash
set -e

# Debug information
echo "Checking environment:"
echo "PATH: $PATH"
echo "plccli location: $(which plccli 2>/dev/null || echo 'Not found')"
echo "telegraf location: $(which telegraf 2>/dev/null || echo 'Not found')"
echo "opc ua endpoint: $OPCUA_ENDPOINT"

# Start the plccli service in the background if OPCUA variables are provided
if [ ! -z "$OPCUA_ENDPOINT" ] && [ ! -z "$OPCUA_USERNAME" ] && [ ! -z "$OPCUA_PASSWORD" ]; then
    echo "Starting PLCCLI service in background..."
    /usr/bin/plccli --service --endpoint "$OPCUA_ENDPOINT" --username "$OPCUA_USERNAME" --password "$OPCUA_PASSWORD" &

    # Brief delay to ensure the service process has started (not for connection establishment)
    # The service will retry connections indefinitely with exponential backoff and random jitter
    sleep 2
    echo "PLCCLI service started (will connect to OPC UA server with automatic retry)"
else
    echo "OPCUA environment variables not set. PLCCLI service will not start."
fi


# Execute telegraf with all arguments
echo "Starting Telegraf..."

if [ $# -eq 0 ]; then
    # No arguments passed - use default
    exec /usr/bin/telegraf
elif [ "$1" = "telegraf" ]; then
    # First argument is "telegraf" - remove it and pass the rest
    shift
    exec /usr/bin/telegraf "$@"
else
    # Arguments passed directly (like from docker-compose command)
    exec /usr/bin/telegraf "$@"
fi
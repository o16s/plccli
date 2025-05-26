# plccli - OPC UA Command Line Interface

`plccli` is a powerful command-line tool for interacting with OPC UA servers. It provides a simple interface for reading and writing values, browsing node structures, and connecting to industrial automation systems with built-in support for InfluxDB and Prometheus monitoring.

![plccli](https://img.shields.io/github/v/release/o16s/plccli)
![license](https://img.shields.io/github/license/o16s/plccli)

## Quick Installation

```bash
curl -s https://raw.githubusercontent.com/o16s/plccli/main/install.sh | bash
```

## Basic Usage

### Starting the Service

Before using `plccli`, you need to start a background service that maintains the connection to your OPC UA server:

```bash
plccli --service --endpoint opc.tcp://your-plc-ip:4840 --username "username" --password "password"
```

Keep this service running in a terminal window, then use other commands in a different terminal.

### Reading a Value

```bash
plccli opcua get ns=3;s=MyVariable
```

### Reading Multiple Values

```bash
plccli opcua get ns=3;s=Variable1 ns=3;s=Variable2 ns=3;s=Variable3
```

### Writing a Value

```bash
plccli opcua set ns=3;s=MyVariable 42 int32
```

### Browsing the Node Structure

```bash
plccli opcua browse
```

Or specify a starting node and depth:

```bash
plccli opcua browse ns=3;s=MyFolder 2
```

## InfluxDB and Prometheus Integration

### Basic InfluxDB Output

```bash
plccli --format influx opcua get ns=3;s=Temperature
```

Output example:
```
opcua_node,node_id=ns\=3\;s\=Temperature,endpoint=opc.tcp://192.168.123.252:4840 value=24.5 1748259207129728000
```

### Custom Measurement Names

Use the `--measurement` flag to specify meaningful metric names:

```bash
# Temperature sensor
plccli --format influx --measurement temperature_celsius opcua get ns=3;s=TempSensor

# Pressure reading
plccli --format influx --measurement pressure_bar opcua get ns=3;s=PressureSensor

# Multiple values with same measurement category
plccli --format influx --measurement device_status opcua get ns=0;i=2258 ns=3;s=DeviceState
```

### Telegraf Configuration Example

```toml
[[inputs.exec]]
  commands = [
    "plccli --format influx --measurement server_time opcua get ns=0;i=2258",
    "plccli --format influx --measurement temperature opcua get ns=3;s=Temperature",
    "plccli --format influx --measurement pressure opcua get ns=3;s=Pressure"
  ]
  data_format = "influx"
  interval = "10s"

[[outputs.prometheus_client]]
  listen = ":9273"
  metric_version = 2
```

This produces Prometheus metrics like:
```
temperature_value{node_id="ns=3;s=Temperature",endpoint="opc.tcp://192.168.1.100:4840"} 24.5
pressure_value{node_id="ns=3;s=Pressure",endpoint="opc.tcp://192.168.1.100:4840"} 2.1
```

## Advanced Features

### Remote Service Connections

Connect to `plccli` services running on remote machines:

```bash
# Start service on one machine (e.g., 192.168.1.100)
plccli --service --endpoint opc.tcp://plc-ip:4840 --username user --password pass

# Connect from another machine
plccli --service-host 192.168.1.100 opcua get ns=3;s=Temperature

# With custom port
plccli --service-host 192.168.1.100 --port 9000 opcua get ns=3;s=Variable
```

### Multiple Connections

You can connect to multiple OPC UA servers simultaneously:

```bash
# Start first connection (default)
plccli --service --endpoint opc.tcp://plc1-ip:4840

# Start second connection in another terminal
plccli --connection plc2 --service --endpoint opc.tcp://plc2-ip:4840

# Use specific connection
plccli --connection plc2 opcua get ns=3;s=Variable

# Remote connection to specific service
plccli --service-host 192.168.1.50 --connection plc2 opcua get ns=3;s=Variable
```

### Security Configuration

```bash
# Anonymous authentication
plccli --auth-method Anonymous --service --endpoint opc.tcp://server:4840

# Custom security policy
plccli --security-policy Basic256Sha256 --security-mode Sign --service --endpoint opc.tcp://server:4840
```

### Node ID Formats

`plccli` supports various node ID formats:

```bash
# Numeric identifier
plccli opcua get ns=0;i=2258
plccli opcua get "ns=0,i=2258"

# String identifier
plccli opcua get "ns=3;s=MyVariable"
plccli opcua get "ns=5;s=\"Root\".\"Objects\".\"Temperature\""

# Complex string paths
plccli opcua get "ns=5;s=\"Root\".\"Objects\".\"ServerInterfaces\".\"Cloud_ServerInterface\".\"rack_log_string\""
```

## Docker Integration

### Using with Docker Compose

```yaml
version: '3'
services:
  plccli-telegraf:
    image: ghcr.io/o16s/plccli:latest
    network_mode: host
    environment:
      - OPCUA_ENDPOINT=opc.tcp://192.168.1.100:4840
      - OPCUA_USERNAME=username
      - OPCUA_PASSWORD=password
    volumes:
      - ./telegraf.conf:/etc/telegraf/telegraf.conf:ro
```

## Command Reference

### Global Flags

- `--service` - Run as background service
- `--endpoint <url>` - OPC UA server endpoint
- `--username <user>` - Authentication username
- `--password <pass>` - Authentication password
- `--format <format>` - Output format (default, influx)
- `--measurement <name>` - InfluxDB measurement name (default: opcua_node)
- `--service-host <host>` - Service host/IP (default: localhost)
- `--port <port>` - Service port (default: 8765)
- `--connection <name>` - Connection name for multiple connections
- `--auth-method <method>` - Authentication method (UserName, Anonymous)
- `--security-policy <policy>` - Security policy (None, Basic128Rsa15, Basic256, Basic256Sha256)
- `--security-mode <mode>` - Security mode (None, Sign, SignAndEncrypt)

### Available Data Types for Writing

`boolean`, `sbyte`, `byte`, `int16`, `uint16`, `int32`, `uint32`, `int64`, `uint64`, `float`, `double`, `string`

## Troubleshooting

### Connection Issues

If you're having trouble connecting to your OPC UA server:

1. Verify the server is running and accessible (try pinging the IP address)
2. Check that port 4840 (or your custom port) is open and not blocked by a firewall
3. Verify your username and password are correct
4. Check if the server requires encryption or specific security policies
5. Try using `--auth-method Anonymous` if the server supports it

### Service Not Running

If you get an error that the service is not running:

```bash
# Start the service
plccli --service --endpoint opc.tcp://your-plc-ip:4840

# Or check if service is running on different host/port
plccli --service-host 192.168.1.100 --port 8765 opcua get ns=0;i=2258
```

### Docker Network Issues

When running in Docker and getting "no route to host" errors:

```yaml
# Use host networking
services:
  plccli:
    network_mode: host

# Or use host.docker.internal for local connections
environment:
  - OPCUA_ENDPOINT=opc.tcp://host.docker.internal:4840
```

## Building from Source

If you prefer to build from source:

```bash
# Clone the repository
git clone https://github.com/o16s/plccli.git
cd plccli

# Build for your platform
make build

# Or build for specific platforms
make build-mac     # For Apple Silicon
make build-linux   # For Linux
```

## Limitations

- **Complex Data Types**: Support for complex structured data types is limited
- **Subscription**: Real-time value change subscriptions are not yet implemented
- **Certificate Management**: Advanced certificate management features are limited
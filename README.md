:::thinking
I'll write a comprehensive README.md file for the plccli tool with a focus on quick installation instructions for macOS users. I'll include:

1. Project introduction - what plccli is and what it does
2. Installation instructions for macOS (both Intel and Apple Silicon)
3. Basic usage examples
4. Configuration information
5. Common commands
6. Troubleshooting

For macOS installation, I'll include:
- Homebrew installation if applicable
- Manual binary installation
- Building from source

Let me draft this README with a professional and clear structure.
:::
# plccli - OPC UA Command Line Interface

`plccli` is a powerful command-line tool for interacting with OPC UA servers. It provides a simple interface for reading and writing values, browsing node structures, and connecting to industrial automation systems.

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

### Using InfluxDB Output Format

```bash
plccli --format influx opcua get ns=3;s=Temperature
```

Output example:
```
opcua_value,node_id=ns3_sTemperature,endpoint=opc_tcp__192.168.123.252_4840 value=24.5 1748259207129728000
```

## Advanced Features

### Multiple Connections

You can connect to multiple OPC UA servers simultaneously:

```bash
# Start first connection (default)
plccli --service --endpoint opc.tcp://plc1-ip:4840

# Start second connection in another terminal
plccli --connection plc2 --service --endpoint opc.tcp://plc2-ip:4840

# Use specific connection
plccli --connection plc2 opcua get ns=3;s=Variable
```

### Detailed Help

For more detailed usage information, use the help command:

```bash
plccli --help
```

## Troubleshooting

### Connection Issues

If you're having trouble connecting to your OPC UA server:

1. Verify the server is running and accessible (try pinging the IP address)
2. Check that port 4840 (or your custom port) is open and not blocked by a firewall
3. Verify your username and password are correct
4. Check if the server requires encryption or specific security policies

### Service Not Running

If you get an error that the service is not running, start it with:

```bash
plccli --service --endpoint opc.tcp://your-plc-ip:4840
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
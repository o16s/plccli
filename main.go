package main
import (
    "flag"
    "fmt"
    "hash/fnv"
    "log"
    "os"
    "strconv"
    "strings"
    "path/filepath"
)

// Version information - these will be set during build
var (
    buildVersion string = "v0.3"
    buildCommit  string = "unknown"
    buildTime    string = "unknown"
)

// Common flags
var (
    version       = flag.Bool("version", false, "Show version information")
    endpoint      = flag.String("endpoint", "opc.tcp://192.168.123.252:4840", "OPC UA Endpoint URL")
    username      = flag.String("username", "", "Username")
    password      = flag.String("password", "", "Password")
    certfile      = flag.String("cert", "cert.pem", "Certificate file")
    keyfile       = flag.String("key", "key.pem", "Private key file")
    gencert       = flag.Bool("gen-cert", true, "Generate a new certificate")
    appuri        = flag.String("app-uri", "urn:plccli:client", "Application URI")
    timeout       = flag.Int("timeout", 300, "All timeouts in seconds")
    service       = flag.Bool("service", false, "Run as a background service")
    port          = flag.Int("port", 8765, "Base port for service mode")
    connection    = flag.String("connection", "default", "Connection name for multiple OPCUA connections")
    verbose       = flag.Bool("verbose", false, "Enable verbose logging")
    outputFormat  = flag.String("format", "influx", "Output format: default, json, or influx")
    securityPolicy = flag.String("security-policy", "Basic256", "Security policy: None, Basic128Rsa15, Basic256, Basic256Sha256")
    securityMode   = flag.String("security-mode", "SignAndEncrypt", "Security mode: None, Sign, SignAndEncrypt")
    authMethod     = flag.String("auth-method", "UserName", "Authentication method: UserName, Anonymous")
)

// Calculate a port number based on connection name
func getPortForConnection(baseName string, basePort int) int {
    if baseName == "default" {
        return basePort
    }

    // Create a deterministic port based on the connection name
    h := fnv.New32a()
    h.Write([]byte(baseName))
    hashValue := h.Sum32()

    // Use the hash to derive a port in the range 10000-65000
    return 10000 + int(hashValue%55000)
}

// Get the service descriptor based on connection name
func getServiceDescriptor(connectionName string) string {
    if connectionName == "default" {
        return "OPCUA service"
    }
    return fmt.Sprintf("OPCUA service '%s'", connectionName)
}

// Print help text with consistent formatting
func printUsage() {
    fmt.Println("Usage: plccli [flags] opcua get <node-id> [node-id2 node-id3 ...]")
    fmt.Println("       plccli [flags] opcua set <node-id> <value> <data-type>")
    fmt.Println("       plccli [flags] opcua browse [node-id] [max-depth]")
    fmt.Println("\nNode ID format: ns=X;i=NUMBER or ns=X;s=STRING (can use comma or semicolon separator)")
    fmt.Println("\nAvailable data types for set: boolean, sbyte, byte, int16, uint16, int32, uint32, int64, uint64, float, double, string")
    fmt.Println("\nOutput formats (--format flag):")
    fmt.Println("  default - Human-readable output")
    fmt.Println("  influx  - InfluxDB Line Protocol format")
    fmt.Println("\nAuthentication options:")
    fmt.Println("  --auth-method UserName (default) - Use username/password authentication")
    fmt.Println("  --auth-method Anonymous - Use anonymous authentication (no credentials)")
    fmt.Println("\nSecurity options:")
    fmt.Println("  --security-policy None|Basic128Rsa15|Basic256|Basic256Sha256")
    fmt.Println("  --security-mode None|Sign|SignAndEncrypt")
    fmt.Println("\nMultiple connections: Use --connection <name> to specify which connection to use")
    fmt.Printf("\nplccli %s (%s, built %s)\n", buildVersion, buildCommit, buildTime)
    flag.PrintDefaults()
}

// Handle connection errors consistently
func handleConnectionError(err error) {
    if strings.Contains(err.Error(), "connection refused") ||
        strings.Contains(err.Error(), "cannot connect to service") {
        serviceDesc := getServiceDescriptor(*connection)
        fmt.Fprintf(os.Stderr, "Error: %s is not running. Start it with:\n", serviceDesc)
        fmt.Fprintf(os.Stderr, "  plccli --connection %s --service --endpoint opc.tcp://opc-ua-server-ip:4840\n", *connection)
        os.Exit(1)
    }
    // For other errors
    fmt.Fprintf(os.Stderr, "Error: %v\n", err)
    os.Exit(1)
}

func main() {
    // Configure logger with timestamps
    log.SetFlags(log.LstdFlags | log.Lmicroseconds)

    // Parse flags before checking for subcommands
    flag.Parse()

    // Show version if requested
    if *version {
        fmt.Printf("plccli version %s\n", buildVersion)
        fmt.Printf("Commit: %s\n", buildCommit)
        fmt.Printf("Built: %s\n", buildTime)
        fmt.Printf("Copyright Octanis Instruments GmbH 2024\n")
        os.Exit(0)
    }

    // Check if we have enough args for a subcommand
    args := flag.Args()

    // Get the actual port to use based on connection name
    actualPort := getPortForConnection(*connection, *port)

    // Service mode
    if *service {
        serviceDesc := getServiceDescriptor(*connection)
        fmt.Printf("Starting %s on port %d...\n", serviceDesc, actualPort)
        fmt.Printf("\nplccli %s (%s, built %s)\n", buildVersion, buildCommit, buildTime)

        // Show connection info
        authInfo := ""
        if strings.ToLower(*authMethod) == "anonymous" {
            authInfo = "with anonymous authentication"
        } else if *username != "" {
            authInfo = fmt.Sprintf("with username '%s'", *username)
        } else {
            authInfo = "without authentication (anonymous)"
        }
        
        fmt.Printf("Connecting to %s %s\n", *endpoint, authInfo)
        fmt.Printf("Security: Policy=%s, Mode=%s\n", *securityPolicy, *securityMode)

        // Check if we need separate cert/key files for this connection
        actualCertFile := *certfile
        actualKeyFile := *keyfile
        if *connection != "default" {
            // For non-default connections, use connection-specific cert/key files
            actualCertFile = strings.TrimSuffix(*certfile, ".pem") + "-" + *connection + ".pem"
            actualKeyFile = strings.TrimSuffix(*keyfile, ".pem") + "-" + *connection + ".pem"
        }

        // Show where certificates will be stored
        homeDir, _ := os.UserHomeDir()
        if homeDir != "" {
            configDir := filepath.Join(homeDir, ".config", "plccli")
            if !filepath.IsAbs(actualCertFile) {
                fmt.Printf("Certificates will be stored in: %s\n", configDir)
            }
        }

        startService(*endpoint, *username, *password, actualCertFile, actualKeyFile,
			*gencert, *appuri, *timeout, actualPort, *verbose, 
			*securityPolicy, *securityMode, *authMethod)
        return
    }

    // Client mode - needs subcommand
    if len(args) < 2 || args[0] != "opcua" {
        printUsage()
        os.Exit(1)
    }

    // Process OPCUA subcommands
    switch args[1] {
    case "browse":
        nodeID := "i=84" // Default to Objects folder
        if len(args) >= 3 {
            nodeID = args[2]
        }
        
        maxDepth := 3 // Default depth
        if len(args) >= 4 {
            if depth, err := strconv.Atoi(args[3]); err == nil {
                maxDepth = depth
            } else {
                fmt.Printf("Warning: Invalid depth value '%s', using default of %d\n", args[3], maxDepth)
            }
        }
        
        if err := browseNode(nodeID, maxDepth, actualPort, *outputFormat); err != nil {
            handleConnectionError(err)
        }

    case "get":
        if len(args) < 3 {
            fmt.Println("Error: Missing node-id")
            printUsage()
            os.Exit(1)
        }
        // Allow multiple node IDs
        nodeIDs := args[2:]
        value, err := getNodeValues(nodeIDs, actualPort, *outputFormat)
        if err != nil {
            handleConnectionError(err)
        }
        fmt.Println(value)

    case "set":
        if len(args) < 5 {
            fmt.Println("Error: Missing arguments for set command")
            printUsage()
            os.Exit(1)
        }
        nodeID := args[2]
        value := args[3]
        dataType := args[4]

        result, err := setNodeValue(nodeID, value, dataType, actualPort, *outputFormat)
        if err != nil {
            handleConnectionError(err)
        }
        fmt.Println(result)

    default:
        fmt.Printf("Unknown command: %s\n\n", args[1])
        printUsage()
        os.Exit(1)
    }
}
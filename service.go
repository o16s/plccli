package main

import (
    "path/filepath"
	"context"
	"crypto/rsa"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"github.com/gopcua/opcua"
	uatest "github.com/gopcua/opcua/tests/python"
	"github.com/gopcua/opcua/ua"
)

var (
	// Global client for service mode
	opcuaClient *opcua.Client
	clientMutex sync.Mutex
	isVerbose   bool
	
	// Store the connection info for diagnostics
	connectionName string
	connectionPort int
)

func startService(endpoint, username, password, certfile, keyfile string, 
                 gencert bool, appuri string, timeout, port int, verbose bool) {
	isVerbose = verbose
	connectionPort = port
	
	// Extract connection name from port if available
	if port != 8765 {
		connectionName = fmt.Sprintf("connection-%d", port)
	} else {
		connectionName = "default"
	}
	
	log.Printf("Starting OPCUA service for connection '%s' on port %d", connectionName, port)
	
	// Configure context with signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	go func() {
		sig := <-sigChan
		log.Printf("[%s] Received signal %v, shutting down...", connectionName, sig)
		cancel()
		// Give time for connections to close gracefully
		time.Sleep(1 * time.Second)
		os.Exit(0)
	}()
	
	// Connect to OPCUA server
	err := connectOPCUA(ctx, endpoint, username, password, certfile, keyfile, gencert, appuri, timeout)
	if err != nil {
		log.Fatalf("[%s] Failed to connect to OPCUA server: %v", connectionName, err)
	}

    http.HandleFunc("/api/browse", func(w http.ResponseWriter, r *http.Request) {
        handleBrowseRequest(w, r)
    })
	
	// Set up HTTP server for API
	http.HandleFunc("/api/node", func(w http.ResponseWriter, r *http.Request) {
		// Route based on HTTP method
		if r.Method == http.MethodGet {
			handleNodeRequest(w, r) // Existing handler for GET
		} else if r.Method == http.MethodPost {
			handleNodeWriteRequest(w, r) // New handler for POST
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	
	// Add new endpoint for batch node operations
	http.HandleFunc("/api/nodes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleBatchNodeRequest(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	
	// Add info endpoint to identify this connection
	http.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		info := map[string]interface{}{
			"connection": connectionName,
			"port":       port,
			"endpoint":   endpoint,
			"status":     "connected",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(info)
	})
	
	// Start the server
	serverAddr := fmt.Sprintf("0.0.0.0:%d", port)
	server := &http.Server{
		Addr: serverAddr,
	}
	
	log.Printf("[%s] OPCUA service running on http://%s", connectionName, serverAddr)
	log.Printf("[%s] Example usage: curl http://%s/api/node?namespace=0&type=i&identifier=2258", connectionName, serverAddr)
	
	// Start HTTP server in a goroutine
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[%s] HTTP server error: %v", connectionName, err)
		}
	}()
	
	// Keep connection alive with periodic reads
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			// Read server time to keep connection alive
			clientMutex.Lock()
			if opcuaClient != nil {
				timeNode := opcuaClient.Node(ua.NewNumericNodeID(0, 2258))
				_, err := timeNode.Value(ctx)
				if err != nil {
					log.Printf("[%s] Keep-alive failed: %v", connectionName, err)
					// Try to reconnect
					reconnectOPCUA(ctx, endpoint, username, password, certfile, keyfile, gencert, appuri, timeout)
				} else if isVerbose {
					log.Printf("[%s] Keep-alive successful", connectionName)
				}
			}
			clientMutex.Unlock()
			
		case <-ctx.Done():
			// Shutdown gracefully
			log.Printf("[%s] Shutting down service...", connectionName)
			
			// Close OPCUA connection
			clientMutex.Lock()
			if opcuaClient != nil {
				opcuaClient.Close(context.Background())
				opcuaClient = nil
			}
			clientMutex.Unlock()
			
			// Shutdown HTTP server
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			
			if err := server.Shutdown(shutdownCtx); err != nil {
				log.Printf("[%s] HTTP server shutdown error: %v", connectionName, err)
			}
			
			return
		}
	}
}

func connectOPCUA(ctx context.Context, endpoint, username, password, certfile, keyfile string, 
                 gencert bool, appuri string, timeout int) error {
    log.Printf("[%s] Connecting to OPCUA server at %s...", connectionName, endpoint)
    
    timeoutDuration := time.Duration(timeout) * time.Second
    
    // Determine the certificate directory based on user's home directory
    homeDir, err := os.UserHomeDir()
    if err != nil {
        log.Printf("[%s] Warning: Could not get user home directory: %v. Using current directory.", connectionName, err)
        homeDir = "."
    } else {
        // Create ~/.config/plccli directory if it doesn't exist
        configDir := filepath.Join(homeDir, ".config")
        if _, err := os.Stat(configDir); err != nil {
            if os.IsNotExist(err) {
                if err := os.Mkdir(configDir, 0755); err != nil {
                    log.Printf("[%s] Warning: Could not create %s directory: %v. Using current directory.", connectionName, configDir, err)
                    homeDir = "."
                }
            } else {
                log.Printf("[%s] Warning: Error checking %s directory: %v. Using current directory.", connectionName, configDir, err)
                homeDir = "."
            }
        }
        
        plcConfigDir := filepath.Join(configDir, "plccli")
        if _, err := os.Stat(plcConfigDir); err != nil {
            if os.IsNotExist(err) {
                if err := os.Mkdir(plcConfigDir, 0755); err != nil {
                    log.Printf("[%s] Warning: Could not create %s directory: %v. Using current directory.", connectionName, plcConfigDir, err)
                    homeDir = "."
                } else {
                    homeDir = plcConfigDir
                }
            } else {
                log.Printf("[%s] Warning: Error checking %s directory: %v. Using current directory.", connectionName, plcConfigDir, err)
                homeDir = "."
            }
        } else {
            homeDir = plcConfigDir
        }
    }
    
    // Update certificate and key file paths to use the home directory
    if !filepath.IsAbs(certfile) {
        certfile = filepath.Join(homeDir, filepath.Base(certfile))
    }
    if !filepath.IsAbs(keyfile) {
        keyfile = filepath.Join(homeDir, filepath.Base(keyfile))
    }
    
    log.Printf("[%s] Using certificate path: %s", connectionName, certfile)
    log.Printf("[%s] Using key path: %s", connectionName, keyfile)
    
    // Generate certificate if needed
    var cert []byte
    var privateKey *rsa.PrivateKey
    
    if gencert {
        log.Printf("[%s] Checking for existing certificate", connectionName)
        // Skip regenerating cert if it exists
        if _, err := os.Stat(certfile); os.IsNotExist(err) {
            log.Printf("[%s] Certificate doesn't exist, generating...", connectionName)
            certPEM, keyPEM, err := uatest.GenerateCert(appuri, 2048, 24*time.Hour)
            if err != nil {
                return fmt.Errorf("failed to generate cert: %v", err)
            }
            if err := os.WriteFile(certfile, certPEM, 0644); err != nil {
                return fmt.Errorf("failed to write %s: %v", certfile, err)
            }
            if err := os.WriteFile(keyfile, keyPEM, 0644); err != nil {
                return fmt.Errorf("failed to write %s: %v", keyfile, err)
            }
            log.Printf("[%s] Generated %s and %s", connectionName, certfile, keyfile)
        } else {
            log.Printf("[%s] Using existing certificate", connectionName)
        }
    }
    
    // Load certificate
    log.Printf("[%s] Loading certificate...", connectionName)
    c, err := tls.LoadX509KeyPair(certfile, keyfile)
    if err != nil {
        return fmt.Errorf("failed to load certificate: %v", err)
    }
    cert = c.Certificate[0]
    if pk, ok := c.PrivateKey.(*rsa.PrivateKey); ok {
        privateKey = pk
    } else {
        return fmt.Errorf("invalid private key type")
    }
    
    // Get endpoints
    log.Printf("[%s] Getting endpoints...", connectionName)
    endpointCtx, cancel := context.WithTimeout(ctx, timeoutDuration)
    defer cancel()
    
    endpoints, err := opcua.GetEndpoints(endpointCtx, endpoint)
    if err != nil {
        return fmt.Errorf("failed to get endpoints: %v", err)
    }
    log.Printf("[%s] Found %d endpoints", connectionName, len(endpoints))
    
    // Find compatible endpoint
    var serverEndpoint *ua.EndpointDescription
    for _, e := range endpoints {
        if e.SecurityPolicyURI == ua.SecurityPolicyURIBasic256 && 
           e.SecurityMode == ua.MessageSecurityModeSignAndEncrypt {
            // Check if it supports username authentication
            for _, t := range e.UserIdentityTokens {
                if t.TokenType == ua.UserTokenTypeUserName {
                    serverEndpoint = e
                    break
                }
            }
            if serverEndpoint != nil {
                break
            }
        }
    }
    
    if serverEndpoint == nil {
        return fmt.Errorf("no compatible endpoint found")
    }
    
    log.Printf("[%s] Selected endpoint: %s with %s/%s", 
        connectionName, serverEndpoint.EndpointURL, 
        serverEndpoint.SecurityPolicyURI, 
        serverEndpoint.SecurityMode)
    
    // Build client options with more aggressive timeouts for reconnection
    opts := []opcua.Option{
        opcua.DialTimeout(timeoutDuration),
        opcua.RequestTimeout(timeoutDuration),
        opcua.SessionTimeout(timeoutDuration * 2), // Longer session timeout
        opcua.AuthUsername(username, password),
        opcua.Certificate(cert),
        opcua.PrivateKey(privateKey),
        opcua.SecurityFromEndpoint(serverEndpoint, ua.UserTokenTypeUserName),
        opcua.AutoReconnect(true), 
    }
    
    // Create client
    log.Printf("[%s] Creating client...", connectionName)
    client, err := opcua.NewClient(endpoint, opts...)
    if err != nil {
        return fmt.Errorf("failed to create client: %v", err)
    }
    
    // Connect
    log.Printf("[%s] Connecting to server...", connectionName)
    connectCtx, cancel := context.WithTimeout(ctx, timeoutDuration)
    defer cancel()
    
    if err := client.Connect(connectCtx); err != nil {
        return fmt.Errorf("failed to connect: %v", err)
    }
    
    log.Printf("[%s] Successfully connected to OPCUA server", connectionName)
    
    // Store client globally
    clientMutex.Lock()
    opcuaClient = client
    clientMutex.Unlock()
    
    return nil
}

func reconnectOPCUA(ctx context.Context, endpoint, username, password, certfile, keyfile string, 
                   gencert bool, appuri string, timeout int) {
    log.Printf("[%s] Attempting to reconnect...", connectionName)
    
    // Close existing connection if any
    clientMutex.Lock()
    if opcuaClient != nil {
        log.Printf("[%s] Closing existing connection...", connectionName)
        opcuaClient.Close(ctx)
        opcuaClient = nil
    }
    clientMutex.Unlock()
    
    // Implement exponential backoff for reconnection
    maxRetries := 5
    for attempt := 0; attempt < maxRetries; attempt++ {
        // Create a fresh context for each attempt
        reconnectTimeout := time.Duration(timeout) * time.Second
        reconnectCtx, cancel := context.WithTimeout(context.Background(), reconnectTimeout)
        
        log.Printf("[%s] Reconnection attempt %d/%d...", connectionName, attempt+1, maxRetries)
        err := connectOPCUA(reconnectCtx, endpoint, username, password, certfile, keyfile, gencert, appuri, timeout)
        cancel()
        
        if err != nil {
            log.Printf("[%s] Reconnection attempt %d failed: %v", connectionName, attempt+1, err)
            
            // Wait before retrying, with exponential backoff
            if attempt < maxRetries-1 {
                backoffTime := time.Duration(1<<uint(attempt)) * time.Second
                if backoffTime > 30*time.Second {
                    backoffTime = 30 * time.Second
                }
                log.Printf("[%s] Waiting %v before next attempt...", connectionName, backoffTime)
                time.Sleep(backoffTime)
            }
        } else {
            log.Printf("[%s] Reconnection successful on attempt %d", connectionName, attempt+1)
            return
        }
    }
    
    log.Printf("[%s] Failed to reconnect after %d attempts, will try again on next keep-alive check", connectionName, maxRetries)
}

func handleNodeRequest(w http.ResponseWriter, r *http.Request) {
    // Get node ID components separately
    namespace := r.URL.Query().Get("namespace")
    idType := r.URL.Query().Get("type")
    identifier := r.URL.Query().Get("identifier")
    
    if namespace == "" || idType == "" || identifier == "" {
        http.Error(w, "Missing required parameters: namespace, type, and identifier", http.StatusBadRequest)
        return
    }
    
    // Try both semicolon and comma formats to build the node ID
    var id *ua.NodeID
    var err error
    var nodeIDStr string
    
    // First try with semicolon (standard format)
    nodeIDStr = fmt.Sprintf("ns=%s;%s=%s", namespace, idType, identifier)
    if isVerbose {
        log.Printf("[%s] Trying to parse node ID: %s", connectionName, nodeIDStr)
    }
    
    id, err = ua.ParseNodeID(nodeIDStr)
    if err != nil {
        // If semicolon format fails, try comma format
        nodeIDStr = fmt.Sprintf("ns=%s,%s=%s", namespace, idType, identifier)
        if isVerbose {
            log.Printf("[%s] Semicolon format failed, trying comma format: %s", connectionName, nodeIDStr)
        }
        
        id, err = ua.ParseNodeID(nodeIDStr)
        if err != nil {
            sendJSONResponse(w, NodeResponse{
                NodeID: nodeIDStr,
                Error:  fmt.Sprintf("Invalid node ID, tried both semicolon and comma formats: %v", err),
            })
            return
        }
    }
    
    clientMutex.Lock()
    client := opcuaClient
    clientMutex.Unlock()
    
    if client == nil {
        http.Error(w, "OPCUA client not connected", http.StatusServiceUnavailable)
        return
    }
    
    // Read the node value
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    if isVerbose {
        log.Printf("[%s] Reading node: %v", connectionName, id)
    }
    
    node := client.Node(id)
    value, err := node.Value(ctx)
    
    if err != nil {
        sendJSONResponse(w, NodeResponse{
            NodeID: nodeIDStr,
            Error:  fmt.Sprintf("Failed to read node: %v", err),
        })
        return
    }
    
    // Return the value
    sendJSONResponse(w, NodeResponse{
        NodeID: nodeIDStr,
        Value:  value.Value(),
    })
}

func handleBatchNodeRequest(w http.ResponseWriter, r *http.Request) {
    // Parse the request body
    var batchRequest struct {
        Nodes []map[string]string `json:"nodes"`
    }
    
    err := json.NewDecoder(r.Body).Decode(&batchRequest)
    if err != nil {
        sendJSONResponseGeneric(w, map[string]interface{}{
            "error": fmt.Sprintf("Failed to parse request: %v", err),
        })
        return
    }
    
    // Validate request
    if len(batchRequest.Nodes) == 0 {
        sendJSONResponseGeneric(w, map[string]interface{}{
            "error": "No nodes specified in request",
        })
        return
    }
    
    clientMutex.Lock()
    client := opcuaClient
    clientMutex.Unlock()
    
    if client == nil {
        sendJSONResponseGeneric(w, map[string]interface{}{
            "error": "OPCUA client not connected",
        })
        return
    }
    
    // Create context with timeout
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    // Process each node
    var results []NodeResponse
    
    for _, nodeParams := range batchRequest.Nodes {
        namespace := nodeParams["namespace"]
        idType := nodeParams["type"]
        identifier := nodeParams["identifier"]
        
        // Validate parameters
        if namespace == "" || idType == "" || identifier == "" {
            results = append(results, NodeResponse{
                NodeID: fmt.Sprintf("ns=%s;%s=%s", namespace, idType, identifier),
                Error:  "Missing required node parameters",
            })
            continue
        }
        
        // Create the node ID
        nodeIDStr := fmt.Sprintf("ns=%s;%s=%s", namespace, idType, identifier)
        id, err := ua.ParseNodeID(nodeIDStr)
        if err != nil {
            results = append(results, NodeResponse{
                NodeID: nodeIDStr,
                Error:  fmt.Sprintf("Invalid node ID: %v", err),
            })
            continue
        }
        
        // Read the node value
        node := client.Node(id)
        value, err := node.Value(ctx)
        
        if err != nil {
            results = append(results, NodeResponse{
                NodeID: nodeIDStr,
                Error:  fmt.Sprintf("Failed to read node: %v", err),
            })
        } else {
            results = append(results, NodeResponse{
                NodeID: nodeIDStr,
                Value:  value.Value(),
            })
        }
    }
    
    // Send the combined response
    sendJSONResponseGeneric(w, map[string]interface{}{
        "results": results,
    })
}

func handleNodeWriteRequest(w http.ResponseWriter, r *http.Request) {
    // Only accept POST requests for writes
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed, use POST for write operations", http.StatusMethodNotAllowed)
        return
    }
    
    // Parse the request body
    var writeRequest struct {
        Namespace  string      `json:"namespace"`
        Type       string      `json:"type"`
        Identifier string      `json:"identifier"`
        Value      string      `json:"value"`  // Always as string, we'll convert
        DataType   string      `json:"dataType"` // REQUIRED
    }
    
    err := json.NewDecoder(r.Body).Decode(&writeRequest)
    if err != nil {
        sendJSONResponse(w, NodeResponse{
            Error: fmt.Sprintf("Failed to parse request: %v", err),
        })
        return
    }
    
    // Validate required fields
    if writeRequest.Namespace == "" || writeRequest.Type == "" || writeRequest.Identifier == "" {
        sendJSONResponse(w, NodeResponse{
            Error: "Missing required fields: namespace, type, and identifier are required",
        })
        return
    }
    
    if writeRequest.DataType == "" {
        sendJSONResponse(w, NodeResponse{
            Error: "Data type is required for writing values",
        })
        return
    }
    
    // Try both semicolon and comma formats for the node ID
    var id *ua.NodeID
    var nodeIDStr string
    
    // First try with semicolon (standard format)
    nodeIDStr = fmt.Sprintf("ns=%s;%s=%s", writeRequest.Namespace, writeRequest.Type, writeRequest.Identifier)
    if isVerbose {
        log.Printf("[%s] Trying to parse node ID: %s", connectionName, nodeIDStr)
    }
    
    id, err = ua.ParseNodeID(nodeIDStr)
    if err != nil {
        // If semicolon format fails, try comma format
        nodeIDStr = fmt.Sprintf("ns=%s,%s=%s", writeRequest.Namespace, writeRequest.Type, writeRequest.Identifier)
        if isVerbose {
            log.Printf("[%s] Semicolon format failed, trying comma format: %s", connectionName, nodeIDStr)
        }
        
        id, err = ua.ParseNodeID(nodeIDStr)
        if err != nil {
            sendJSONResponse(w, NodeResponse{
                NodeID: nodeIDStr,
                Error:  fmt.Sprintf("Invalid node ID, tried both semicolon and comma formats: %v", err),
            })
            return
        }
    }
    
    // Get the client
    clientMutex.Lock()
    client := opcuaClient
    clientMutex.Unlock()
    
    if client == nil {
        sendJSONResponse(w, NodeResponse{
            NodeID: nodeIDStr,
            Error:  "OPCUA client not connected",
        })
        return
    }
    
    // Create context with timeout
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
	
    // Convert the value to the appropriate type based on explicit dataType
    var variant *ua.Variant
    
    switch strings.ToLower(writeRequest.DataType) {
    case "boolean":
        boolValue, err := strconv.ParseBool(writeRequest.Value)
        if err != nil {
            sendJSONResponse(w, NodeResponse{
                NodeID: nodeIDStr,
                Error:  fmt.Sprintf("Invalid boolean value: %v", err),
            })
            return
        }
        variant, err = ua.NewVariant(boolValue)
        
    case "sbyte":
        intValue, err := strconv.ParseInt(writeRequest.Value, 10, 8)
        if err != nil {
            sendJSONResponse(w, NodeResponse{
                NodeID: nodeIDStr,
                Error:  fmt.Sprintf("Invalid sbyte value: %v", err),
            })
            return
        }
        variant, err = ua.NewVariant(int8(intValue))
        
    case "byte":
        uintValue, err := strconv.ParseUint(writeRequest.Value, 10, 8)
        if err != nil {
            sendJSONResponse(w, NodeResponse{
                NodeID: nodeIDStr,
                Error:  fmt.Sprintf("Invalid byte value: %v", err),
            })
            return
        }
        variant, err = ua.NewVariant(uint8(uintValue))
        
    case "int16":
        intValue, err := strconv.ParseInt(writeRequest.Value, 10, 16)
        if err != nil {
            sendJSONResponse(w, NodeResponse{
                NodeID: nodeIDStr,
                Error:  fmt.Sprintf("Invalid int16 value: %v", err),
            })
            return
        }
        variant, err = ua.NewVariant(int16(intValue))
        
    case "uint16":
        uintValue, err := strconv.ParseUint(writeRequest.Value, 10, 16)
        if err != nil {
            sendJSONResponse(w, NodeResponse{
                NodeID: nodeIDStr,
                Error:  fmt.Sprintf("Invalid uint16 value: %v", err),
            })
            return
        }
        variant, err = ua.NewVariant(uint16(uintValue))
        
    case "int32":
        intValue, err := strconv.ParseInt(writeRequest.Value, 10, 32)
        if err != nil {
            sendJSONResponse(w, NodeResponse{
                NodeID: nodeIDStr,
                Error:  fmt.Sprintf("Invalid int32 value: %v", err),
            })
            return
        }
        variant, err = ua.NewVariant(int32(intValue))
        
    case "uint32":
        uintValue, err := strconv.ParseUint(writeRequest.Value, 10, 32)
        if err != nil {
            sendJSONResponse(w, NodeResponse{
                NodeID: nodeIDStr,
                Error:  fmt.Sprintf("Invalid uint32 value: %v", err),
            })
            return
        }
        variant, err = ua.NewVariant(uint32(uintValue))
        
    case "int64":
        intValue, err := strconv.ParseInt(writeRequest.Value, 10, 64)
        if err != nil {
            sendJSONResponse(w, NodeResponse{
                NodeID: nodeIDStr,
                Error:  fmt.Sprintf("Invalid int64 value: %v", err),
            })
            return
        }
        variant, err = ua.NewVariant(intValue)
        
    case "uint64":
        uintValue, err := strconv.ParseUint(writeRequest.Value, 10, 64)
        if err != nil {
            sendJSONResponse(w, NodeResponse{
                NodeID: nodeIDStr,
                Error:  fmt.Sprintf("Invalid uint64 value: %v", err),
            })
            return
        }
        variant, err = ua.NewVariant(uintValue)
        
    case "float":
        floatValue, err := strconv.ParseFloat(writeRequest.Value, 32)
        if err != nil {
            sendJSONResponse(w, NodeResponse{
                NodeID: nodeIDStr,
                Error:  fmt.Sprintf("Invalid float value: %v", err),
            })
            return
        }
        variant, err = ua.NewVariant(float32(floatValue))
        
    case "double":
        doubleValue, err := strconv.ParseFloat(writeRequest.Value, 64)
        if err != nil {
            sendJSONResponse(w, NodeResponse{
                NodeID: nodeIDStr,
                Error:  fmt.Sprintf("Invalid double value: %v", err),
            })
            return
        }
        variant, err = ua.NewVariant(doubleValue)
        
    case "string":
        variant, err = ua.NewVariant(writeRequest.Value)
        
    default:
        sendJSONResponse(w, NodeResponse{
            NodeID: nodeIDStr,
            Error:  fmt.Sprintf("Unsupported data type: %s. Use one of: boolean, sbyte, byte, int16, uint16, int32, uint32, int64, uint64, float, double, string", writeRequest.DataType),
        })
        return
    }
    
    if err != nil {
        sendJSONResponse(w, NodeResponse{
            NodeID: nodeIDStr,
            Error:  fmt.Sprintf("Failed to create variant: %v", err),
        })
        return
    }
    
    // Create a proper write request following the example
    req := &ua.WriteRequest{
        NodesToWrite: []*ua.WriteValue{
            {
                NodeID:      id,
                AttributeID: ua.AttributeIDValue,
                Value: &ua.DataValue{
                    EncodingMask: ua.DataValueValue,
                    Value:        variant,
                },
            },
        },
    }
    
    // Execute the write operation
    resp, err := client.Write(ctx, req)
    if err != nil {
        sendJSONResponse(w, NodeResponse{
            NodeID: nodeIDStr,
            Error:  fmt.Sprintf("Failed to write value: %v", err),
        })
        return
    }
    
    // Check write result
    if resp.Results[0] != ua.StatusOK {
        sendJSONResponse(w, NodeResponse{
            NodeID: nodeIDStr,
            Error:  fmt.Sprintf("Write operation failed with status: %v", resp.Results[0]),
        })
        return
    }
    
    // Return success response
    sendJSONResponse(w, NodeResponse{
        NodeID: nodeIDStr,
        Value:  writeRequest.Value,
    })
}

func sendJSONResponse(w http.ResponseWriter, response NodeResponse) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Generic JSON response function
func sendJSONResponseGeneric(w http.ResponseWriter, response interface{}) {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}


func handleBrowseRequest(w http.ResponseWriter, r *http.Request) {
    // Get parameters
    nodeIDStr := r.URL.Query().Get("nodeid")
    if nodeIDStr == "" {
        nodeIDStr = "i=84" // Default to Objects folder
    }

    nodeIDStr = strings.Replace(nodeIDStr, ",", ";", 1)

    maxDepthStr := r.URL.Query().Get("maxdepth")
    maxDepth := 10 // Default
    if maxDepthStr != "" {
        if depth, err := strconv.Atoi(maxDepthStr); err == nil {
            maxDepth = depth
        }
    }
    
    clientMutex.Lock()
    client := opcuaClient
    clientMutex.Unlock()
    
    if client == nil {
        http.Error(w, "OPCUA client not connected", http.StatusServiceUnavailable)
        return
    }
    
    // Create context with timeout
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    // Perform browse operation
    nodes, err := doBrowse(ctx, client, nodeIDStr, maxDepth)
    if err != nil {
        sendJSONResponseGeneric(w, map[string]interface{}{
            "error": fmt.Sprintf("Browse failed: %v", err),
        })
        return
    }
    
    // Convert NodeInfo to JSON-friendly format
    result := make([]map[string]interface{}, len(nodes))
    for i, node := range nodes {
        result[i] = map[string]interface{}{
            "nodeId":      node.NodeID.String(),
            "browseName":  node.BrowseName,
            "path":        node.Path,
            "dataType":    node.DataType,
            "writable":    node.Writable,
            "description": node.Description,
        }
    }
    
    // Send response
    sendJSONResponseGeneric(w, map[string]interface{}{
        "nodes": result,
    })
}
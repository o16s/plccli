package main

import (
    "path/filepath"
	"context"
	"crypto/rsa"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
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
                 gencert bool, appuri string, timeout, port int, verbose bool,
                 securityPolicy, securityMode, authMethod string) {
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
	
	// Connect to OPCUA server with infinite retries
	connectWithRetry(ctx, endpoint, username, password, certfile, keyfile, gencert, appuri, timeout)

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
            clientMutex.Lock()
            client := opcuaClient
            clientMutex.Unlock()
            
            if client == nil {
                log.Printf("[%s] Client is nil, attempting reconnection", connectionName)
                reconnectOPCUA(ctx, endpoint, username, password, certfile, keyfile, gencert, appuri, timeout)
                continue
            }
            
            // Try keep-alive
            timeNode := client.Node(ua.NewNumericNodeID(0, 2258))
            _, err := timeNode.Value(ctx)
            if err != nil {
                log.Printf("[%s] Keep-alive failed: %v", connectionName, err)
                reconnectOPCUA(ctx, endpoint, username, password, certfile, keyfile, gencert, appuri, timeout)
            } else if isVerbose {
                log.Printf("[%s] Keep-alive successful", connectionName)
            }
			
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
    
    // Get endpoints first to determine if we need certificates
    log.Printf("[%s] Getting endpoints...", connectionName)
    endpointCtx, cancel := context.WithTimeout(ctx, timeoutDuration)
    defer cancel()
    
    endpoints, err := opcua.GetEndpoints(endpointCtx, endpoint)
    if err != nil {
        return fmt.Errorf("failed to get endpoints: %v", err)
    }
    log.Printf("[%s] Found %d endpoints", connectionName, len(endpoints))


    // Add detailed endpoint logging
    log.Printf("[%s] Available endpoints:", connectionName)
    for i, e := range endpoints {
        log.Printf("[%s]   [%d] SecurityPolicy=%s, SecurityMode=%s, TokenTypes=%v", 
            connectionName, i, 
            e.SecurityPolicyURI, 
            e.SecurityMode,
            getTokenTypes(e.UserIdentityTokens))
    }

    
    // Determine security policy and mode from available endpoints
    var serverEndpoint *ua.EndpointDescription
    var useAnonymous bool
    
    // First check if server supports anonymous authentication with no security
    for _, e := range endpoints {
        if e.SecurityPolicyURI == ua.SecurityPolicyURINone && 
           e.SecurityMode == ua.MessageSecurityModeNone {
            // Check if it supports anonymous authentication
            for _, t := range e.UserIdentityTokens {
                if t.TokenType == ua.UserTokenTypeAnonymous {
                    serverEndpoint = e
                    useAnonymous = true
                    break
                }
            }
            if serverEndpoint != nil {
                break
            }
        }
    }
    
    // If no anonymous endpoint was found, look for username authentication
    if serverEndpoint == nil && username != "" {
        // Try to find an endpoint that supports username authentication
        for _, e := range endpoints {
            // Prefer Basic256 with SignAndEncrypt if available
            if e.SecurityPolicyURI == ua.SecurityPolicyURIBasic256 && 
               e.SecurityMode == ua.MessageSecurityModeSignAndEncrypt {
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
        
        // If no preferred endpoint found, try any security policy that supports username
        if serverEndpoint == nil {
            for _, e := range endpoints {
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
    }
    
    // If still no endpoint found, try to use anonymous authentication as fallback
    if serverEndpoint == nil {
        for _, e := range endpoints {
            for _, t := range e.UserIdentityTokens {
                if t.TokenType == ua.UserTokenTypeAnonymous {
                    serverEndpoint = e
                    useAnonymous = true
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
    
    // Determine if we need certificates
    needCertificates := serverEndpoint.SecurityPolicyURI != ua.SecurityPolicyURINone &&
                       serverEndpoint.SecurityMode != ua.MessageSecurityModeNone
    
    // Generate or load certificates if needed
    var cert []byte
    var privateKey *rsa.PrivateKey
    
    if needCertificates {
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
    }
    
    // Build client options with more aggressive timeouts for reconnection
    opts := []opcua.Option{
        opcua.DialTimeout(timeoutDuration),
        opcua.RequestTimeout(timeoutDuration),
        opcua.SessionTimeout(timeoutDuration * 2), // Longer session timeout
        opcua.AutoReconnect(true), 
    }
    
    // Add security options
    if useAnonymous {
        log.Printf("[%s] Using anonymous authentication", connectionName)
        opts = append(opts, opcua.SecurityFromEndpoint(serverEndpoint, ua.UserTokenTypeAnonymous))
    } else {
        log.Printf("[%s] Using username authentication", connectionName)
        opts = append(opts, 
            opcua.AuthUsername(username, password),
            opcua.SecurityFromEndpoint(serverEndpoint, ua.UserTokenTypeUserName))
    }
    
    // Add certificate options if needed
    if needCertificates {
        opts = append(opts,
            opcua.Certificate(cert),
            opcua.PrivateKey(privateKey))
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


// connectWithRetry attempts to connect with infinite retries and exponential backoff with jitter
func connectWithRetry(ctx context.Context, endpoint, username, password, certfile, keyfile string,
                      gencert bool, appuri string, timeout int) {
    // Seed random number generator with current time
    rnd := rand.New(rand.NewSource(time.Now().UnixNano()))

    // Add initial random jitter (0-300 seconds = 5 minutes) to desynchronize containers that start simultaneously
    // With ~19 containers and connection attempts taking up to 5 minutes, this spreads the load significantly
    initialJitter := time.Duration(rnd.Intn(300)) * time.Second
    log.Printf("[%s] Adding initial jitter of %v to desynchronize startup", connectionName, initialJitter)

    select {
    case <-time.After(initialJitter):
        // Continue to connection attempts
    case <-ctx.Done():
        log.Printf("[%s] Context cancelled during initial jitter", connectionName)
        return
    }

    attempt := 0
    for {
        // Check if context is cancelled
        if ctx.Err() != nil {
            log.Printf("[%s] Context cancelled, stopping connection attempts", connectionName)
            return
        }

        attempt++
        log.Printf("[%s] Initial connection attempt %d...", connectionName, attempt)

        // Create a fresh context for each attempt
        connectTimeout := time.Duration(timeout) * time.Second
        connectCtx, cancel := context.WithTimeout(context.Background(), connectTimeout)

        err := connectOPCUA(connectCtx, endpoint, username, password, certfile, keyfile, gencert, appuri, timeout)
        cancel()

        if err == nil {
            log.Printf("[%s] Successfully connected on attempt %d", connectionName, attempt)
            return
        }

        log.Printf("[%s] Connection attempt %d failed: %v", connectionName, attempt, err)

        // Calculate exponential backoff, capped at 180 seconds (3 minutes)
        // Given that connection attempts can take up to 5 minutes, we want reasonable spacing
        backoffExponent := attempt - 1
        if backoffExponent > 7 {
            backoffExponent = 7  // Cap at 2^7 = 128 seconds
        }
        baseBackoff := time.Duration(1<<uint(backoffExponent)) * time.Second
        if baseBackoff > 180*time.Second {
            baseBackoff = 180 * time.Second
        }

        // Add random jitter (±50%) to prevent synchronized retry storms
        jitterPercent := 0.5 + rnd.Float64()  // Random value between 0.5 and 1.5 (±50% around 1.0)
        backoffTime := time.Duration(float64(baseBackoff) * jitterPercent)

        log.Printf("[%s] Waiting %v (base: %v + jitter) before retry attempt %d...",
            connectionName, backoffTime, baseBackoff, attempt+1)

        // Sleep with context awareness
        select {
        case <-time.After(backoffTime):
            // Continue to next attempt
        case <-ctx.Done():
            log.Printf("[%s] Context cancelled during backoff, stopping connection attempts", connectionName)
            return
        }
    }
}

func reconnectOPCUA(ctx context.Context, endpoint, username, password, certfile, keyfile string,
                   gencert bool, appuri string, timeout int) {
    log.Printf("[%s] Attempting to reconnect...", connectionName)

    // At the start of reconnectOPCUA
    if ctx.Err() != nil {
        log.Printf("[%s] Context already cancelled, skipping reconnection", connectionName)
        return
    }

    // Force close existing connection if any
    clientMutex.Lock()
    if opcuaClient != nil {
        log.Printf("[%s] Closing existing connection...", connectionName)
        // Ensure the connection is fully closed, ignore errors
        opcuaClient.Close(ctx)
        // Important: Explicitly set to nil to ensure GC and complete cleanup
        opcuaClient = nil
    }
    clientMutex.Unlock()

    // Add a small delay to ensure server-side cleanup
    time.Sleep(2 * time.Second)

    // Seed random number generator for jitter
    rnd := rand.New(rand.NewSource(time.Now().UnixNano()))

    // Infinite retry loop with exponential backoff and jitter
    attempt := 0
    for {
        if ctx.Err() != nil {
            log.Printf("[%s] Context cancelled, stopping reconnection attempts", connectionName)
            return
        }

        attempt++
        log.Printf("[%s] Reconnection attempt %d...", connectionName, attempt)

        // Create a fresh context for each attempt
        reconnectTimeout := time.Duration(timeout) * time.Second
        reconnectCtx, cancel := context.WithTimeout(context.Background(), reconnectTimeout)

        // Complete new connection attempt
        err := connectOPCUA(reconnectCtx, endpoint, username, password, certfile, keyfile, gencert, appuri, timeout)
        cancel()

        if err == nil {
            log.Printf("[%s] Reconnection successful on attempt %d", connectionName, attempt)
            return
        }

        log.Printf("[%s] Reconnection attempt %d failed: %v", connectionName, attempt, err)

        // Calculate exponential backoff with jitter, capped at 180 seconds
        backoffExponent := attempt - 1
        if backoffExponent > 7 {
            backoffExponent = 7  // Cap at 2^7 = 128 seconds
        }
        baseBackoff := time.Duration(1<<uint(backoffExponent)) * time.Second
        if baseBackoff > 180*time.Second {
            baseBackoff = 180 * time.Second
        }

        // Add random jitter (±50%)
        jitterPercent := 0.5 + rnd.Float64()
        backoffTime := time.Duration(float64(baseBackoff) * jitterPercent)

        log.Printf("[%s] Waiting %v (base: %v + jitter) before reconnection attempt %d...",
            connectionName, backoffTime, baseBackoff, attempt+1)

        // Sleep with context awareness
        select {
        case <-time.After(backoffTime):
            // Continue to next attempt
        case <-ctx.Done():
            log.Printf("[%s] Context cancelled during reconnection backoff", connectionName)
            return
        }
    }
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
        // Check if this might be a DTL node (error indicates ExtensionObject decode failure)
        if strings.Contains(err.Error(), "extension object") || strings.Contains(err.Error(), "data type id") {
            // Try reading as DTL
            dtlValue, dtlErr := readDTLFields(ctx, client, id)
            if dtlErr == nil {
                sendJSONResponse(w, NodeResponse{
                    NodeID: nodeIDStr,
                    Value:  dtlValue,
                })
                return
            }
        }

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

    case "dtl":
        year, month, day, weekday, hour, minute, second, nanosecond, err := parseDTL(writeRequest.Value)
        if err != nil {
            sendJSONResponse(w, NodeResponse{
                NodeID: nodeIDStr,
                Error:  fmt.Sprintf("Invalid DTL format: %v", err),
            })
            return
        }

        // Write DTL by setting individual child fields
        err = writeDTLFields(ctx, client, id, year, month, day, weekday, hour, minute, second, nanosecond)
        if err != nil {
            sendJSONResponse(w, NodeResponse{
                NodeID: nodeIDStr,
                Error:  fmt.Sprintf("Failed to write DTL: %v", err),
            })
            return
        }

        // DTL write succeeded, return success
        sendJSONResponse(w, NodeResponse{
            NodeID: nodeIDStr,
            Value:  writeRequest.Value,
        })
        return

    default:
        sendJSONResponse(w, NodeResponse{
            NodeID: nodeIDStr,
            Error:  fmt.Sprintf("Unsupported data type: %s. Use one of: boolean, sbyte, byte, int16, uint16, int32, uint32, int64, uint64, float, double, string, dtl", writeRequest.DataType),
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



// Helper function to add at the end of the file
func getTokenTypes(tokens []*ua.UserTokenPolicy) []string {
    var types []string
    for _, t := range tokens {
        switch t.TokenType {
        case ua.UserTokenTypeAnonymous:
            types = append(types, "Anonymous")
        case ua.UserTokenTypeUserName:
            types = append(types, "Username")
        case ua.UserTokenTypeCertificate:
            types = append(types, "Certificate")
        case ua.UserTokenTypeIssuedToken:
            types = append(types, "IssuedToken")
        default:
            types = append(types, fmt.Sprintf("Unknown(%d)", t.TokenType))
        }
    }
    return types
}

// parseDTL parses ISO 8601 datetime string to Siemens DTL components
// Accepts: "2025-03-09T14:30:00" or "2025-03-09 14:30:00"
// Returns: year, month, day, weekday, hour, minute, second, nanosecond, error
func parseDTL(dtlStr string) (uint16, uint8, uint8, uint8, uint8, uint8, uint8, uint32, error) {
	// Try RFC3339 with timezone first: "2025-03-09T14:30:00Z"
	t, err := time.Parse(time.RFC3339, dtlStr)
	if err != nil {
		// Try ISO 8601 without timezone: "2025-03-09T14:30:00"
		t, err = time.Parse("2006-01-02T15:04:05", dtlStr)
		if err != nil {
			// Try space-separated format: "2025-03-09 14:30:00"
			t, err = time.Parse("2006-01-02 15:04:05", dtlStr)
			if err != nil {
				return 0, 0, 0, 0, 0, 0, 0, 0, fmt.Errorf("invalid datetime format. Use: 2025-03-09T14:30:00")
			}
		}
	}

	// Extract components
	year := uint16(t.Year())
	month := uint8(t.Month())
	day := uint8(t.Day())
	hour := uint8(t.Hour())
	minute := uint8(t.Minute())
	second := uint8(t.Second())
	nanosecond := uint32(0) // Keep it simple, don't use sub-second precision

	// Calculate weekday: Go's Sunday=0, Siemens expects Sunday=1
	weekday := uint8(t.Weekday() + 1)
	if weekday > 7 {
		weekday = 1
	}

	return year, month, day, weekday, hour, minute, second, nanosecond, nil
}

// writeDTLFields writes DTL values to individual child fields
// DTL child fields follow pattern: parent_id+1 (YEAR), parent_id+2 (MONTH), etc.
func writeDTLFields(ctx context.Context, client *opcua.Client, parentID *ua.NodeID, year uint16, month, day, weekday, hour, minute, second uint8, nanosecond uint32) error {
	// Extract namespace and identifier from parent node
	namespace := parentID.Namespace()
	identifier := parentID.IntID()

	// Create write values for all 8 DTL child fields
	// Child field offsets: YEAR(+1), MONTH(+2), DAY(+3), WEEKDAY(+4), HOUR(+5), MINUTE(+6), SECOND(+7), NANOSECOND(+8)
	writeValues := []*ua.WriteValue{
		{
			NodeID:      ua.NewNumericNodeID(namespace, identifier+1), // YEAR
			AttributeID: ua.AttributeIDValue,
			Value: &ua.DataValue{
				EncodingMask: ua.DataValueValue,
				Value:        mustVariant(ua.NewVariant(year)),
			},
		},
		{
			NodeID:      ua.NewNumericNodeID(namespace, identifier+2), // MONTH
			AttributeID: ua.AttributeIDValue,
			Value: &ua.DataValue{
				EncodingMask: ua.DataValueValue,
				Value:        mustVariant(ua.NewVariant(month)),
			},
		},
		{
			NodeID:      ua.NewNumericNodeID(namespace, identifier+3), // DAY
			AttributeID: ua.AttributeIDValue,
			Value: &ua.DataValue{
				EncodingMask: ua.DataValueValue,
				Value:        mustVariant(ua.NewVariant(day)),
			},
		},
		{
			NodeID:      ua.NewNumericNodeID(namespace, identifier+4), // WEEKDAY
			AttributeID: ua.AttributeIDValue,
			Value: &ua.DataValue{
				EncodingMask: ua.DataValueValue,
				Value:        mustVariant(ua.NewVariant(weekday)),
			},
		},
		{
			NodeID:      ua.NewNumericNodeID(namespace, identifier+5), // HOUR
			AttributeID: ua.AttributeIDValue,
			Value: &ua.DataValue{
				EncodingMask: ua.DataValueValue,
				Value:        mustVariant(ua.NewVariant(hour)),
			},
		},
		{
			NodeID:      ua.NewNumericNodeID(namespace, identifier+6), // MINUTE
			AttributeID: ua.AttributeIDValue,
			Value: &ua.DataValue{
				EncodingMask: ua.DataValueValue,
				Value:        mustVariant(ua.NewVariant(minute)),
			},
		},
		{
			NodeID:      ua.NewNumericNodeID(namespace, identifier+7), // SECOND
			AttributeID: ua.AttributeIDValue,
			Value: &ua.DataValue{
				EncodingMask: ua.DataValueValue,
				Value:        mustVariant(ua.NewVariant(second)),
			},
		},
		{
			NodeID:      ua.NewNumericNodeID(namespace, identifier+8), // NANOSECOND
			AttributeID: ua.AttributeIDValue,
			Value: &ua.DataValue{
				EncodingMask: ua.DataValueValue,
				Value:        mustVariant(ua.NewVariant(nanosecond)),
			},
		},
	}

	// Execute the batch write operation
	req := &ua.WriteRequest{
		NodesToWrite: writeValues,
	}

	resp, err := client.Write(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to write DTL fields: %v", err)
	}

	// Check all write results
	for i, status := range resp.Results {
		if status != ua.StatusOK {
			fieldNames := []string{"YEAR", "MONTH", "DAY", "WEEKDAY", "HOUR", "MINUTE", "SECOND", "NANOSECOND"}
			return fmt.Errorf("failed to write %s field: %v", fieldNames[i], status)
		}
	}

	return nil
}

// readDTLFields reads DTL values from individual child fields and formats as datetime string
func readDTLFields(ctx context.Context, client *opcua.Client, parentID *ua.NodeID) (string, error) {
	namespace := parentID.Namespace()
	identifier := parentID.IntID()

	// Read all 8 child fields
	req := &ua.ReadRequest{
		NodesToRead: []*ua.ReadValueID{
			{NodeID: ua.NewNumericNodeID(namespace, identifier+1), AttributeID: ua.AttributeIDValue}, // YEAR
			{NodeID: ua.NewNumericNodeID(namespace, identifier+2), AttributeID: ua.AttributeIDValue}, // MONTH
			{NodeID: ua.NewNumericNodeID(namespace, identifier+3), AttributeID: ua.AttributeIDValue}, // DAY
			{NodeID: ua.NewNumericNodeID(namespace, identifier+4), AttributeID: ua.AttributeIDValue}, // WEEKDAY
			{NodeID: ua.NewNumericNodeID(namespace, identifier+5), AttributeID: ua.AttributeIDValue}, // HOUR
			{NodeID: ua.NewNumericNodeID(namespace, identifier+6), AttributeID: ua.AttributeIDValue}, // MINUTE
			{NodeID: ua.NewNumericNodeID(namespace, identifier+7), AttributeID: ua.AttributeIDValue}, // SECOND
			{NodeID: ua.NewNumericNodeID(namespace, identifier+8), AttributeID: ua.AttributeIDValue}, // NANOSECOND
		},
	}

	resp, err := client.Read(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to read DTL fields: %v", err)
	}

	// Extract values
	if len(resp.Results) != 8 {
		return "", fmt.Errorf("expected 8 DTL fields, got %d", len(resp.Results))
	}

	year, _ := resp.Results[0].Value.Value().(uint16)
	month, _ := resp.Results[1].Value.Value().(uint8)
	day, _ := resp.Results[2].Value.Value().(uint8)
	// weekday := resp.Results[3].Value.Value().(uint8) // not needed for formatting
	hour, _ := resp.Results[4].Value.Value().(uint8)
	minute, _ := resp.Results[5].Value.Value().(uint8)
	second, _ := resp.Results[6].Value.Value().(uint8)
	// nanosecond := resp.Results[7].Value.Value().(uint32) // not used in output

	// Format as ISO 8601 datetime string
	dtlTime := time.Date(int(year), time.Month(month), int(day), int(hour), int(minute), int(second), 0, time.UTC)
	return dtlTime.Format("2006-01-02T15:04:05"), nil
}

// Helper to unwrap variant (assumes variant creation never fails for simple types)
func mustVariant(v *ua.Variant, err error) *ua.Variant {
	if err != nil {
		panic(err)
	}
	return v
}
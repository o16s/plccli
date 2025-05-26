package main

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "os"
    "strings"
    "text/tabwriter"
    "time"

    "github.com/gopcua/opcua"
    "github.com/gopcua/opcua/id"
    "github.com/gopcua/opcua/ua"
)

// NodeInfo represents discovered node information
type NodeInfo struct {
	NodeID      *ua.NodeID
	NodeClass   ua.NodeClass
	BrowseName  string
	Description string
	AccessLevel ua.AccessLevelType
	Path        string
	DataType    string
	Writable    bool
}

// getEndpointTag gets a cleaned endpoint tag for InfluxDB format
func getEndpointTag(port int) string {
    // Get connection info to extract endpoint
    info, err := getConnectionInfo(port)
    if err != nil {
        return "unknown"
    }
    
    endpoint, ok := info["endpoint"].(string)
    if !ok {
        return "unknown"
    }
    
    // Clean endpoint for tags - only replace characters not allowed in InfluxDB tags
	tagEscaper := strings.NewReplacer(
		",", "\\,",
		"=", "\\=",
		" ", "\\ ",
	)

	cleanEndpoint := tagEscaper.Replace(endpoint)
    return cleanEndpoint
}

// Browse nodes from the OPC UA server using the HTTP service
func browseNode(startNodeID string, maxDepth int, port int, format string) error {
    // Create a client with timeout
    client := &http.Client{
        Timeout: 120 * time.Second,
    }
    
    // Build the request URL with port
    reqURL := fmt.Sprintf("http://localhost:%d/api/browse?nodeid=%s&maxdepth=%d", 
        port, url.QueryEscape(startNodeID), maxDepth)
    
    // Make the request
    resp, err := client.Get(reqURL)
    if err != nil {
        return fmt.Errorf("cannot connect to OPCUA service on port %d: %v (is it running?)", port, err)
    }
    defer resp.Body.Close()
    
    // Read response body
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return fmt.Errorf("error reading response: %v", err)
    }
    
    // Check HTTP status
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("service error: %s", body)
    }
    
    // Parse the JSON response
    var browseResp struct {
        Nodes []struct {
            NodeId      string `json:"nodeId"`
            BrowseName  string `json:"browseName"`
            Path        string `json:"path"`
            DataType    string `json:"dataType"`
            Writable    bool   `json:"writable"`
            Description string `json:"description"`
        } `json:"nodes"`
        Error string `json:"error,omitempty"`
    }
    
    if err := json.Unmarshal(body, &browseResp); err != nil {
        return fmt.Errorf("error parsing response: %v", err)
    }
    
    // Check for errors in the response
    if browseResp.Error != "" {
        return fmt.Errorf("service reported error: %s", browseResp.Error)
    }
    
     // Check format and print results accordingly
    if format == "influx" {
        // Print results in InfluxDB Line Protocol format
        timestamp := time.Now().UnixNano()
        
        for _, node := range browseResp.Nodes {
            // Clean up names for InfluxDB compatibility
            measurementName := "opcua_node"
            nodePath := strings.Replace(node.Path, " ", "_", -1)
            nodePath = strings.Replace(nodePath, ".", "_", -1)
            nodeId := strings.Replace(node.NodeId, ";", "_", -1)
            nodeId = strings.Replace(nodeId, "=", "", -1)
            nodeId = strings.Replace(nodeId, ",", "_", -1)
            
            // Get endpoint for the connection
            endpointTag := getEndpointTag(port)
            
            // Generate line protocol format
            // measurement,tag1=value1,tag2=value2 field1=value1,field2=value2 timestamp
            fmt.Printf("%s,node_id=%s,path=%s,data_type=%s,endpoint=%s writable=%v,description=\"%s\" %d\n",
                measurementName,
                nodeId,
                nodePath,
                node.DataType,
                endpointTag,
                node.Writable,
                strings.Replace(node.Description, "\"", "\\\"", -1),
                timestamp)
        }
    } else {
        // Original tabular format
        w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
        fmt.Fprintln(w, "Path\tNodeID\tDataType\tWritable\tDescription")
        fmt.Fprintln(w, "----\t------\t--------\t--------\t-----------")
        
        for _, node := range browseResp.Nodes {
            fmt.Fprintf(w, "%s\t%s\t%s\t%v\t%s\n",
                node.Path,
                node.NodeId,
                node.DataType,
                node.Writable,
                strings.ReplaceAll(node.Description, "\n", " "))
        }
        w.Flush()
    }
    
    return nil
}

// This function will be called from service.go to perform the actual browse
func doBrowse(ctx context.Context, client *opcua.Client, startNodeID string, maxDepth int) ([]NodeInfo, error) {
	id, err := ua.ParseNodeID(startNodeID)
	if err != nil {
		return nil, fmt.Errorf("invalid node id: %v", err)
	}

	// Create root node
	n := client.Node(id)
	
	// Perform browse operation recursively
	nodes, err := browseRecursive(ctx, n, "", 0, maxDepth)
	if err != nil {
		return nil, err
	}
	
	return nodes, nil
}

// Recursive function to browse nodes
func browseRecursive(ctx context.Context, n *opcua.Node, path string, level, maxDepth int) ([]NodeInfo, error) {
	if level > maxDepth {
		return nil, nil
	}

	// Get node attributes
	attrs, err := n.Attributes(ctx, 
		ua.AttributeIDNodeClass, 
		ua.AttributeIDBrowseName, 
		ua.AttributeIDDescription, 
		ua.AttributeIDAccessLevel, 
		ua.AttributeIDDataType)
	if err != nil {
		return nil, err
	}

	// Create node info
	var info = NodeInfo{
		NodeID: n.ID,
	}

	// Extract NodeClass
	if attrs[0].Status == ua.StatusOK {
		info.NodeClass = ua.NodeClass(attrs[0].Value.Int())
	}

	// Extract BrowseName
	if attrs[1].Status == ua.StatusOK {
		info.BrowseName = attrs[1].Value.String()
	}

	// Extract Description
	if attrs[2].Status == ua.StatusOK {
		info.Description = attrs[2].Value.String()
	}

	// Extract AccessLevel
	if attrs[3].Status == ua.StatusOK {
		info.AccessLevel = ua.AccessLevelType(attrs[3].Value.Int())
		info.Writable = info.AccessLevel&ua.AccessLevelTypeCurrentWrite == ua.AccessLevelTypeCurrentWrite
	}

	// Extract DataType
	if attrs[4].Status == ua.StatusOK {
		switch v := attrs[4].Value.NodeID().IntID(); v {
		case id.DateTime, id.UtcTime:
			info.DataType = "time.Time"
		case id.Boolean:
			info.DataType = "bool"
		case id.SByte:
			info.DataType = "int8"
		case id.Int16:
			info.DataType = "int16"
		case id.Int32:
			info.DataType = "int32"
		case id.Byte:
			info.DataType = "byte"
		case id.UInt16:
			info.DataType = "uint16"
		case id.UInt32:
			info.DataType = "uint32"
		case id.String:
			info.DataType = "string"
		case id.Float:
			info.DataType = "float32"
		case id.Double:
			info.DataType = "float64"
		default:
			info.DataType = attrs[4].Value.NodeID().String()
		}
	}

	// Set path
	info.Path = joinPath(path, info.BrowseName)

	// Store results
	var nodes []NodeInfo
	if info.NodeClass == ua.NodeClassVariable {
		nodes = append(nodes, info)
	}

	// Browse child nodes
	browseChildren := func(refType uint32) error {
		refs, err := n.ReferencedNodes(ctx, refType, ua.BrowseDirectionForward, ua.NodeClassAll, true)
		if err != nil {
			return fmt.Errorf("references lookup error: %v", err)
		}
		
		for _, rn := range refs {
			children, err := browseRecursive(ctx, rn, info.Path, level+1, maxDepth)
			if err != nil {
				return fmt.Errorf("browse children error: %v", err)
			}
			nodes = append(nodes, children...)
		}
		return nil
	}

	// Browse different reference types
	if err := browseChildren(id.HasComponent); err != nil {
		return nil, err
	}
	if err := browseChildren(id.Organizes); err != nil {
		return nil, err
	}
	if err := browseChildren(id.HasProperty); err != nil {
		return nil, err
	}

	return nodes, nil
}

// Helper to join path components
func joinPath(a, b string) string {
	if a == "" {
		return b
	}
	return a + "." + b
}
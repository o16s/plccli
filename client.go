package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// parseNodeID extracts namespace, type and identifier from an OPC UA node ID
func parseNodeID(nodeID string) (string, string, string, error) {
	// Expected formats: ns=X,Y=Z or ns=X;Y=Z
	var namespace, idType, identifier string
	
	// Determine which separator is used (comma or semicolon)
	var parts []string
	if strings.Contains(nodeID, ",") {
		parts = strings.Split(nodeID, ",")
	} else if strings.Contains(nodeID, ";") {
		parts = strings.Split(nodeID, ";")
	} else {
		return "", "", "", fmt.Errorf("invalid node ID format. Expected format: ns=X,Y=Z or ns=X;Y=Z")
	}
	
	// Extract components
	if len(parts) == 2 {
		// Extract namespace
		nsParts := strings.Split(parts[0], "=")
		if len(nsParts) == 2 && nsParts[0] == "ns" {
			namespace = nsParts[1]
		}
		
		// Extract type and identifier
		idParts := strings.Split(parts[1], "=")
		if len(idParts) == 2 {
			idType = idParts[0]
			identifier = idParts[1]
		}
	}
	
	if namespace == "" || idType == "" || identifier == "" {
		return "", "", "", fmt.Errorf("invalid node ID format. Expected format: ns=X,Y=Z or ns=X;Y=Z where Y is 'i' or 's'")
	}
	
	// Validate that idType is either 'i' or 's'
	if idType != "i" && idType != "s" {
		return "", "", "", fmt.Errorf("unsupported identifier type '%s'. Only 'i' (numeric) and 's' (string) are supported", idType)
	}
	
	return namespace, idType, identifier, nil
}

// formatInfluxOutput converts a value to InfluxDB Line Protocol format
func formatInfluxOutput(measurementName, nodeID string, value interface{}, dataType string, endpoint string) string {
    tagEscaper := strings.NewReplacer(
        ",", "\\,",
        "=", "\\=",
        " ", "\\ ",
        "\"", "\\\"",
    )

    // Clean up names for InfluxDB compatibility
    cleanNodeID := tagEscaper.Replace(nodeID)
    cleanEndpoint := tagEscaper.Replace(endpoint)

    // Handle different value types - FIXED TO OUTPUT NUMERIC VALUES
    var valueStr string
    switch v := value.(type) {
    case string:
        // Try to parse timestamp strings to unix time
        if t, err := time.Parse("2006-01-02T15:04:05.999999Z", v); err == nil {
            // Convert timestamp to unix nanoseconds (numeric)
            valueStr = fmt.Sprintf("value=%d", t.UnixNano())
        } else if t, err := time.Parse("2006-01-02T15:04:05Z", v); err == nil {
            // Try without microseconds
            valueStr = fmt.Sprintf("value=%d", t.UnixNano())
        } else {
            // For non-timestamp strings, create a constant numeric value and keep string as tag
            valueStr = fmt.Sprintf("value=1,string_value=\"%s\"", strings.Replace(v, "\"", "\\\"", -1))
        }
    case bool:
        // Convert boolean to numeric (0 or 1)
        if v {
            valueStr = "value=1"
        } else {
            valueStr = "value=0"
        }
    case float64, float32, int, int32, int64, uint, uint32, uint64:
        valueStr = fmt.Sprintf("value=%v", v)
    default:
        // Fallback: convert to string and add numeric constant
        valueStr = fmt.Sprintf("value=1,string_value=\"%v\"", v)
    }
    
    timestamp := time.Now().UnixNano()
    return fmt.Sprintf("%s,node_id=%s,endpoint=%s %s %d",
        measurementName,
        cleanNodeID,
        cleanEndpoint,
        valueStr,
        timestamp)
}

// formatInfluxOutputWithBits formats a uint32 value with bit expansion for InfluxDB
// Returns a slice of InfluxDB line protocol strings, one for each of the 32 bits
func formatInfluxOutputWithBits(measurementName, nodeID string, value interface{}, endpoint string, bitNames []string) ([]string, error) {
	tagEscaper := strings.NewReplacer(
		",", "\\,",
		"=", "\\=",
		" ", "\\ ",
		"\"", "\\\"",
	)

	// Convert value to uint32
	var uint32Value uint32
	switch v := value.(type) {
	case float64:
		uint32Value = uint32(v)
	case float32:
		uint32Value = uint32(v)
	case int:
		uint32Value = uint32(v)
	case int32:
		uint32Value = uint32(v)
	case int64:
		uint32Value = uint32(v)
	case uint:
		uint32Value = uint32(v)
	case uint32:
		uint32Value = v
	case uint64:
		uint32Value = uint32(v)
	default:
		return nil, fmt.Errorf("value type %T cannot be converted to uint32 for bit extraction", value)
	}

	// Extract all 32 bits
	bits, err := extractBits(uint32Value, bitNames)
	if err != nil {
		return nil, err
	}

	// Format each bit as a separate InfluxDB line
	cleanNodeID := tagEscaper.Replace(nodeID)
	cleanEndpoint := tagEscaper.Replace(endpoint)
	timestamp := time.Now().UnixNano()

	lines := make([]string, 0, len(bits))
	for _, bit := range bits {
		cleanBitName := tagEscaper.Replace(bit.Name)
		line := fmt.Sprintf("%s,node_id=%s,endpoint=%s,bit=%d,bit_name=%s value=%d %d",
			measurementName,
			cleanNodeID,
			cleanEndpoint,
			bit.BitNum,
			cleanBitName,
			bit.Value,
			timestamp)
		lines = append(lines, line)
	}

	return lines, nil
}

func setNodeValue(nodeID string, value string, dataType string, host string, port int, format string) (string, error) {
	namespace, idType, identifier, err := parseNodeID(nodeID)
	if err != nil {
		return "", err
	}
	
	// Data type is REQUIRED
	if dataType == "" {
		return "", fmt.Errorf("data type is required for writing values. Use one of: boolean, sbyte, byte, int16, uint16, int32, uint32, int64, uint64, float, double, string")
	}
	
	// Prepare the request body
	requestBody := map[string]interface{}{
		"namespace":  namespace,
		"type":       idType,
		"identifier": identifier,
		"value":      value,
		"dataType":   dataType,
	}
	
	// Convert request to JSON
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}
	
	// Build the request URL with host and port
	reqURL := fmt.Sprintf("http://%s:%d/api/node", host, port)
	
	// Create a client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	
	// Make the POST request
	resp, err := client.Post(reqURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		// Enhanced error message with connection details
		return "", fmt.Errorf("cannot connect to OPCUA service on %s:%d: %v (is it running?)", host, port, err)
	}
	defer resp.Body.Close()
	
	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response: %v", err)
	}
	
	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("service error: %s", body)
	}
	
	// Parse the JSON response
	var nodeResp NodeResponse
	if err := json.Unmarshal(body, &nodeResp); err != nil {
		return "", fmt.Errorf("error parsing response: %v", err)
	}
	
	// Check for errors in the response
	if nodeResp.Error != "" {
		return "", fmt.Errorf("service reported error: %s", nodeResp.Error)
	}
	
	// Get endpoint for the connection
	info, err := getConnectionInfo(host, port)
	if err != nil {
		// If we can't get the endpoint, just use a placeholder
		info = map[string]interface{}{"endpoint": "unknown"}
	}
	endpoint, _ := info["endpoint"].(string)
	
	if format == "influx" {
		return formatInfluxOutput("opcua_set", nodeID, value, dataType, endpoint), nil
	}

	// Original format
	return fmt.Sprintf("Successfully set %s to %v with type %s (via %s:%d)", nodeID, nodeResp.Value, dataType, host, port), nil
}

func getNodeValues(nodeIDs []string, host string, port int, format string, measurement string, extractBits bool, bitNamesStr string) (string, error) {
	if len(nodeIDs) == 0 {
		return "", fmt.Errorf("no node IDs provided")
	}

	// Parse bit names if provided
	var bitNames []string
	if bitNamesStr != "" {
		bitNames = strings.Split(bitNamesStr, ",")
		// Trim whitespace from each name
		for i := range bitNames {
			bitNames[i] = strings.TrimSpace(bitNames[i])
		}
		// Validate bit names
		if err := validateBitNames(bitNames); err != nil {
			return "", err
		}
	}

	// Get endpoint for the connection
	info, err := getConnectionInfo(host, port)
	if err != nil {
		// If we can't get the endpoint, just use a placeholder
		info = map[string]interface{}{"endpoint": "unknown"}
	}
	endpoint, _ := info["endpoint"].(string)

	// If there's only one node ID, use the existing method
	if len(nodeIDs) == 1 {
		return getNodeValue(nodeIDs[0], host, port, format, endpoint, measurement, extractBits, bitNames)
	}
	
	// For multiple nodes, build a batch request
	var requestParams []map[string]string
	
	for _, nodeID := range nodeIDs {
		namespace, idType, identifier, err := parseNodeID(nodeID)
		if err != nil {
			return "", err
		}
		
		requestParams = append(requestParams, map[string]string{
			"namespace":  namespace,
			"type":       idType,
			"identifier": identifier,
		})
	}
	
	// Convert request to JSON
	jsonData, err := json.Marshal(map[string]interface{}{
		"nodes": requestParams,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}
	
	// Build the request URL with host and port
	reqURL := fmt.Sprintf("http://%s:%d/api/nodes", host, port)
	
	// Create a client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	
	// Make the POST request
	resp, err := client.Post(reqURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		// Enhanced error message with connection details
		return "", fmt.Errorf("cannot connect to OPCUA service on %s:%d: %v (is it running?)", host, port, err)
	}
	defer resp.Body.Close()
	
	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response: %v", err)
	}
	
	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("service error: %s", body)
	}
	
	// Parse the JSON response
	var batchResp struct {
		Results []NodeResponse `json:"results"`
		Error   string         `json:"error,omitempty"`
	}
	
	if err := json.Unmarshal(body, &batchResp); err != nil {
		return "", fmt.Errorf("error parsing response: %v", err)
	}
	
	// Check for errors in the response
	if batchResp.Error != "" {
		return "", fmt.Errorf("service reported error: %s", batchResp.Error)
	}
	
	// Format the output based on the desired format
	if format == "influx" {
		var lines []string
		for i, result := range batchResp.Results {
			if result.Error != "" {
				continue // Skip nodes with errors
			}

			// Check if bit expansion is requested
			if extractBits {
				bitLines, err := formatInfluxOutputWithBits(measurement, nodeIDs[i], result.Value, endpoint, bitNames)
				if err != nil {
					return "", fmt.Errorf("bit expansion failed for %s: %v", nodeIDs[i], err)
				}
				lines = append(lines, bitLines...)
			} else {
				lines = append(lines, formatInfluxOutput(measurement, nodeIDs[i], result.Value, "", endpoint))
			}
		}
		return strings.Join(lines, "\n"), nil
	}
	
	// Default format - just return the values
	var values []string
	for _, result := range batchResp.Results {
		if result.Error != "" {
			values = append(values, fmt.Sprintf("Error: %s", result.Error))
		} else {
			values = append(values, fmt.Sprintf("%v", result.Value))
		}
	}
	return strings.Join(values, "\n"), nil
}

func getNodeValue(nodeID string, host string, port int, format string, endpoint string, measurement string, extractBits bool, bitNames []string) (string, error) {
	namespace, idType, identifier, err := parseNodeID(nodeID)
	if err != nil {
		return "", err
	}
	
	// Build the request URL with host, port and parameters
	reqURL := fmt.Sprintf("http://%s:%d/api/node?namespace=%s&type=%s&identifier=%s", 
		host, port, url.QueryEscape(namespace), url.QueryEscape(idType), url.QueryEscape(identifier))
	
	// Create a client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	
	// Make the request
	resp, err := client.Get(reqURL)
	if err != nil {
		// Enhanced error message with connection details
		return "", fmt.Errorf("cannot connect to OPCUA service on %s:%d: %v (is it running?)", host, port, err)
	}
	defer resp.Body.Close()
	
	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response: %v", err)
	}
	
	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("service error: %s", body)
	}
	
	// Parse the JSON response
	var nodeResp NodeResponse
	if err := json.Unmarshal(body, &nodeResp); err != nil {
		return "", fmt.Errorf("error parsing response: %v", err)
	}
	
	// Check for errors in the response
	if nodeResp.Error != "" {
		return "", fmt.Errorf("service reported error: %s", nodeResp.Error)
	}
	
	if format == "influx" {
		// Check if bit expansion is requested
		if extractBits {
			bitLines, err := formatInfluxOutputWithBits(measurement, nodeID, nodeResp.Value, endpoint, bitNames)
			if err != nil {
				return "", fmt.Errorf("bit expansion failed: %v", err)
			}
			return strings.Join(bitLines, "\n"), nil
		}
		return formatInfluxOutput(measurement, nodeID, nodeResp.Value, "", endpoint), nil
	}

	// Original format
	return fmt.Sprintf("%v", nodeResp.Value), nil
}

// Add this function to get information about a connection
func getConnectionInfo(host string, port int) (map[string]interface{}, error) {
	// Create a client with timeout
	client := &http.Client{
		Timeout: 2 * time.Second,
	}
	
	// Build the request URL with host and port
	reqURL := fmt.Sprintf("http://%s:%d/api/info", host, port)
	
	// Make the request
	resp, err := client.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to OPCUA service on %s:%d: %v", host, port, err)
	}
	defer resp.Body.Close()
	
	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %v", err)
	}
	
	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("service error: %s", body)
	}
	
	// Parse the JSON response
	var info map[string]interface{}
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("error parsing response: %v", err)
	}
	
	return info, nil
}
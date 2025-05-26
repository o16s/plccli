package main

// Response format for API
type NodeResponse struct {
	NodeID string      `json:"nodeID"`
	Value  interface{} `json:"value"`
	Error  string      `json:"error,omitempty"`
}
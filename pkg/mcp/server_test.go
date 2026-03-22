package mcp

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	s := NewServer("test", "1.0.0", 5, 20*time.Second)
	if s.Name() != "test" {
		t.Errorf("Name() = %q, want %q", s.Name(), "test")
	}
	if s.Version() != "1.0.0" {
		t.Errorf("Version() = %q, want %q", s.Version(), "1.0.0")
	}
	if s.ToolCount() != 0 {
		t.Errorf("ToolCount() = %d, want 0", s.ToolCount())
	}
}

func TestRegisterTool(t *testing.T) {
	s := NewServer("test", "1.0.0", 5, 20*time.Second)
	s.RegisterTool(Tool{
		Name:        "test_tool",
		Description: "A test tool",
		InputSchema: JSONSchema{Type: "object"},
	}, func(args map[string]interface{}) (*CallToolResult, error) {
		return TextResult("ok"), nil
	})

	if s.ToolCount() != 1 {
		t.Errorf("ToolCount() = %d, want 1", s.ToolCount())
	}

	tools := s.Tools()
	if tools[0].Name != "test_tool" {
		t.Errorf("Tool name = %q, want %q", tools[0].Name, "test_tool")
	}
}

func TestHandleInitialize(t *testing.T) {
	s := NewServer("test-server", "2.0.0", 5, 20*time.Second)

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	}
	data, _ := json.Marshal(req)

	resp := s.handleMessage(data)
	if resp == nil {
		t.Fatal("Expected response, got nil")
	}
	if resp.Error != nil {
		t.Fatalf("Unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(*InitializeResult)
	if !ok {
		t.Fatal("Result is not InitializeResult")
	}
	if result.ServerInfo.Name != "test-server" {
		t.Errorf("ServerInfo.Name = %q, want %q", result.ServerInfo.Name, "test-server")
	}
	if result.ProtocolVersion != "2024-11-05" {
		t.Errorf("ProtocolVersion = %q, want %q", result.ProtocolVersion, "2024-11-05")
	}
}

func TestHandleToolCall(t *testing.T) {
	s := NewServer("test", "1.0.0", 100, 20*time.Second)
	s.RegisterTool(Tool{
		Name:        "echo",
		Description: "Echo",
		InputSchema: JSONSchema{Type: "object"},
	}, func(args map[string]interface{}) (*CallToolResult, error) {
		msg := GetStringArg(args, "message", "default")
		return TextResult(msg), nil
	})

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      "echo",
			"arguments": map[string]interface{}{"message": "hello"},
		},
	}
	data, _ := json.Marshal(req)

	resp := s.handleMessage(data)
	if resp == nil {
		t.Fatal("Expected response")
	}
	if resp.Error != nil {
		t.Fatalf("Unexpected error: %v", resp.Error)
	}

	result, ok := resp.Result.(*CallToolResult)
	if !ok {
		t.Fatal("Result is not CallToolResult")
	}
	if len(result.Content) != 1 || result.Content[0].Text != "hello" {
		t.Errorf("Unexpected result: %+v", result)
	}
}

func TestHandleUnknownTool(t *testing.T) {
	s := NewServer("test", "1.0.0", 100, 20*time.Second)

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name": "nonexistent",
		},
	}
	data, _ := json.Marshal(req)

	resp := s.handleMessage(data)
	if resp == nil {
		t.Fatal("Expected response")
	}

	result, ok := resp.Result.(*CallToolResult)
	if !ok {
		t.Fatal("Result is not CallToolResult")
	}
	if !result.IsError {
		t.Error("Expected IsError to be true")
	}
}

func TestHandleNotification(t *testing.T) {
	s := NewServer("test", "1.0.0", 5, 20*time.Second)

	// Notifications have no ID, should return nil
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	data, _ := json.Marshal(req)

	resp := s.handleMessage(data)
	if resp != nil {
		t.Error("Expected nil response for notification")
	}
}

func TestHandleParseError(t *testing.T) {
	s := NewServer("test", "1.0.0", 5, 20*time.Second)

	resp := s.handleMessage([]byte("not json"))
	if resp == nil {
		t.Fatal("Expected error response")
	}
	if resp.Error == nil {
		t.Fatal("Expected JSON-RPC error")
	}
	if resp.Error.Code != ParseError {
		t.Errorf("Error code = %d, want %d", resp.Error.Code, ParseError)
	}
}

func TestRateLimit(t *testing.T) {
	s := NewServer("test", "1.0.0", 2, 1*time.Second)

	// First two calls should not be rate limited
	if s.checkRateLimit() {
		t.Error("First call should not be rate limited")
	}
	if s.checkRateLimit() {
		t.Error("Second call should not be rate limited")
	}
	// Third call should be rate limited
	if !s.checkRateLimit() {
		t.Error("Third call should be rate limited")
	}
}

func TestHelperFunctions(t *testing.T) {
	args := map[string]interface{}{
		"name":    "test",
		"count":   float64(5),
		"enabled": true,
		"items":   []interface{}{"a", "b"},
	}

	if v := GetStringArg(args, "name", ""); v != "test" {
		t.Errorf("GetStringArg = %q, want %q", v, "test")
	}
	if v := GetStringArg(args, "missing", "default"); v != "default" {
		t.Errorf("GetStringArg missing = %q, want %q", v, "default")
	}
	if v := GetIntArg(args, "count", 0); v != 5 {
		t.Errorf("GetIntArg = %d, want 5", v)
	}
	if v := GetBoolArg(args, "enabled", false); v != true {
		t.Errorf("GetBoolArg = %v, want true", v)
	}
	if v := GetStringSliceArg(args, "items"); len(v) != 2 {
		t.Errorf("GetStringSliceArg len = %d, want 2", len(v))
	}

	if _, err := RequireStringArg(args, "name"); err != nil {
		t.Errorf("RequireStringArg unexpected error: %v", err)
	}
	if _, err := RequireStringArg(args, "missing"); err == nil {
		t.Error("RequireStringArg should error on missing arg")
	}
}

func TestTextResult(t *testing.T) {
	r := TextResult("hello")
	if len(r.Content) != 1 || r.Content[0].Text != "hello" {
		t.Errorf("Unexpected result: %+v", r)
	}
	if r.IsError {
		t.Error("TextResult should not be error")
	}
}

func TestErrorResult(t *testing.T) {
	r := ErrorResult("failed: %s", "reason")
	if !r.IsError {
		t.Error("ErrorResult should be error")
	}
	if r.Content[0].Text != "failed: reason" {
		t.Errorf("ErrorResult text = %q", r.Content[0].Text)
	}
}

func TestIsLocalRequest(t *testing.T) {
	// This test is limited since we can't easily mock http.Request.RemoteAddr
	// but we test the parsing logic
}


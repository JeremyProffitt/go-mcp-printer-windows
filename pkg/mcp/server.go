package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ToolHandler is a function that handles a tool call.
type ToolHandler func(arguments map[string]interface{}) (*CallToolResult, error)

// ResourceHandler is a function that returns the content of a resource.
type ResourceHandler func() (string, error)

// PromptHandler is a function that returns prompt messages given arguments.
type PromptHandler func(arguments map[string]string) (*GetPromptResult, error)

// Server represents an MCP HTTP server.
type Server struct {
	name    string
	version string
	tools   []Tool
	handlers map[string]ToolHandler
	mu      sync.RWMutex

	// Resources
	resources        []Resource
	resourceHandlers map[string]ResourceHandler

	// Prompts
	prompts        []Prompt
	promptHandlers map[string]PromptHandler

	// Rate limiting
	toolCallTimestamps []time.Time
	rateLimitMu        sync.Mutex
	rateLimitCalls     int
	rateLimitWindow    time.Duration

	// Callbacks
	onToolCall func(name string, args map[string]interface{}, duration time.Duration, success bool)
	onError    func(err error, context string)
	onRequest  func(method string)
}

// NewServer creates a new MCP server.
func NewServer(name, version string, rateLimitCalls int, rateLimitWindow time.Duration) *Server {
	if rateLimitCalls <= 0 {
		rateLimitCalls = 10
	}
	if rateLimitWindow <= 0 {
		rateLimitWindow = 20 * time.Second
	}
	return &Server{
		name:               name,
		version:            version,
		tools:              make([]Tool, 0),
		handlers:           make(map[string]ToolHandler),
		resources:          make([]Resource, 0),
		resourceHandlers:   make(map[string]ResourceHandler),
		prompts:            make([]Prompt, 0),
		promptHandlers:     make(map[string]PromptHandler),
		toolCallTimestamps: make([]time.Time, 0),
		rateLimitCalls:     rateLimitCalls,
		rateLimitWindow:    rateLimitWindow,
	}
}

func (s *Server) SetToolCallCallback(cb func(name string, args map[string]interface{}, duration time.Duration, success bool)) {
	s.onToolCall = cb
}

func (s *Server) SetErrorCallback(cb func(err error, context string)) {
	s.onError = cb
}

func (s *Server) SetRequestCallback(cb func(method string)) {
	s.onRequest = cb
}

// RegisterTool registers a tool with its handler.
func (s *Server) RegisterTool(tool Tool, handler ToolHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools = append(s.tools, tool)
	s.handlers[tool.Name] = handler
}

// RegisterResource registers a static resource.
func (s *Server) RegisterResource(resource Resource, handler ResourceHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resources = append(s.resources, resource)
	s.resourceHandlers[resource.URI] = handler
}

// RegisterPrompt registers a prompt template.
func (s *Server) RegisterPrompt(prompt Prompt, handler PromptHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, prompt)
	s.promptHandlers[prompt.Name] = handler
}

func (s *Server) checkRateLimit() bool {
	s.rateLimitMu.Lock()
	defer s.rateLimitMu.Unlock()

	now := time.Now()
	windowStart := now.Add(-s.rateLimitWindow)

	newTimestamps := make([]time.Time, 0)
	for _, ts := range s.toolCallTimestamps {
		if ts.After(windowStart) {
			newTimestamps = append(newTimestamps, ts)
		}
	}
	s.toolCallTimestamps = newTimestamps

	if len(s.toolCallTimestamps) >= s.rateLimitCalls {
		return true
	}

	s.toolCallTimestamps = append(s.toolCallTimestamps, now)
	return false
}

// BuildMux creates the HTTP mux with all routes.
func (s *Server) BuildMux() *http.ServeMux {
	mux := http.NewServeMux()

	// Health check (no auth)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"version": s.version,
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
	})

	// MCP endpoint
	mux.HandleFunc("/mcp", s.handleMCPEndpoint)

	return mux
}

func (s *Server) handleMCPEndpoint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(&JSONRPCResponse{
			JSONRPC: "2.0",
			Error:   &JSONRPCError{Code: ParseError, Message: "Parse error"},
		})
		return
	}

	response := s.handleMessage(body)
	if response != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

// RunHTTP starts the HTTP server with graceful shutdown.
func (s *Server) RunHTTP(ctx context.Context, addr string) error {
	mux := s.BuildMux()

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	log.Printf("[HTTP] Listening on %s", addr)

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutCtx)
	}()

	return server.ListenAndServe()
}

func (s *Server) handleMessage(data []byte) *JSONRPCResponse {
	var request JSONRPCRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			Error: &JSONRPCError{
				Code:    ParseError,
				Message: "Parse error",
				Data:    err.Error(),
			},
		}
	}

	// Notifications have no ID
	if request.ID == nil {
		s.handleNotification(&request)
		return nil
	}

	return s.handleRequest(&request)
}

func (s *Server) handleNotification(request *JSONRPCRequest) {
	// Handle known notifications silently
}

func (s *Server) handleRequest(request *JSONRPCRequest) *JSONRPCResponse {
	if s.onRequest != nil {
		s.onRequest(request.Method)
	}

	response := &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      request.ID,
	}

	switch request.Method {
	case "initialize":
		response.Result = s.handleInitialize()
	case "tools/list":
		response.Result = s.handleListTools()
	case "tools/call":
		result, err := s.handleCallTool(request.Params)
		if err != nil {
			response.Error = &JSONRPCError{
				Code:    InternalError,
				Message: err.Error(),
			}
		} else {
			response.Result = result
		}
	case "resources/list":
		response.Result = s.handleListResources()
	case "resources/read":
		result, err := s.handleReadResource(request.Params)
		if err != nil {
			response.Error = &JSONRPCError{
				Code:    InvalidParams,
				Message: err.Error(),
			}
		} else {
			response.Result = result
		}
	case "prompts/list":
		response.Result = s.handleListPrompts()
	case "prompts/get":
		result, err := s.handleGetPrompt(request.Params)
		if err != nil {
			response.Error = &JSONRPCError{
				Code:    InvalidParams,
				Message: err.Error(),
			}
		} else {
			response.Result = result
		}
	case "ping":
		response.Result = map[string]interface{}{}
	default:
		response.Error = &JSONRPCError{
			Code:    MethodNotFound,
			Message: fmt.Sprintf("Method not found: %s", request.Method),
		}
	}

	return response
}

func (s *Server) handleInitialize() *InitializeResult {
	caps := ServerCapabilities{
		Tools: &ToolsCapability{ListChanged: false},
	}
	s.mu.RLock()
	hasResources := len(s.resources) > 0
	hasPrompts := len(s.prompts) > 0
	s.mu.RUnlock()
	if hasResources {
		caps.Resources = &ResourcesCapability{}
	}
	if hasPrompts {
		caps.Prompts = &PromptsCapability{}
	}
	return &InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities:    caps,
		ServerInfo: ServerInfo{
			Name:    s.name,
			Version: s.version,
		},
	}
}

func (s *Server) handleListTools() *ListToolsResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &ListToolsResult{Tools: s.tools}
}

func (s *Server) handleListResources() *ListResourcesResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &ListResourcesResult{Resources: s.resources}
}

func (s *Server) handleReadResource(params interface{}) (*ReadResourceResult, error) {
	paramsMap, ok := params.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid params type")
	}
	uri, ok := paramsMap["uri"].(string)
	if !ok || uri == "" {
		return nil, fmt.Errorf("missing resource URI")
	}

	s.mu.RLock()
	handler, exists := s.resourceHandlers[uri]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unknown resource: %s", uri)
	}

	text, err := handler()
	if err != nil {
		return nil, err
	}

	return &ReadResourceResult{
		Contents: []ResourceContent{{
			URI:      uri,
			MimeType: "application/json",
			Text:     text,
		}},
	}, nil
}

func (s *Server) handleListPrompts() *ListPromptsResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &ListPromptsResult{Prompts: s.prompts}
}

func (s *Server) handleGetPrompt(params interface{}) (*GetPromptResult, error) {
	paramsMap, ok := params.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid params type")
	}
	name, ok := paramsMap["name"].(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("missing prompt name")
	}

	// Extract string arguments
	args := make(map[string]string)
	if argsRaw, ok := paramsMap["arguments"].(map[string]interface{}); ok {
		for k, v := range argsRaw {
			if s, ok := v.(string); ok {
				args[k] = s
			}
		}
	}

	s.mu.RLock()
	handler, exists := s.promptHandlers[name]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unknown prompt: %s", name)
	}

	return handler(args)
}

func (s *Server) handleCallTool(params interface{}) (*CallToolResult, error) {
	paramsMap, ok := params.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid params type")
	}

	name, ok := paramsMap["name"].(string)
	if !ok {
		return nil, fmt.Errorf("missing tool name")
	}

	arguments, _ := paramsMap["arguments"].(map[string]interface{})

	if s.checkRateLimit() {
		return &CallToolResult{
			Content: []ContentItem{{
				Type: "text",
				Text: fmt.Sprintf("Rate limit exceeded: Maximum %d tool calls per %s. Please try again later.", s.rateLimitCalls, s.rateLimitWindow),
			}},
			IsError: true,
		}, nil
	}

	s.mu.RLock()
	handler, exists := s.handlers[name]
	s.mu.RUnlock()

	if !exists {
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: fmt.Sprintf("Unknown tool: %s", name)}},
			IsError: true,
		}, nil
	}

	startTime := time.Now()
	result, err := handler(arguments)
	duration := time.Since(startTime)

	success := err == nil && (result == nil || !result.IsError)

	if s.onToolCall != nil {
		s.onToolCall(name, arguments, duration, success)
	}

	if err != nil {
		if s.onError != nil {
			s.onError(err, fmt.Sprintf("tool_%s", name))
		}
		return &CallToolResult{
			Content: []ContentItem{{Type: "text", Text: fmt.Sprintf("Error: %s", err.Error())}},
			IsError: true,
		}, nil
	}

	return result, nil
}

// Version returns the server version.
func (s *Server) Version() string {
	return s.version
}

// Name returns the server name.
func (s *Server) Name() string {
	return s.name
}

// ToolCount returns the number of registered tools.
func (s *Server) ToolCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.tools)
}

// ResourceCount returns the number of registered resources.
func (s *Server) ResourceCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.resources)
}

// PromptCount returns the number of registered prompts.
func (s *Server) PromptCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.prompts)
}

// Tools returns a copy of the registered tools.
func (s *Server) Tools() []Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Tool, len(s.tools))
	copy(result, s.tools)
	return result
}

// FormatSSEEvent formats a JSON-RPC response as an SSE event (for future streaming support).
func FormatSSEEvent(response *JSONRPCResponse) ([]byte, error) {
	data, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}
	return append([]byte("data: "), append(data, '\n', '\n')...), nil
}

// TextResult is a convenience constructor for a text CallToolResult.
func TextResult(text string) *CallToolResult {
	return &CallToolResult{
		Content: []ContentItem{{Type: "text", Text: text}},
	}
}

// ErrorResult is a convenience constructor for an error CallToolResult.
func ErrorResult(format string, args ...interface{}) *CallToolResult {
	return &CallToolResult{
		Content: []ContentItem{{Type: "text", Text: fmt.Sprintf(format, args...)}},
		IsError: true,
	}
}

// JSONResult marshals a value to JSON and returns it as a text result.
func JSONResult(v interface{}) (*CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return TextResult(string(data)), nil
}

// GetStringArg extracts a string argument with a default value.
func GetStringArg(args map[string]interface{}, key, defaultVal string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return defaultVal
}

// GetIntArg extracts an integer argument with a default value.
func GetIntArg(args map[string]interface{}, key string, defaultVal int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return defaultVal
}

// GetBoolArg extracts a boolean argument with a default value.
func GetBoolArg(args map[string]interface{}, key string, defaultVal bool) bool {
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return defaultVal
}

// GetStringSliceArg extracts a string slice argument.
func GetStringSliceArg(args map[string]interface{}, key string) []string {
	if v, ok := args[key]; ok {
		if arr, ok := v.([]interface{}); ok {
			result := make([]string, 0, len(arr))
			for _, item := range arr {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
	}
	return nil
}

// RequireStringArg extracts a required string argument, returning an error if missing.
func RequireStringArg(args map[string]interface{}, key string) (string, error) {
	s := GetStringArg(args, key, "")
	if s == "" {
		return "", fmt.Errorf("missing required argument: %s", key)
	}
	return s, nil
}

// Pointer helpers for JSON schema
func Float64Ptr(v float64) *float64 { return &v }
func IntPtr(v int) *int             { return &v }

// ParseToolCall extracts tool name and arguments from raw params.
func ParseToolCall(params interface{}) (string, map[string]interface{}, error) {
	paramsMap, ok := params.(map[string]interface{})
	if !ok {
		return "", nil, fmt.Errorf("invalid params type")
	}
	name, ok := paramsMap["name"].(string)
	if !ok || name == "" {
		return "", nil, fmt.Errorf("missing tool name")
	}
	arguments, _ := paramsMap["arguments"].(map[string]interface{})
	if arguments == nil {
		arguments = make(map[string]interface{})
	}
	return name, arguments, nil
}

// ListenAddr returns the appropriate listen address string.
func ListenAddr(port int) string {
	return fmt.Sprintf(":%d", port)
}

// IsLocalRequest checks if a request originates from localhost.
func IsLocalRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	host = strings.TrimSpace(host)
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}

package tutoring

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sheepy-yang/CampusClaw_Group/api/internal/security"
)

const mcpProtocolVersion = "2025-06-18"
const maxMCPResponseBytes = 1 << 20

var (
	ErrMCPUnavailable    = errors.New("mcp server unavailable")
	ErrMCPTimeout        = errors.New("mcp request timed out")
	ErrMCPProtocol       = errors.New("invalid mcp protocol response")
	ErrMCPToolNotAllowed = errors.New("mcp tool is not discovered or allowed")
	ErrMCPArguments      = errors.New("invalid mcp tool arguments")
	ErrMCPOutput         = errors.New("invalid mcp tool output")
	mcpToolNamePattern   = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
)

type MCPToolDescriptor struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type MCPHost interface {
	Discover(context.Context) ([]MCPToolDescriptor, error)
	Bind(context.Context, ...string) (map[string]MCPToolDescriptor, error)
	Call(context.Context, string, map[string]any) (map[string]any, error)
}

type HTTPMCPHost struct {
	endpoint  string
	token     string
	client    *http.Client
	timeout   time.Duration
	whitelist map[string]struct{}

	discoverMu sync.Mutex
	cacheMu    sync.RWMutex
	discovered bool
	tools      map[string]MCPToolDescriptor
	sequence   atomic.Uint64
}

func NewHTTPMCPHost(baseURL, token string, client *http.Client, timeout time.Duration) (*HTTPMCPHost, error) {
	endpoint, err := mcpEndpoint(baseURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(token) == "" {
		return nil, invalid("MCP token is required")
	}
	if client == nil {
		client = &http.Client{}
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &HTTPMCPHost{
		endpoint: endpoint,
		token:    token,
		client:   client,
		timeout:  timeout,
		whitelist: map[string]struct{}{
			"formula_verify":   {},
			"assignment_stats": {},
			"student_mistakes": {},
		},
		tools: make(map[string]MCPToolDescriptor),
	}, nil
}

func NewHTTPMCPHostFromEnv(client *http.Client, timeout time.Duration) (*HTTPMCPHost, error) {
	return NewHTTPMCPHost(os.Getenv("MCP_URL"), os.Getenv("MCP_TOKEN"), client, timeout)
}

func (h *HTTPMCPHost) Discover(ctx context.Context) ([]MCPToolDescriptor, error) {
	h.cacheMu.RLock()
	if h.discovered {
		result := sortedToolDescriptors(h.tools)
		h.cacheMu.RUnlock()
		return result, nil
	}
	h.cacheMu.RUnlock()

	h.discoverMu.Lock()
	defer h.discoverMu.Unlock()
	h.cacheMu.RLock()
	if h.discovered {
		result := sortedToolDescriptors(h.tools)
		h.cacheMu.RUnlock()
		return result, nil
	}
	h.cacheMu.RUnlock()

	var initialized struct {
		ProtocolVersion string `json:"protocolVersion"`
		Capabilities    struct {
			Tools map[string]any `json:"tools"`
		} `json:"capabilities"`
		ServerInfo map[string]any `json:"serverInfo"`
	}
	if err := h.rpc(ctx, "initialize", map[string]any{
		"protocolVersion": mcpProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]string{
			"name": "campusclaw-api", "version": "1.0.0",
		},
	}, &initialized); err != nil {
		return nil, err
	}
	if strings.TrimSpace(initialized.ProtocolVersion) == "" || initialized.Capabilities.Tools == nil {
		return nil, fmt.Errorf("%w: initialize result is incomplete", ErrMCPProtocol)
	}

	var listed struct {
		Tools []MCPToolDescriptor `json:"tools"`
	}
	if err := h.rpc(ctx, "tools/list", map[string]any{}, &listed); err != nil {
		return nil, err
	}
	if len(listed.Tools) == 0 || len(listed.Tools) > 100 {
		return nil, fmt.Errorf("%w: invalid tool count", ErrMCPProtocol)
	}
	cache := make(map[string]MCPToolDescriptor, len(listed.Tools))
	for _, tool := range listed.Tools {
		if err := validateToolDescriptor(tool); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrMCPProtocol, err)
		}
		if _, whitelisted := h.whitelist[tool.Name]; whitelisted {
			if err := validateWhitelistedToolContract(tool); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrMCPProtocol, err)
			}
		}
		if _, duplicate := cache[tool.Name]; duplicate {
			return nil, fmt.Errorf("%w: duplicate tool %q", ErrMCPProtocol, tool.Name)
		}
		tool.InputSchema = cloneJSONMap(tool.InputSchema)
		cache[tool.Name] = tool
	}
	h.cacheMu.Lock()
	h.tools = cache
	h.discovered = true
	h.cacheMu.Unlock()
	return sortedToolDescriptors(cache), nil
}

func (h *HTTPMCPHost) Bind(ctx context.Context, names ...string) (map[string]MCPToolDescriptor, error) {
	if _, err := h.Discover(ctx); err != nil {
		return nil, err
	}
	bound := make(map[string]MCPToolDescriptor, len(names))
	h.cacheMu.RLock()
	defer h.cacheMu.RUnlock()
	for _, name := range names {
		if !mcpToolNamePattern.MatchString(name) {
			return nil, ErrMCPToolNotAllowed
		}
		if _, allowed := h.whitelist[name]; !allowed {
			return nil, fmt.Errorf("%w: %s", ErrMCPToolNotAllowed, name)
		}
		tool, discovered := h.tools[name]
		if !discovered {
			return nil, fmt.Errorf("%w: %s", ErrMCPToolNotAllowed, name)
		}
		tool.InputSchema = cloneJSONMap(tool.InputSchema)
		bound[name] = tool
	}
	return bound, nil
}

func (h *HTTPMCPHost) Call(ctx context.Context, name string, arguments map[string]any) (map[string]any, error) {
	bound, err := h.Bind(ctx, name)
	if err != nil {
		return nil, err
	}
	tool := bound[name]
	if err := validateJSONSchema(tool.InputSchema, arguments); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMCPArguments, err)
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StructuredContent map[string]any `json:"structuredContent"`
		IsError           bool           `json:"isError"`
	}
	if err := h.rpc(ctx, "tools/call", map[string]any{
		"name": name, "arguments": arguments,
	}, &result); err != nil {
		return nil, err
	}
	if result.IsError || result.StructuredContent == nil {
		return nil, fmt.Errorf("%w: missing structured result", ErrMCPOutput)
	}
	if err := validateMCPToolOutput(ctx, name, arguments, result.StructuredContent); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMCPOutput, err)
	}
	return cloneJSONMap(result.StructuredContent), nil
}

type mcpRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code int `json:"code"`
		Data struct {
			Code string `json:"code"`
		} `json:"data"`
	} `json:"error"`
}

func (h *HTTPMCPHost) rpc(ctx context.Context, method string, params map[string]any, destination any) error {
	requestID := h.sequence.Add(1)
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": requestID, "method": method, "params": params,
	})
	if err != nil {
		return fmt.Errorf("%w: encode request", ErrMCPProtocol)
	}
	requestContext, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodPost, h.endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("%w: build request", ErrMCPUnavailable)
	}
	request.Header.Set("Authorization", "Bearer "+h.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := h.client.Do(request)
	if err != nil {
		var networkError net.Error
		if errors.Is(requestContext.Err(), context.DeadlineExceeded) ||
			errors.Is(err, context.DeadlineExceeded) ||
			(errors.As(err, &networkError) && networkError.Timeout()) {
			return fmt.Errorf("%w: %v", ErrMCPTimeout, err)
		}
		return fmt.Errorf("%w: %v", ErrMCPUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("%w: HTTP status %d", ErrMCPUnavailable, response.StatusCode)
	}
	responsePayload, err := io.ReadAll(io.LimitReader(response.Body, maxMCPResponseBytes+1))
	if err != nil {
		return fmt.Errorf("%w: read response", ErrMCPProtocol)
	}
	if len(responsePayload) > maxMCPResponseBytes {
		return fmt.Errorf("%w: response is too large", ErrMCPProtocol)
	}
	decoder := json.NewDecoder(bytes.NewReader(responsePayload))
	var rpcResponse mcpRPCResponse
	if err := decoder.Decode(&rpcResponse); err != nil {
		return fmt.Errorf("%w: decode response", ErrMCPProtocol)
	}
	if err := ensureDecoderEOF(decoder); err != nil {
		return fmt.Errorf("%w: trailing response data", ErrMCPProtocol)
	}
	if rpcResponse.JSONRPC != "2.0" || !sameJSONID(rpcResponse.ID, requestID) {
		return fmt.Errorf("%w: response envelope mismatch", ErrMCPProtocol)
	}
	if rpcResponse.Error != nil {
		switch rpcResponse.Error.Data.Code {
		case "TOOL_TIMEOUT":
			return fmt.Errorf("%w: remote tool timeout", ErrMCPTimeout)
		case "INVALID_PARAMS", "INVALID_TOOL_ARGUMENTS":
			return fmt.Errorf("%w: remote rejected arguments", ErrMCPArguments)
		case "TOOL_NOT_FOUND":
			return fmt.Errorf("%w: remote tool not found", ErrMCPToolNotAllowed)
		case "TOOL_RESULT_INVALID":
			return fmt.Errorf("%w: remote tool result invalid", ErrMCPOutput)
		default:
			return fmt.Errorf("%w: remote code %d (%s)", ErrMCPProtocol, rpcResponse.Error.Code, rpcResponse.Error.Data.Code)
		}
	}
	if len(rpcResponse.Result) == 0 || bytes.Equal(bytes.TrimSpace(rpcResponse.Result), []byte("null")) {
		return fmt.Errorf("%w: result is missing", ErrMCPProtocol)
	}
	if err := json.Unmarshal(rpcResponse.Result, destination); err != nil {
		return fmt.Errorf("%w: decode result", ErrMCPProtocol)
	}
	return nil
}

func mcpEndpoint(baseURL string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return "", invalid("MCP_URL must be an absolute http or https URL")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(parsed.Path, "/mcp") {
		parsed.Path += "/mcp"
	}
	return parsed.String(), nil
}

func validateToolDescriptor(tool MCPToolDescriptor) error {
	if !mcpToolNamePattern.MatchString(tool.Name) {
		return errors.New("tool has an invalid name")
	}
	if runeLength(strings.TrimSpace(tool.Description)) < 1 || runeLength(tool.Description) > 500 {
		return errors.New("tool has an invalid description")
	}
	if tool.InputSchema == nil || tool.InputSchema["type"] != "object" {
		return errors.New("tool input schema must be an object")
	}
	if _, ok := tool.InputSchema["properties"].(map[string]any); !ok {
		return errors.New("tool input schema has invalid properties")
	}
	if err := validateSchemaDefinition(tool.InputSchema); err != nil {
		return err
	}
	return nil
}

func validateWhitelistedToolContract(tool MCPToolDescriptor) error {
	var allowed, required []string
	switch tool.Name {
	case "formula_verify":
		allowed = []string{"expression", "expected"}
		required = allowed
	case "assignment_stats":
		allowed = []string{"assignment_id", "scores", "passing_score"}
		required = []string{"assignment_id", "scores"}
	case "student_mistakes":
		allowed = []string{"student_id", "mistakes"}
		required = allowed
	default:
		return nil
	}
	properties, _ := tool.InputSchema["properties"].(map[string]any)
	if len(properties) != len(allowed) {
		return fmt.Errorf("%s schema properties do not match the API binding", tool.Name)
	}
	for _, name := range allowed {
		if _, ok := properties[name]; !ok {
			return fmt.Errorf("%s schema is missing %s", tool.Name, name)
		}
	}
	requiredSet := make(map[string]bool)
	for _, name := range schemaStringList(tool.InputSchema["required"]) {
		requiredSet[name] = true
	}
	if len(requiredSet) != len(required) {
		return fmt.Errorf("%s schema required fields do not match the API binding", tool.Name)
	}
	for _, name := range required {
		if !requiredSet[name] {
			return fmt.Errorf("%s schema does not require %s", tool.Name, name)
		}
	}
	return nil
}

func validateSchemaDefinition(schema map[string]any) error {
	schemaType, _ := schema["type"].(string)
	switch schemaType {
	case "object":
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			return errors.New("object schema has invalid properties")
		}
		if additional, ok := schema["additionalProperties"].(bool); !ok || additional {
			return errors.New("object schema must reject additional properties")
		}
		for _, property := range properties {
			nested, ok := property.(map[string]any)
			if !ok {
				return errors.New("property schema is invalid")
			}
			if err := validateSchemaDefinition(nested); err != nil {
				return err
			}
		}
	case "array":
		items, ok := schema["items"].(map[string]any)
		if !ok {
			return errors.New("array schema has invalid items")
		}
		return validateSchemaDefinition(items)
	case "string", "number":
		return nil
	default:
		return fmt.Errorf("unsupported schema type %q", schemaType)
	}
	return nil
}

func ensureDecoderEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("multiple JSON values")
	}
	return err
}

func sameJSONID(raw json.RawMessage, expected uint64) bool {
	var actual uint64
	return json.Unmarshal(raw, &actual) == nil && actual == expected
}

func sortedToolDescriptors(tools map[string]MCPToolDescriptor) []MCPToolDescriptor {
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sortStrings(names)
	result := make([]MCPToolDescriptor, 0, len(names))
	for _, name := range names {
		tool := tools[name]
		tool.InputSchema = cloneJSONMap(tool.InputSchema)
		result = append(result, tool)
	}
	return result
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func cloneJSONMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = cloneJSONValue(value)
	}
	return result
}

func cloneJSONValue(value any) any {
	switch item := value.(type) {
	case map[string]any:
		return cloneJSONMap(item)
	case []any:
		result := make([]any, len(item))
		for index := range item {
			result[index] = cloneJSONValue(item[index])
		}
		return result
	case []string:
		return append([]string(nil), item...)
	default:
		return item
	}
}

func validateMCPToolOutput(ctx context.Context, name string, arguments, output map[string]any) error {
	allowed := map[string]map[string]bool{
		"formula_verify": {
			"verified": true, "status": true, "method": true,
		},
		"assignment_stats": {
			"assignmentId": true, "count": true, "average": true, "median": true,
			"minimum": true, "maximum": true, "passingScore": true, "passRate": true,
		},
		"student_mistakes": {
			"studentId": true, "total": true, "byKnowledgePoint": true, "mistakes": true,
		},
	}[name]
	if allowed == nil {
		return errors.New("unknown output contract")
	}
	for key := range output {
		if !allowed[key] {
			return fmt.Errorf("unexpected output field %q", key)
		}
	}
	switch name {
	case "formula_verify":
		verified, ok := output["verified"].(bool)
		if !ok {
			return errors.New("verified must be boolean")
		}
		status, ok := output["status"].(string)
		if !ok || (status != "verified" && status != "not_equivalent" && status != "indeterminate") {
			return errors.New("invalid formula status")
		}
		if verified != (status == "verified") {
			return errors.New("formula status contradicts verified")
		}
		method, ok := output["method"].(string)
		if !ok || runeLength(method) < 1 || runeLength(method) > 64 {
			return errors.New("invalid formula method")
		}
		if err := security.CheckInput(ctx, method); err != nil {
			return err
		}
	case "assignment_stats":
		if output["assignmentId"] != arguments["assignment_id"] {
			return errors.New("assignment scope mismatch")
		}
		numbers := make(map[string]float64)
		for _, field := range []string{"count", "average", "median", "minimum", "maximum", "passingScore", "passRate"} {
			number, ok := JSONNumber(output[field])
			if !ok {
				return fmt.Errorf("%s must be numeric", field)
			}
			numbers[field] = number
		}
		rawScores, _ := arguments["scores"].([]any)
		if numbers["count"] != float64(len(rawScores)) || numbers["count"] < 1 || numbers["count"] > 500 {
			return errors.New("assignment count does not match scoped input")
		}
		for _, field := range []string{"average", "median", "minimum", "maximum", "passingScore"} {
			if numbers[field] < 0 || numbers[field] > 100 {
				return fmt.Errorf("%s is outside 0 to 100", field)
			}
		}
		if numbers["passRate"] < 0 || numbers["passRate"] > 1 ||
			numbers["minimum"] > numbers["median"] || numbers["median"] > numbers["maximum"] {
			return errors.New("assignment statistics are inconsistent")
		}
		scores := make([]float64, len(rawScores))
		total, passingScore, passed := 0.0, 60.0, 0
		if configured, ok := JSONNumber(arguments["passing_score"]); ok {
			passingScore = configured
		}
		for index, raw := range rawScores {
			score, ok := JSONNumber(raw)
			if !ok {
				return errors.New("scoped score is invalid")
			}
			scores[index] = score
			total += score
			if score >= passingScore {
				passed++
			}
		}
		sort.Float64s(scores)
		median := scores[len(scores)/2]
		if len(scores)%2 == 0 {
			median = (scores[len(scores)/2-1] + scores[len(scores)/2]) / 2
		}
		expected := map[string]float64{
			"average":      total / float64(len(scores)),
			"median":       median,
			"minimum":      scores[0],
			"maximum":      scores[len(scores)-1],
			"passingScore": passingScore,
			"passRate":     float64(passed) / float64(len(scores)),
		}
		for field, value := range expected {
			if math.Abs(numbers[field]-value) > 0.00011 {
				return fmt.Errorf("%s does not match scoped input", field)
			}
		}
	case "student_mistakes":
		if output["studentId"] != arguments["student_id"] {
			return errors.New("student scope mismatch")
		}
		total, ok := JSONNumber(output["total"])
		if !ok || total < 0 || total > 100 {
			return errors.New("invalid mistake total")
		}
		inputMistakes, _ := arguments["mistakes"].([]any)
		outputMistakes, ok := output["mistakes"].([]any)
		if !ok || total != float64(len(inputMistakes)) || len(outputMistakes) != len(inputMistakes) {
			return errors.New("mistake output count does not match scoped input")
		}
		allowedKnowledgePoints := make(map[string]bool)
		for _, raw := range inputMistakes {
			mistake, ok := raw.(map[string]any)
			if !ok {
				return errors.New("scoped mistake input is invalid")
			}
			knowledgePoint, _ := mistake["knowledge_point"].(string)
			allowedKnowledgePoints[knowledgePoint] = true
		}
		if err := validateMistakeOutput(output["byKnowledgePoint"], outputMistakes, inputMistakes, allowedKnowledgePoints); err != nil {
			return err
		}
		if err := validateSafeJSONStrings(ctx, output, 300); err != nil {
			return err
		}
	}
	return nil
}

func validateMistakeOutput(rawCounts any, outputMistakes, inputMistakes []any, allowedKnowledgePoints map[string]bool) error {
	counts, ok := rawCounts.([]any)
	if !ok {
		return errors.New("byKnowledgePoint must be an array")
	}
	seenKnowledgePoints := make(map[string]bool)
	total := 0
	for _, raw := range counts {
		item, ok := raw.(map[string]any)
		if !ok || !hasOnlyJSONKeys(item, "knowledgePoint", "count") {
			return errors.New("invalid knowledge-point aggregate")
		}
		knowledgePoint, ok := item["knowledgePoint"].(string)
		count, countOK := JSONNumber(item["count"])
		if !ok || !countOK || count < 1 || count != float64(int(count)) ||
			!allowedKnowledgePoints[knowledgePoint] || seenKnowledgePoints[knowledgePoint] {
			return errors.New("invalid knowledge-point aggregate values")
		}
		seenKnowledgePoints[knowledgePoint] = true
		total += int(count)
	}
	if total != len(inputMistakes) {
		return errors.New("knowledge-point aggregates do not cover scoped input")
	}
	for index, raw := range outputMistakes {
		item, ok := raw.(map[string]any)
		if !ok || !hasOnlyJSONKeys(item, "index", "knowledgePoint", "errorCause", "questionSummary") {
			return errors.New("invalid mistake summary")
		}
		itemIndex, ok := JSONNumber(item["index"])
		if !ok || int(itemIndex) != index+1 || itemIndex != float64(index+1) {
			return errors.New("mistake summary index is invalid")
		}
		input, ok := inputMistakes[index].(map[string]any)
		if !ok || item["knowledgePoint"] != input["knowledge_point"] {
			return errors.New("mistake summary changed its knowledge point")
		}
		if value, exists := item["errorCause"]; exists && value != input["error_cause"] {
			return errors.New("mistake summary changed its error cause")
		}
		if value, exists := item["questionSummary"]; exists && value != input["question_summary"] {
			return errors.New("mistake summary changed its question text")
		}
	}
	return nil
}

func hasOnlyJSONKeys(value map[string]any, allowed ...string) bool {
	set := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		set[key] = true
	}
	for key := range value {
		if !set[key] {
			return false
		}
	}
	return true
}

func validateSafeJSONStrings(ctx context.Context, value any, maxLength int) error {
	switch item := value.(type) {
	case string:
		if runeLength(item) > maxLength {
			return errors.New("output text is too long")
		}
		return security.CheckInput(ctx, item)
	case map[string]any:
		for _, nested := range item {
			if err := validateSafeJSONStrings(ctx, nested, maxLength); err != nil {
				return err
			}
		}
	case []any:
		for _, nested := range item {
			if err := validateSafeJSONStrings(ctx, nested, maxLength); err != nil {
				return err
			}
		}
	}
	return nil
}

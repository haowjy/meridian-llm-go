package openrouter

import (
	"encoding/json"
	"fmt"

	"github.com/haowjy/meridian-llm-go"
)

// ChatCompletionRequest represents an OpenRouter chat completion request.
// OpenRouter uses OpenAI-compatible format.
type ChatCompletionRequest struct {
	Model       string      `json:"model"`
	Messages    []Message   `json:"messages"`
	MaxTokens   *int        `json:"max_tokens,omitempty"`
	Temperature *float64    `json:"temperature,omitempty"`
	TopP        *float64    `json:"top_p,omitempty"`
	TopK        *int        `json:"top_k,omitempty"`
	Stop        []string    `json:"stop,omitempty"`
	Stream      bool        `json:"stream"`
	Tools       []Tool      `json:"tools,omitempty"`
	ToolChoice  interface{} `json:"tool_choice,omitempty"` // "auto", "none", "required", or {"type": "function", "function": {"name": "..."}}
}

// Message represents a message in the conversation.
type Message struct {
	Role             string            `json:"role"` // "system", "user", "assistant", "tool"
	Content          interface{}       `json:"content,omitempty"` // string or []ContentPart
	Name             *string           `json:"name,omitempty"`
	ToolCalls        []ToolCall        `json:"tool_calls,omitempty"`
	ToolCallID       *string           `json:"tool_call_id,omitempty"`      // For role:"tool" messages
	Reasoning        *string           `json:"reasoning,omitempty"`         // Simple reasoning field (often just placeholder)
	ReasoningDetails []ReasoningDetail `json:"reasoning_details,omitempty"` // Actual thinking content from models like kimi-k2-thinking
	Annotations      []Annotation      `json:"annotations,omitempty"`       // Web search citations for :online models
}

// ContentPart represents a part of multimodal content.
type ContentPart struct {
	Type     string    `json:"type"` // "text", "image_url"
	Text     *string   `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL represents an image URL in content.
type ImageURL struct {
	URL    string  `json:"url"`
	Detail *string `json:"detail,omitempty"` // "auto", "low", "high"
}

// ToolCall represents a function call in assistant messages.
type ToolCall struct {
	Index    *int         `json:"index,omitempty"` // Streaming only - index of this tool call in the array
	ID       string       `json:"id"`
	Type     string       `json:"type"` // "function"
	Function FunctionCall `json:"function"`
}

// FunctionCall represents the function details of a tool call.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// Annotation represents a citation or reference in the response.
// Used by OpenRouter :online models to provide web search results.
type Annotation struct {
	Type        string       `json:"type"` // "url_citation"
	URLCitation *URLCitation `json:"url_citation,omitempty"`
}

// URLCitation represents a web search result citation.
// Returned by OpenRouter :online models (e.g., moonshotai/kimi-k2-thinking:online).
type URLCitation struct {
	URL        string  `json:"url"`
	StartIndex int     `json:"start_index"` // Position in content where citation starts
	EndIndex   int     `json:"end_index"`   // Position in content where citation ends
	Title      *string `json:"title,omitempty"`
	Content    *string `json:"content,omitempty"` // Snippet/excerpt from the page
}

// ReasoningDetail represents a reasoning/thinking detail in the response.
// Used by reasoning-enabled models like moonshotai/kimi-k2-thinking to provide extended thinking.
// The reasoning_details array contains structured reasoning information that can be of different types.
type ReasoningDetail struct {
	Type    string  `json:"type"`              // "reasoning.text", "reasoning.summary", "reasoning.encrypted"
	Text    *string `json:"text,omitempty"`    // Actual thinking content (for type: "reasoning.text")
	Summary *string `json:"summary,omitempty"` // Summary of reasoning (for type: "reasoning.summary")
	Data    *string `json:"data,omitempty"`    // Encrypted data (for type: "reasoning.encrypted")
}

// Tool represents a function tool definition.
type Tool struct {
	Type     string             `json:"type"` // "function"
	Function FunctionDefinition `json:"function"`
}

// FunctionDefinition represents a function tool definition.
type FunctionDefinition struct {
	Name        string                 `json:"name"`
	Description *string                `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters"` // JSON Schema
}

// ChatCompletionResponse represents an OpenRouter chat completion response (non-streaming).
type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"` // "chat.completion"
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice represents a completion choice in the response.
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason *string `json:"finish_reason"` // "stop", "length", "tool_calls", "content_filter"
}

// Usage represents token usage in the response.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// buildChatCompletionRequest constructs an OpenRouter API request from a GenerateRequest.
// This function is shared between GenerateResponse and StreamResponse to avoid duplication.
func buildChatCompletionRequest(req *llmprovider.GenerateRequest) (*ChatCompletionRequest, error) {
	// Convert library messages to OpenRouter format
	messages, err := convertToOpenRouterMessages(req.Messages)
	if err != nil {
		return nil, fmt.Errorf("failed to convert messages: %w", err)
	}

	// Extract params or use defaults
	params := req.Params
	if params == nil {
		params = &llmprovider.RequestParams{}
	}

	// Build request with defaults
	openrouterReq := &ChatCompletionRequest{
		Model:    req.Model,
		Messages: messages,
		Stream:   false,
	}

	// MaxTokens
	if params.MaxTokens != nil {
		openrouterReq.MaxTokens = params.MaxTokens
	}

	// Temperature
	if params.Temperature != nil {
		openrouterReq.Temperature = params.Temperature
	}

	// Top-P
	if params.TopP != nil {
		openrouterReq.TopP = params.TopP
	}

	// Top-K
	if params.TopK != nil {
		openrouterReq.TopK = params.TopK
	}

	// Stop sequences
	if len(params.Stop) > 0 {
		openrouterReq.Stop = params.Stop
	}

	// Tools
	if len(params.Tools) > 0 {
		openrouterTools, err := convertToOpenRouterTools(params.Tools)
		if err != nil {
			return nil, fmt.Errorf("failed to convert tools: %w", err)
		}
		openrouterReq.Tools = openrouterTools
	}

	// Tool choice
	if params.ToolChoice != nil {
		toolChoice, err := convertToolChoice(params.ToolChoice)
		if err != nil {
			return nil, fmt.Errorf("failed to convert tool choice: %w", err)
		}
		openrouterReq.ToolChoice = toolChoice
	}

	return openrouterReq, nil
}

// BuildChatCompletionRequestDebug builds the OpenRouter request payload for debugging.
// It converts the library GenerateRequest to OpenRouter's ChatCompletionRequest format
// and returns it as a map[string]interface{} for inspection.
//
// This is used by debug endpoints to show the exact JSON that would be sent to OpenRouter's API.
func BuildChatCompletionRequestDebug(req *llmprovider.GenerateRequest) (map[string]interface{}, error) {
	// Build the ChatCompletionRequest using the same logic as GenerateResponse
	chatReq, err := buildChatCompletionRequest(req)
	if err != nil {
		return nil, err
	}

	// Marshal to JSON using OpenRouter's struct tags (which use "parameters" not "input_schema")
	jsonBytes, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal openrouter request: %w", err)
	}

	// Unmarshal back into a map for flexible inspection
	var result map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal openrouter request: %w", err)
	}

	return result, nil
}

// convertToolChoice converts library tool choice to OpenRouter format.
func convertToolChoice(choice interface{}) (interface{}, error) {
	// Check for nil first
	if choice == nil {
		return "auto", nil
	}

	// Handle ToolChoice type
	if tc, ok := choice.(*llmprovider.ToolChoice); ok {
		// Check if tc itself is nil (typed nil pointer)
		if tc == nil {
			return "auto", nil
		}

		switch tc.Mode {
		case llmprovider.ToolChoiceModeAuto:
			return "auto", nil
		case llmprovider.ToolChoiceModeRequired:
			return "required", nil
		case llmprovider.ToolChoiceModeNone:
			return "none", nil
		case llmprovider.ToolChoiceModeSpecific:
			if tc.ToolName == nil {
				return nil, fmt.Errorf("specific tool choice requires tool_name")
			}
			return map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name": *tc.ToolName,
				},
			}, nil
		default:
			return "auto", nil
		}
	}

	// Fallback to auto
	return "auto", nil
}

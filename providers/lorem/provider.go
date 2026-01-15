package lorem

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	loremgen "github.com/bozaro/golorem"

	llmprovider "github.com/haowjy/meridian-llm-go"
)

// Provider is a mock LLM provider that generates lorem ipsum text.
// Used for testing and development without requiring real API keys.
type Provider struct {
	generator       *loremgen.Lorem
	logger          *slog.Logger
	debugStreamLogs bool
}

type Option func(*Provider)

func WithLogger(logger *slog.Logger) Option {
	return func(p *Provider) {
		if logger != nil {
			p.logger = logger
		}
	}
}

func WithDebugStreamLogs(enabled bool) Option {
	return func(p *Provider) {
		p.debugStreamLogs = enabled
	}
}

// NewProvider creates a new lorem ipsum provider.
func NewProvider(opts ...Option) *Provider {
	p := &Provider{
		generator: loremgen.New(),
		logger:    slog.Default(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}
	return p
}

// Name returns the provider identifier.
func (p *Provider) Name() llmprovider.ProviderID {
	return llmprovider.ProviderLorem
}

// SupportsModel returns true if the model name starts with "lorem-".
// Example models: "lorem-fast", "lorem-slow", "lorem-test"
func (p *Provider) SupportsModel(model string) bool {
	return strings.HasPrefix(model, "lorem-")
}

// GenerateResponse generates a complete lorem ipsum response with a 10-second delay.
// This simulates a blocking API call to a real LLM provider.
func (p *Provider) GenerateResponse(ctx context.Context, req *llmprovider.GenerateRequest) (*llmprovider.GenerateResponse, error) {
	// Validate model
	if !p.SupportsModel(req.Model) {
		return nil, &llmprovider.ModelError{
			Model:    req.Model,
			Provider: p.Name().String(),
			Reason:   "model not supported by Lorem provider (must start with 'lorem-')",
			Err:      llmprovider.ErrInvalidModel,
		}
	}

	// Extract parameters
	params := req.Params
	if params == nil {
		params = &llmprovider.RequestParams{}
	}
	maxTokens := params.GetMaxTokens(4096)

	// Simulate 10-second processing delay
	select {
	case <-time.After(10 * time.Second):
		// Continue after delay
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Generate lorem ipsum text
	// Estimate: 1 token ≈ 4 characters
	targetChars := maxTokens * 4
	text := p.generateText(targetChars)

	// Estimate token counts (rough approximation)
	inputTokens := p.estimateTokens(req.Messages)
	outputTokens := len(strings.Fields(text)) // Word count as proxy

	// Determine stop reason based on model type
	stopReason := "end_turn"
	if isCutoffModel(req.Model) {
		stopReason = "max_tokens"
	}

	// Create response
	return &llmprovider.GenerateResponse{
		Blocks: []*llmprovider.Block{
			{
				BlockType:   llmprovider.BlockTypeText,
				TextContent: &text,
			},
		},
		Model:        req.Model,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		StopReason:   stopReason,
		ResponseMetadata: map[string]interface{}{
			"mock":     true,
			"provider": "lorem",
		},
	}, nil
}

// getStreamDelay returns the delay between words based on the model name.
// - lorem-slow: 2 words/second (500ms per word)
// - lorem-fast: 30 words/second (33ms per word)
// - lorem-medium: 10 words/second (100ms per word)
// - default: 10 words/second
func getStreamDelay(model string) time.Duration {
	if strings.Contains(model, "slow") {
		return 500 * time.Millisecond // 2 words/second
	}
	if strings.Contains(model, "fast") {
		return 33 * time.Millisecond // 30 words/second
	}
	if strings.Contains(model, "medium") {
		return 100 * time.Millisecond // 10 words/second
	}
	return 100 * time.Millisecond // default: 10 words/second
}

// isCutoffModel returns true if the model should simulate max_tokens cutoff.
func isCutoffModel(model string) bool {
	return strings.Contains(model, "cutoff") || strings.Contains(model, "small")
}

// toolTemplate defines a mock tool call template
type toolTemplate struct {
	name  string
	input map[string]interface{}
}

// getToolTemplates returns the rotating tool templates used in multi-block streaming
func getToolTemplates() []toolTemplate {
	return []toolTemplate{
		{
			name: "search_files",
			input: map[string]interface{}{
				"query":       "lorem ipsum",
				"max_results": 10,
				"file_types":  []string{"txt", "md"},
			},
		},
		{
			name: "analyze_character",
			input: map[string]interface{}{
				"name":   "dolor",
				"traits": []string{"amet", "consectetur", "adipiscing"},
				"depth":  "detailed",
			},
		},
		{
			name: "get_outline",
			input: map[string]interface{}{
				"document_id":      "doc-lorem-123",
				"include_chapters": true,
				"max_depth":        3,
			},
		},
	}
}

// StreamResponse generates a streaming lorem ipsum response with rotating block types.
// Speed varies based on model name (lorem-slow, lorem-fast, lorem-medium).
// Rotates through: text (20 words) → thinking (20 words, if enabled) → tool_use → repeat
func (p *Provider) StreamResponse(ctx context.Context, req *llmprovider.GenerateRequest) (<-chan llmprovider.StreamEvent, error) {
	// Validate model
	if !p.SupportsModel(req.Model) {
		return nil, &llmprovider.ModelError{
			Model:    req.Model,
			Provider: p.Name().String(),
			Reason:   "model not supported by Lorem provider (must start with 'lorem-')",
			Err:      llmprovider.ErrInvalidModel,
		}
	}

	// Extract parameters
	params := req.Params
	if params == nil {
		params = &llmprovider.RequestParams{}
	}
	maxTokens := params.GetMaxTokens(4096)
	thinkingEnabled := params.ThinkingEnabled != nil && *params.ThinkingEnabled
	toolsEnabled := len(params.Tools) > 0

	// Create buffered channel
	eventChan := make(chan llmprovider.StreamEvent, 10)

	// Start streaming goroutine
	go func() {
		defer close(eventChan)

		blockIndex := 0
		totalOutputTokens := 0
		stopReason := "end_turn"
		toolIndex := 0 // Rotate through requested tools

		if p.debugStreamLogs {
			p.logger.Debug("lorem stream started",
				"model", req.Model,
				"thinking_enabled", thinkingEnabled,
				"tools_enabled", toolsEnabled,
				"max_tokens", maxTokens,
			)
		}

		// Rotation pattern: text → [thinking] → [tool_use if enabled] → repeat
		// Each text/thinking block: 20 words
		// Tool blocks: ~20 tokens for JSON
		for totalOutputTokens < maxTokens {
			if p.debugStreamLogs {
				p.logger.Debug("lorem loop iteration",
					"block_index", blockIndex,
					"total_output_tokens", totalOutputTokens,
					"remaining_tokens", maxTokens-totalOutputTokens,
				)
			}
			remainingTokens := maxTokens - totalOutputTokens

			// Block 0, 3, 6, 9... : Text block (20 words)
			if blockIndex%3 == 0 || (blockIndex%3 == 1 && !thinkingEnabled) {
				if p.debugStreamLogs {
					p.logger.Debug("lorem executing text block", "block_index", blockIndex)
				}
				targetWords := 20
				if remainingTokens < targetWords {
					targetWords = remainingTokens
				}

				outputTokens, cutoff, err := p.streamTextBlock(ctx, eventChan, blockIndex, targetWords, req.Model)
				if err != nil {
					eventChan <- llmprovider.StreamEvent{Error: err}
					return
				}
				totalOutputTokens += outputTokens
				blockIndex++
				if p.debugStreamLogs {
					p.logger.Debug("lorem text block complete",
						"output_tokens", outputTokens,
						"new_total", totalOutputTokens,
						"cutoff", cutoff,
					)
				}

				if cutoff {
					stopReason = "max_tokens"
					break
				}
			} else if blockIndex%3 == 1 && thinkingEnabled {
				// Block 1, 4, 7... : Thinking block (20 words, only if enabled)
				if p.debugStreamLogs {
					p.logger.Debug("lorem executing thinking block", "block_index", blockIndex)
				}
				targetWords := 20
				if remainingTokens < targetWords {
					targetWords = remainingTokens
				}

				outputTokens, cutoff, err := p.streamThinkingBlock(ctx, eventChan, blockIndex, targetWords, req.Model)
				if err != nil {
					eventChan <- llmprovider.StreamEvent{Error: err}
					return
				}
				totalOutputTokens += outputTokens
				blockIndex++
				if p.debugStreamLogs {
					p.logger.Debug("lorem thinking block complete",
						"output_tokens", outputTokens,
						"new_total", totalOutputTokens,
						"cutoff", cutoff,
					)
				}

				if cutoff {
					stopReason = "max_tokens"
					break
				}
			} else if toolsEnabled {
				// Block 2, 5, 8... : Tool use block (~20 tokens for JSON)
				if p.debugStreamLogs {
					p.logger.Debug("lorem executing tool_use block", "block_index", blockIndex, "tool_index", toolIndex)
				}
				if remainingTokens < 20 {
					if p.debugStreamLogs {
						p.logger.Debug("lorem skipping tool_use: insufficient tokens", "remaining_tokens", remainingTokens)
					}
					// Not enough budget for tool block
					break
				}

				// Use requested tool (rotate through Tools)
				builtInTool := params.Tools[toolIndex%len(params.Tools)]
				outputTokens, err := p.streamToolUseBlockFromBuiltIn(ctx, eventChan, blockIndex, &builtInTool, req.Model)
				if err != nil {
					eventChan <- llmprovider.StreamEvent{Error: err}
					return
				}
				totalOutputTokens += outputTokens
				blockIndex++
				toolIndex++
				if p.debugStreamLogs {
					p.logger.Debug("lorem tool_use block complete",
						"output_tokens", outputTokens,
						"new_total", totalOutputTokens,
					)
				}
			} else {
				// No tools enabled, skip tool block
				blockIndex++
			}

			// Safety check: prevent infinite loop
			if blockIndex > 100 {
				if p.debugStreamLogs {
					p.logger.Debug("lorem loop exit: safety check (block_index > 100)", "block_index", blockIndex)
				}
				break
			}
		}

		if p.debugStreamLogs {
			p.logger.Debug("lorem loop exited",
				"total_output_tokens", totalOutputTokens,
				"max_tokens", maxTokens,
				"block_index", blockIndex,
			)
		}

		// Note: stopReason is already set correctly:
		// - "max_tokens" is set inside the loop when cutoff=true (cutoff models only)
		// - "end_turn" remains for normal completion (non-cutoff models)

		// Send final metadata
		inputTokens := p.estimateTokens(req.Messages)
		eventChan <- llmprovider.StreamEvent{
			Metadata: &llmprovider.StreamMetadata{
				Model:        req.Model,
				InputTokens:  inputTokens,
				OutputTokens: totalOutputTokens,
				StopReason:   stopReason,
				ResponseMetadata: map[string]interface{}{
					"mock":     true,
					"provider": "lorem",
				},
			},
		}
	}()

	return eventChan, nil
}

// streamThinkingBlock streams a thinking block with signature and targetWords words.
// Returns (word count, cutoff flag, error).
// Signature is sent as the LAST delta (matching Anthropic behavior).
func (p *Provider) streamThinkingBlock(ctx context.Context, eventChan chan<- llmprovider.StreamEvent, blockIndex int, targetWords int, model string) (int, bool, error) {
	// Send block start WITHOUT signature (signature comes at the end)
	thinkingType := llmprovider.BlockTypeThinking
	eventChan <- llmprovider.StreamEvent{
		Delta: &llmprovider.BlockDelta{
			BlockIndex: blockIndex,
			BlockType:  &thinkingType,
		},
	}

	// Generate thinking text
	thinkingText := p.generateTextWords(targetWords)
	words := strings.Fields(thinkingText)

	// Get delay based on model
	delay := getStreamDelay(model)

	// Stream words with model-specific delay
	wordsSent := 0
	for _, word := range words {
		select {
		case <-ctx.Done():
			return wordsSent, false, ctx.Err()
		default:
		}

		delta := word + " "
		eventChan <- llmprovider.StreamEvent{
			Delta: &llmprovider.BlockDelta{
				BlockIndex: blockIndex,
				DeltaType:  llmprovider.DeltaTypeThinking,
				TextDelta:  &delta,
			},
		}

		time.Sleep(delay)
		wordsSent++
	}

	// AFTER all thinking text, send signature as final delta
	signature := "4k_a" // Mock Anthropic signature
	eventChan <- llmprovider.StreamEvent{
		Delta: &llmprovider.BlockDelta{
			BlockIndex:     blockIndex,
			DeltaType:      llmprovider.DeltaTypeSignature,
			SignatureDelta: &signature,
		},
	}

	return wordsSent, false, nil
}

// streamToolUseBlock streams a tool_use block with JSON input.
// Returns (token count, error).
func (p *Provider) streamToolUseBlock(ctx context.Context, eventChan chan<- llmprovider.StreamEvent, blockIndex int, tool toolTemplate, model string) (int, error) {
	// Send block start with tool metadata
	toolUseType := llmprovider.BlockTypeToolUse
	toolID := fmt.Sprintf("toolu_%s_%d", tool.name, blockIndex)
	eventChan <- llmprovider.StreamEvent{
		Delta: &llmprovider.BlockDelta{
			BlockIndex:   blockIndex,
			BlockType:    &toolUseType,
			DeltaType:    llmprovider.DeltaTypeToolCallStart,
			ToolCallID:   &toolID,
			ToolCallName: &tool.name,
		},
	}

	// Serialize tool input to JSON
	jsonBytes, err := json.MarshalIndent(tool.input, "", "  ")
	if err != nil {
		p.logger.Error("failed to marshal tool input", "error", err)
		return 0, fmt.Errorf("failed to marshal tool input: %w", err)
	}
	jsonStr := string(jsonBytes)

	// Get delay based on model
	delay := getStreamDelay(model)

	// Stream JSON character by character (simulating incremental JSON building)
	for i, char := range jsonStr {
		select {
		case <-ctx.Done():
			return i, ctx.Err()
		default:
		}

		delta := string(char)
		eventChan <- llmprovider.StreamEvent{
			Delta: &llmprovider.BlockDelta{
				BlockIndex: blockIndex,
				DeltaType:  llmprovider.DeltaTypeJSON,
				JSONDelta:  &delta,
			},
		}

		time.Sleep(delay / 10) // JSON streams faster than words
	}

	// Estimate tokens (rough: 1 token per 4 chars in JSON)
	tokenCount := len(jsonStr) / 4
	return tokenCount, nil
}

// streamTextBlock streams a text block up to maxTokens words.
// Returns (word count, cutoff flag, error).
// For cutoff models, generates extra words and stops at maxTokens limit.
func (p *Provider) streamTextBlock(ctx context.Context, eventChan chan<- llmprovider.StreamEvent, blockIndex int, maxTokens int, model string) (int, bool, error) {
	// Send block start
	textType := llmprovider.BlockTypeText
	eventChan <- llmprovider.StreamEvent{
		Delta: &llmprovider.BlockDelta{
			BlockIndex: blockIndex,
			BlockType:  &textType,
		},
	}

	// Determine target words
	targetWords := maxTokens
	cutoffModel := isCutoffModel(model)

	if cutoffModel {
		// Cutoff models generate 50% more to simulate hitting max_tokens
		targetWords = maxTokens + (maxTokens / 2)
	}

	// Generate paragraphs
	text := p.generateTextWords(targetWords)
	words := strings.Fields(text)

	// Get delay based on model
	delay := getStreamDelay(model)

	// Stream words with potential cutoff
	wordsSent := 0
	for _, word := range words {
		select {
		case <-ctx.Done():
			return wordsSent, false, ctx.Err()
		default:
		}

		// Check if we hit max_tokens limit (only for cutoff models)
		if cutoffModel && wordsSent >= maxTokens {
			// Cut off streaming
			return wordsSent, true, nil
		}

		delta := word + " "
		eventChan <- llmprovider.StreamEvent{
			Delta: &llmprovider.BlockDelta{
				BlockIndex: blockIndex,
				DeltaType:  llmprovider.DeltaTypeTextDelta,
				TextDelta:  &delta,
			},
		}

		time.Sleep(delay)
		wordsSent++
	}

	// If we sent all words without hitting limit, no cutoff
	return wordsSent, false, nil
}

// generateText generates lorem ipsum text with approximately targetChars characters.
func (p *Provider) generateText(targetChars int) string {
	var sb strings.Builder
	for sb.Len() < targetChars {
		paragraph := p.generator.Paragraph(3, 5)
		sb.WriteString(paragraph)
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String())
}

// generateTextWords generates lorem ipsum text with exactly targetWords words.
// Truncates output to ensure we don't exceed the target.
func (p *Provider) generateTextWords(targetWords int) string {
	if targetWords <= 0 {
		return ""
	}

	var allWords []string

	for len(allWords) < targetWords {
		// Generate sentence with 5-15 words
		sentence := p.generator.Sentence(5, 15)
		words := strings.Fields(sentence)
		allWords = append(allWords, words...)
	}

	// Truncate to exactly targetWords
	if len(allWords) > targetWords {
		allWords = allWords[:targetWords]
	}

	return strings.Join(allWords, " ")
}

// streamToolUseBlockFromBuiltIn streams a tool_use block based on BuiltInTool.
// Returns (token count, error).
func (p *Provider) streamToolUseBlockFromBuiltIn(ctx context.Context, eventChan chan<- llmprovider.StreamEvent, blockIndex int, tool *llmprovider.Tool, model string) (int, error) {
	// Generate mock input based on tool function name (OpenAI format)
	var input map[string]interface{}

	switch tool.Function.Name {
	case "search":
		input = map[string]interface{}{
			"query": "lorem ipsum dolor sit amet",
		}
	case "text_editor":
		input = map[string]interface{}{
			"command":   "str_replace",
			"file_path": "/path/to/file.txt",
			"old_str":   "consectetur",
			"new_str":   "adipiscing",
		}
	case "bash":
		input = map[string]interface{}{
			"command": "echo 'lorem ipsum'",
		}
	default:
		// Custom tool - use parameters schema if available
		if len(tool.Function.Parameters.Properties.Values) > 0 {
			// Generate mock values based on schema
			input = map[string]interface{}{
				"param1": "lorem",
				"param2": "ipsum",
			}
		} else {
			input = map[string]interface{}{
				"data": "mock input for " + tool.Function.Name,
			}
		}
	}

	// Send block start with tool metadata
	toolUseType := llmprovider.BlockTypeToolUse
	toolID := fmt.Sprintf("toolu_%s_%d", tool.Function.Name, blockIndex)

	eventChan <- llmprovider.StreamEvent{
		Delta: &llmprovider.BlockDelta{
			BlockIndex:   blockIndex,
			BlockType:    &toolUseType,
			DeltaType:    llmprovider.DeltaTypeToolCallStart,
			ToolCallID:   &toolID,
			ToolCallName: &tool.Function.Name,
		},
	}

	// Note: ExecutionSide is set at the Block level, not in Delta
	// The consumer will need to check tool capabilities to determine execution side

	// Serialize tool input to JSON
	jsonBytes, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		p.logger.Error("failed to marshal tool input", "error", err)
		return 0, fmt.Errorf("failed to marshal tool input: %w", err)
	}
	jsonStr := string(jsonBytes)

	// Get delay based on model
	delay := getStreamDelay(model)

	// Stream JSON character by character (simulating incremental JSON building)
	for i, char := range jsonStr {
		select {
		case <-ctx.Done():
			return i, ctx.Err()
		default:
		}

		delta := string(char)
		eventChan <- llmprovider.StreamEvent{
			Delta: &llmprovider.BlockDelta{
				BlockIndex: blockIndex,
				DeltaType:  llmprovider.DeltaTypeJSON,
				JSONDelta:  &delta,
			},
		}

		time.Sleep(delay / 10) // JSON streams faster than words
	}

	// Estimate tokens (rough: 1 token per 4 chars in JSON)
	tokenCount := len(jsonStr) / 4
	return tokenCount, nil
}

// estimateTokens estimates the token count for a list of messages.
// Uses word count as a rough approximation.
func (p *Provider) estimateTokens(messages []llmprovider.Message) int {
	totalWords := 0
	for _, msg := range messages {
		for _, block := range msg.Blocks {
			if block.TextContent != nil {
				words := len(strings.Fields(*block.TextContent))
				totalWords += words
			}
		}
	}
	return totalWords
}

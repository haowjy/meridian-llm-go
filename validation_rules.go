package llmprovider

import (
	"fmt"
)

// ModelValidationRule checks model-related warnings
type ModelValidationRule struct {
	registry *CapabilityRegistry
}

func (r *ModelValidationRule) Name() string {
	return "Model Validation"
}

func (r *ModelValidationRule) Check(provider string, req *GenerateRequest) []ValidationWarning {
	var warnings []ValidationWarning

	// Check if model exists in capabilities (might be outdated)
	if !r.registry.SupportsModel(provider, req.Model) {
		warnings = append(warnings, ValidationWarning{
			Code:     WarningCodeModelUnknown,
			Category: "model",
			Field:    "model",
			Value:    req.Model,
			Message:  fmt.Sprintf("Model %s not found in %s capabilities (capabilities may be outdated)", req.Model, provider),
			Severity: SeverityWarning,
		})
	}

	return warnings
}

// ToolValidationRule checks tool-related warnings
type ToolValidationRule struct {
	registry *CapabilityRegistry
}

func (r *ToolValidationRule) Name() string {
	return "Tool Validation"
}

func (r *ToolValidationRule) Check(provider string, req *GenerateRequest) []ValidationWarning {
	var warnings []ValidationWarning

	if req.Params == nil || len(req.Params.Tools) == 0 {
		return warnings
	}

	modelCap, err := r.registry.GetModelCapability(provider, req.Model)
	if err != nil {
		// Can't check without capabilities
		return warnings
	}

	if !modelCap.Features.Tools {
		warnings = append(warnings, ValidationWarning{
			Code:     WarningCodeModelDoesNotSupportTools,
			Category: "tool",
			Field:    "tools",
			Value:    len(req.Params.Tools),
			Message:  fmt.Sprintf("Model %s might not support tools", req.Model),
			Severity: SeverityWarning,
		})
		return warnings
	}

	for _, tool := range req.Params.Tools {
		_, err := r.registry.GetToolCapability(provider, req.Model, tool.Function.Name)
		if err != nil {
			warnings = append(warnings, ValidationWarning{
				Code:     WarningCodeToolNotInCapabilities,
				Category: "tool",
				Field:    "tools",
				Value:    tool.Function.Name,
				Message:  fmt.Sprintf("Tool %s might not be supported by %s", tool.Function.Name, req.Model),
				Severity: SeverityInfo,
			})
		}
	}

	return warnings
}

// ThinkingValidationRule checks thinking-related warnings
type ThinkingValidationRule struct {
	registry *CapabilityRegistry
}

func (r *ThinkingValidationRule) Name() string {
	return "Thinking Validation"
}

func (r *ThinkingValidationRule) Check(provider string, req *GenerateRequest) []ValidationWarning {
	var warnings []ValidationWarning

	if req.Params == nil || req.Params.ThinkingEnabled == nil || !*req.Params.ThinkingEnabled {
		return warnings
	}

	modelCap, err := r.registry.GetModelCapability(provider, req.Model)
	if err != nil {
		// Can't check without capabilities
		return warnings
	}

	if !modelCap.Features.Thinking {
		warnings = append(warnings, ValidationWarning{
			Code:     WarningCodeThinkingUnsupported,
			Category: "thinking",
			Field:    "thinking",
			Value:    true,
			Message:  fmt.Sprintf("Model %s might not support extended thinking", req.Model),
			Severity: SeverityWarning,
		})
		return warnings
	}

	// Check explicit budget
	if req.Params.ThinkingBudget != nil {
		budget := *req.Params.ThinkingBudget
		min, max := modelCap.Thinking.MinBudget, modelCap.Thinking.MaxBudget

		if budget < min {
			warnings = append(warnings, ValidationWarning{
				Code:     WarningCodeThinkingBudgetTooLow,
				Category: "thinking",
				Field:    "thinking_budget",
				Value:    budget,
				Message:  fmt.Sprintf("Thinking budget %d below recommended minimum %d", budget, min),
				Severity: SeverityInfo,
			})
		}

		if budget > max {
			warnings = append(warnings, ValidationWarning{
				Code:     WarningCodeThinkingBudgetTooHigh,
				Category: "thinking",
				Field:    "thinking_budget",
				Value:    budget,
				Message:  fmt.Sprintf("Thinking budget %d above maximum %d (will likely fail)", budget, max),
				Severity: SeverityError,
			})
		}
	}

	// Check effort level
	if req.Params.ThinkingLevel != nil {
		_, err := r.registry.ConvertEffortToBudget(provider, req.Model, *req.Params.ThinkingLevel)
		if err != nil {
			warnings = append(warnings, ValidationWarning{
				Code:     WarningCodeThinkingLevelInvalid,
				Category: "thinking",
				Field:    "thinking_level",
				Value:    *req.Params.ThinkingLevel,
				Message:  "Unknown thinking level (valid: low, medium, high)",
				Severity: SeverityWarning,
			})
		}
	}

	return warnings
}

// VisionValidationRule checks vision-related warnings
type VisionValidationRule struct {
	registry *CapabilityRegistry
}

func (r *VisionValidationRule) Name() string {
	return "Vision Validation"
}

func (r *VisionValidationRule) Check(provider string, req *GenerateRequest) []ValidationWarning {
	var warnings []ValidationWarning

	if !hasImageContent(req.Messages) {
		return warnings
	}

	modelCap, err := r.registry.GetModelCapability(provider, req.Model)
	if err != nil {
		// Can't check without capabilities
		return warnings
	}

	if !modelCap.Features.Vision {
		warnings = append(warnings, ValidationWarning{
			Code:     WarningCodeVisionUnsupported,
			Category: "vision",
			Field:    "messages",
			Value:    "contains images",
			Message:  fmt.Sprintf("Model %s might not support vision (check capabilities)", req.Model),
			Severity: SeverityWarning,
		})
	}

	return warnings
}

// ParameterValidationRule checks parameter range warnings
type ParameterValidationRule struct {
	registry *CapabilityRegistry
}

func (r *ParameterValidationRule) Name() string {
	return "Parameter Validation"
}

func (r *ParameterValidationRule) Check(provider string, req *GenerateRequest) []ValidationWarning {
	var warnings []ValidationWarning

	if req.Params == nil {
		return warnings
	}

	providerCaps, err := r.registry.GetProviderCapabilities(provider)
	if err != nil {
		// Can't check without capabilities
		return warnings
	}

	constraints := providerCaps.Constraints

	// Check temperature
	if req.Params.Temperature != nil {
		temp := *req.Params.Temperature
		if temp < constraints.TemperatureMin || temp > constraints.TemperatureMax {
			warnings = append(warnings, ValidationWarning{
				Code:     WarningCodeTemperatureOutOfRange,
				Category: "parameter",
				Field:    "temperature",
				Value:    temp,
				Message:  fmt.Sprintf("Temperature %.2f outside recommended range [%.2f, %.2f]", temp, constraints.TemperatureMin, constraints.TemperatureMax),
				Severity: SeverityWarning,
			})
		}
	}

	// Check top_p
	if req.Params.TopP != nil {
		topP := *req.Params.TopP
		if topP < constraints.TopPMin || topP > constraints.TopPMax {
			warnings = append(warnings, ValidationWarning{
				Code:     WarningCodeTopPOutOfRange,
				Category: "parameter",
				Field:    "top_p",
				Value:    topP,
				Message:  fmt.Sprintf("TopP %.2f outside recommended range [%.2f, %.2f]", topP, constraints.TopPMin, constraints.TopPMax),
				Severity: SeverityWarning,
			})
		}
	}

	// Check top_k
	if req.Params.TopK != nil {
		topK := *req.Params.TopK
		if topK < constraints.TopKMin || topK > constraints.TopKMax {
			warnings = append(warnings, ValidationWarning{
				Code:     WarningCodeTopKOutOfRange,
				Category: "parameter",
				Field:    "top_k",
				Value:    topK,
				Message:  fmt.Sprintf("TopK %d outside recommended range [%d, %d]", topK, constraints.TopKMin, constraints.TopKMax),
				Severity: SeverityWarning,
			})
		}
	}

	return warnings
}

// hasImageContent checks if any messages contain image blocks
func hasImageContent(messages []Message) bool {
	for _, msg := range messages {
		for _, block := range msg.Blocks {
			if block.BlockType == BlockTypeImage {
				return true
			}
		}
	}
	return false
}

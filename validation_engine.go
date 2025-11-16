package llmprovider

import (
	"sync"
)

// ValidationEngine manages validation rules and executes them
type ValidationEngine struct {
	rules []ValidationRule
	mu    sync.RWMutex
}

var (
	globalValidationEngine     *ValidationEngine
	globalValidationEngineOnce sync.Once
)

// GetValidationEngine returns the global validation engine (singleton)
func GetValidationEngine() *ValidationEngine {
	globalValidationEngineOnce.Do(func() {
		globalValidationEngine = &ValidationEngine{
			rules: make([]ValidationRule, 0),
		}
		// Register default rules
		globalValidationEngine.registerDefaultRules()
	})
	return globalValidationEngine
}

// registerDefaultRules registers the built-in validation rules
func (ve *ValidationEngine) registerDefaultRules() {
	registry := GetCapabilityRegistry()

	ve.AddRule(&ModelValidationRule{registry: registry})
	ve.AddRule(&ToolValidationRule{registry: registry})
	ve.AddRule(&ThinkingValidationRule{registry: registry})
	ve.AddRule(&VisionValidationRule{registry: registry})
	ve.AddRule(&ParameterValidationRule{registry: registry})
}

// AddRule adds a validation rule to the engine
func (ve *ValidationEngine) AddRule(rule ValidationRule) {
	ve.mu.Lock()
	defer ve.mu.Unlock()
	ve.rules = append(ve.rules, rule)
}

// RemoveRule removes a validation rule by name
func (ve *ValidationEngine) RemoveRule(name string) bool {
	ve.mu.Lock()
	defer ve.mu.Unlock()

	for i, rule := range ve.rules {
		if rule.Name() == name {
			ve.rules = append(ve.rules[:i], ve.rules[i+1:]...)
			return true
		}
	}
	return false
}

// Validate runs all validation rules and returns warnings
func (ve *ValidationEngine) Validate(provider string, req *GenerateRequest) []ValidationWarning {
	ve.mu.RLock()
	defer ve.mu.RUnlock()

	var warnings []ValidationWarning
	for _, rule := range ve.rules {
		warnings = append(warnings, rule.Check(provider, req)...)
	}
	return warnings
}

// GetValidationWarnings returns potential issues with a request.
// These are INFORMATIONAL - callers can choose to show warnings or ignore them.
// The library does NOT block requests based on warnings.
// Provider APIs validate requests - trust the source of truth.
//
// This is the main entry point for validation. It uses the global validation engine.
func GetValidationWarnings(provider string, req *GenerateRequest) []ValidationWarning {
	return GetValidationEngine().Validate(provider, req)
}

// FilterWarningsBySeverity returns warnings matching the specified severities
func FilterWarningsBySeverity(warnings []ValidationWarning, severities ...Severity) []ValidationWarning {
	filtered := make([]ValidationWarning, 0)
	severityMap := make(map[Severity]bool)
	for _, s := range severities {
		severityMap[s] = true
	}

	for _, w := range warnings {
		if severityMap[w.Severity] {
			filtered = append(filtered, w)
		}
	}
	return filtered
}

// FilterWarningsByCategory returns warnings matching the specified categories
func FilterWarningsByCategory(warnings []ValidationWarning, categories ...string) []ValidationWarning {
	filtered := make([]ValidationWarning, 0)
	categoryMap := make(map[string]bool)
	for _, c := range categories {
		categoryMap[c] = true
	}

	for _, w := range warnings {
		if categoryMap[w.Category] {
			filtered = append(filtered, w)
		}
	}
	return filtered
}

// FilterWarningsByCode returns warnings matching the specified codes
func FilterWarningsByCode(warnings []ValidationWarning, codes ...WarningCode) []ValidationWarning {
	filtered := make([]ValidationWarning, 0)
	codeMap := make(map[WarningCode]bool)
	for _, c := range codes {
		codeMap[c] = true
	}

	for _, w := range warnings {
		if codeMap[w.Code] {
			filtered = append(filtered, w)
		}
	}
	return filtered
}

package manifest

import (
	"fmt"
	"strings"
	"time"
)

// ValidationError represents a manifest validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidateManifest validates an agent manifest against the schema
func ValidateManifest(m *AgentManifest) error {
	var errors []string

	// Validate API version
	if m.APIVersion == "" {
		errors = append(errors, "apiVersion is required")
	} else if m.APIVersion != "syntor.dev/v1" {
		errors = append(errors, fmt.Sprintf("unsupported apiVersion: %s (expected syntor.dev/v1)", m.APIVersion))
	}

	// Validate kind
	if m.Kind == "" {
		errors = append(errors, "kind is required")
	} else if m.Kind != "Agent" {
		errors = append(errors, fmt.Sprintf("invalid kind: %s (expected Agent)", m.Kind))
	}

	// Validate metadata
	if m.Metadata.Name == "" {
		errors = append(errors, "metadata.name is required")
	} else if !isValidName(m.Metadata.Name) {
		errors = append(errors, "metadata.name must be lowercase alphanumeric with hyphens")
	}

	// Validate spec
	if err := validateSpec(&m.Spec); err != nil {
		errors = append(errors, err.Error())
	}

	if len(errors) > 0 {
		return fmt.Errorf("manifest validation failed:\n  - %s", strings.Join(errors, "\n  - "))
	}

	return nil
}

// validateSpec validates the agent spec
func validateSpec(spec *AgentSpec) error {
	var errors []string

	// Validate type
	switch spec.Type {
	case AgentTypeCoordination, AgentTypeSpecialist, AgentTypeWorker:
		// Valid
	case "":
		errors = append(errors, "spec.type is required")
	default:
		errors = append(errors, fmt.Sprintf("invalid spec.type: %s (expected coordination, specialist, or worker)", spec.Type))
	}

	// Validate model
	if spec.Model.Default == "" {
		errors = append(errors, "spec.model.default is required")
	}

	// Validate prompt
	if spec.Prompt.System == "" {
		errors = append(errors, "spec.prompt.system is required")
	}

	// Validate capabilities (at least one required)
	if len(spec.Capabilities) == 0 {
		errors = append(errors, "spec.capabilities must have at least one capability")
	}
	for i, cap := range spec.Capabilities {
		if cap.Name == "" {
			errors = append(errors, fmt.Sprintf("spec.capabilities[%d].name is required", i))
		}
	}

	// Validate context sources
	for i, ctx := range spec.Prompt.Context {
		switch ctx.Type {
		case ContextTypeAgents, ContextTypeProject, ContextTypeMemory, ContextTypeValues:
			// Valid
		case "":
			errors = append(errors, fmt.Sprintf("spec.prompt.context[%d].type is required", i))
		default:
			errors = append(errors, fmt.Sprintf("invalid spec.prompt.context[%d].type: %s", i, ctx.Type))
		}
	}

	// Validate handoff protocol
	if spec.Handoff.Protocol != "" {
		switch spec.Handoff.Protocol {
		case "structured", "natural":
			// Valid
		default:
			errors = append(errors, fmt.Sprintf("invalid spec.handoff.protocol: %s (expected structured or natural)", spec.Handoff.Protocol))
		}
	}

	// Validate constraints
	if spec.Constraints.Timeout != "" {
		if _, err := time.ParseDuration(spec.Constraints.Timeout); err != nil {
			errors = append(errors, fmt.Sprintf("invalid spec.constraints.timeout: %v", err))
		}
	}

	if spec.Constraints.MaxConcurrent < 0 {
		errors = append(errors, "spec.constraints.maxConcurrent must be non-negative")
	}

	if len(errors) > 0 {
		return fmt.Errorf("validation errors: %s", strings.Join(errors, "; "))
	}

	return nil
}

// isValidName checks if a name follows the naming convention
func isValidName(name string) bool {
	if len(name) == 0 || len(name) > 63 {
		return false
	}

	// Must start with lowercase letter
	if name[0] < 'a' || name[0] > 'z' {
		return false
	}

	// Must end with alphanumeric
	last := name[len(name)-1]
	if !((last >= 'a' && last <= 'z') || (last >= '0' && last <= '9')) {
		return false
	}

	// Characters must be lowercase alphanumeric or hyphen
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}

	// No consecutive hyphens
	if strings.Contains(name, "--") {
		return false
	}

	return true
}

// ValidateProjectContext validates a project context file
func ValidateProjectContext(p *ProjectContext) error {
	var errors []string

	// Validate API version
	if p.APIVersion == "" {
		errors = append(errors, "apiVersion is required")
	} else if p.APIVersion != "syntor.dev/v1" {
		errors = append(errors, fmt.Sprintf("unsupported apiVersion: %s", p.APIVersion))
	}

	// Validate kind
	if p.Kind == "" {
		errors = append(errors, "kind is required")
	} else if p.Kind != "ProjectContext" {
		errors = append(errors, fmt.Sprintf("invalid kind: %s (expected ProjectContext)", p.Kind))
	}

	// Validate metadata
	if p.Metadata.Name == "" {
		errors = append(errors, "metadata.name is required")
	}

	if len(errors) > 0 {
		return fmt.Errorf("project context validation failed:\n  - %s", strings.Join(errors, "\n  - "))
	}

	return nil
}

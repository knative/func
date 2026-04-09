package functions

import (
	"errors"
	"fmt"
	"strings"
)

// CompatibilityRule defines which runtimes a builder or deployer supports.
type CompatibilityRule interface {
	// SupportsRuntime returns true if the runtime is supported.
	SupportsRuntime(runtime string) bool

	// Description returns a human-readable explanation of supported runtimes.
	Description() string
}

// RuntimeMatcher is a function that matches runtime strings
type RuntimeMatcher func(runtime string) bool

// CompatibilityRuleImpl implements CompatibilityRule with flexible runtime matching
type CompatibilityRuleImpl struct {
	matcher     RuntimeMatcher
	description string
}

// NewCompatibilityRule creates a new compatibility rule with a matcher and description
func NewCompatibilityRule(matcher RuntimeMatcher, description string) CompatibilityRule {
	return &CompatibilityRuleImpl{
		matcher:     matcher,
		description: description,
	}
}

// SupportsRuntime checks if the runtime matches this rule
func (r *CompatibilityRuleImpl) SupportsRuntime(runtime string) bool {
	return r.matcher(runtime)
}

// Description returns the human-readable description
func (r *CompatibilityRuleImpl) Description() string {
	return r.description
}

// Runtime matcher factory functions

// ExactMatcher matches a specific runtime exactly
func ExactMatcher(runtime string) RuntimeMatcher {
	return func(r string) bool {
		return r == runtime
	}
}

// SuffixMatcher matches runtimes with a specific suffix
func SuffixMatcher(suffix string) RuntimeMatcher {
	return func(runtime string) bool {
		return strings.HasSuffix(runtime, suffix)
	}
}

// NotMatcher inverts another matcher
func NotMatcher(matcher RuntimeMatcher) RuntimeMatcher {
	return func(runtime string) bool {
		return !matcher(runtime)
	}
}

// ErrIncompatibleConfiguration is returned when runtime/builder/deployer combination is invalid.
// Deprecated: prefer ErrIncompatibleBuilder or ErrIncompatibleDeployer for richer error messages.
type ErrIncompatibleConfiguration struct {
	Runtime  string
	Builder  string
	Deployer string
	Reason   string
}

func (e ErrIncompatibleConfiguration) Error() string {
	return fmt.Sprintf("incompatible configuration: runtime=%q, builder=%q, deployer=%q: %s",
		e.Runtime, e.Builder, e.Deployer, e.Reason)
}

// ErrIncompatibility is the sentinel error for all compatibility failures.
// Use errors.Is to detect any compatibility error.
var ErrIncompatibility = errors.New("incompatible runtime/builder/deployer combination")

// ErrIncompatibleBuilder indicates that the builder is not compatible with the runtime.
type ErrIncompatibleBuilder struct {
	Runtime       string
	Builder       string
	ValidBuilders []string
}

func (e *ErrIncompatibleBuilder) Error() string {
	return fmt.Sprintf(
		"builder %q is not compatible with runtime %q. Valid builders for this runtime are: %s",
		e.Builder, e.Runtime, FormatList(e.ValidBuilders),
	)
}

func (e *ErrIncompatibleBuilder) Unwrap() error {
	return ErrIncompatibility
}

// ErrIncompatibleDeployer indicates that the deployer is not compatible with the runtime.
type ErrIncompatibleDeployer struct {
	Runtime        string
	Deployer       string
	ValidDeployers []string
}

func (e *ErrIncompatibleDeployer) Error() string {
	return fmt.Sprintf(
		"deployer %q is not compatible with runtime %q. Valid deployers for this runtime are: %s",
		e.Deployer, e.Runtime, FormatList(e.ValidDeployers),
	)
}

func (e *ErrIncompatibleDeployer) Unwrap() error {
	return ErrIncompatibility
}

// FormatList returns a grammatically correct comma-separated list.
// Examples: "a", "a and b", "a, b, and c"
func FormatList(items []string) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) == 1 {
		return items[0]
	}
	if len(items) == 2 {
		return items[0] + " and " + items[1]
	}
	// For 3+ items: "a, b, c, and d"
	return strings.Join(items[:len(items)-1], ", ") + ", and " + items[len(items)-1]
}

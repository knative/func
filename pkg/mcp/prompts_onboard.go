package mcp

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//go:embed prompts_onboard.md
var onboardPromptBody string

// onboardPromptTemplate is parsed once at init; a parse failure is a
// programming error in the embedded markdown, so panicking is correct.
var onboardPromptTemplate = template.Must(
	template.New("onboard").Parse(onboardPromptBody))

// Defaults applied when the caller omits the argument. Language and registry
// have no default on purpose: there is no sane guess, so the agent is
// instructed to ask the user.
const (
	defaultOnboardTemplate = "http"
	defaultOnboardCluster  = "local"
)

// Accepted argument values. Languages mirror the runtimes shipped in
// templates/; the authoritative list is still the func://languages resource,
// which the prompt instructs the agent to read, so this is a guard against
// obvious typos rather than a second source of truth.
var (
	onboardLanguages = []string{"go", "node", "python", "typescript", "rust", "quarkus", "springboot"}
	onboardTemplates = []string{"http", "cloudevent", "cloudevents"}
	onboardClusters  = []string{"local", "remote"}
)

var onboardPrompt = &mcp.Prompt{
	Name:  "onboard",
	Title: "Onboard a Function",
	Description: "Multi-step onboarding: check prerequisites, choose a language, " +
		"scaffold, run and invoke locally, configure a registry, deploy, invoke " +
		"the live instance, and summarize. All arguments are optional; omitted " +
		"ones are gathered from the user as the steps run.",
	Arguments: []*mcp.PromptArgument{
		{
			Name:        "language",
			Title:       "Language",
			Description: "Target runtime: " + strings.Join(onboardLanguages, ", ") + " (asked for if omitted)",
		},
		{
			Name:        "template",
			Title:       "Template",
			Description: "Function template: http or cloudevents (default: " + defaultOnboardTemplate + ")",
		},
		{
			Name:        "registry",
			Title:       "Registry",
			Description: "Container registry prefix, e.g. docker.io/alice (asked for if omitted)",
		},
		{
			Name:        "cluster",
			Title:       "Cluster",
			Description: "Deployment target: local (e.g. kind) or remote (default: " + defaultOnboardCluster + ")",
		},
	},
}

// onboardParams are the values rendered into the onboarding prompt.
// Language and Registry are empty when the caller did not supply them, which
// the template renders as an explicit "ask the user" instruction.
type onboardParams struct {
	Language string
	Template string
	Registry string
	Cluster  string
	Readonly bool
}

func (s *Server) onboardHandler(_ context.Context, r *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	p, err := newOnboardParams(r, s.readonly.Load())
	if err != nil {
		return nil, err
	}

	var buf strings.Builder
	if err := onboardPromptTemplate.Execute(&buf, p); err != nil {
		return nil, fmt.Errorf("error rendering onboarding prompt: %w", err)
	}

	return newUserPromptResult(onboardPrompt.Description, buf.String()), nil
}

// newOnboardParams validates the request's arguments and applies defaults.
func newOnboardParams(r *mcp.GetPromptRequest, readonly bool) (p onboardParams, err error) {
	p = onboardParams{
		Language: promptArg(r, "language"),
		Template: promptArg(r, "template"),
		// The registry is a case-sensitive image reference prefix, so unlike
		// the other arguments it must not be lowercased.
		Registry: strings.TrimSpace(rawPromptArg(r, "registry")),
		Cluster:  promptArg(r, "cluster"),
		Readonly: readonly,
	}

	if err = validateChoice("language", p.Language, onboardLanguages); err != nil {
		return
	}
	if err = validateChoice("template", p.Template, onboardTemplates); err != nil {
		return
	}
	if err = validateChoice("cluster", p.Cluster, onboardClusters); err != nil {
		return
	}

	if p.Template == "" {
		p.Template = defaultOnboardTemplate
	}
	// "cloudevent" is accepted as an alias because that is how the format is
	// spelled elsewhere (e.g. invoke's --format), but the template shipped in
	// templates/<runtime>/ is named "cloudevents"; normalize so the value
	// handed to the create tool is one that actually exists.
	if p.Template == "cloudevent" {
		p.Template = "cloudevents"
	}
	if p.Cluster == "" {
		p.Cluster = defaultOnboardCluster
	}
	return
}

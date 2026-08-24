package mcp

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"knative.dev/func/pkg/oci"
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

// onboardClusters are the accepted values for the cluster argument. Unlike
// language, template and registry, cluster is not passed through to func: it
// only selects which guidance the prompt renders, so the set of valid values
// is fully known here and can be validated.
var onboardClusters = []string{"local", "remote"}

var onboardPrompt = &mcp.Prompt{
	Name:  "onboard",
	Title: "Onboard a Function",
	Description: "Multi-step onboarding: check prerequisites, choose a language, " +
		"scaffold, configure a registry, run and invoke locally, deploy, invoke " +
		"the live instance, and summarize. All arguments are optional; omitted " +
		"ones are gathered from the user as the steps run.",
	Arguments: []*mcp.PromptArgument{
		{
			Name:  "language",
			Title: "Language",
			Description: "Target runtime, as reported by the func://languages resource, " +
				"e.g. go, node, python (asked for if omitted)",
		},
		{
			Name:  "template",
			Title: "Template",
			Description: "Function template available for the chosen runtime, as reported " +
				"by the func://templates resource (default: " + defaultOnboardTemplate + ")",
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
	// HostBuilder reports whether the host builder supports Language, and is
	// what the deploy step branches on rather than naming runtimes itself.
	// False when no language was supplied.
	HostBuilder bool
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
//
// Only cluster is validated against a fixed set. Language, template and
// registry are passed through: the runtimes and templates which actually
// exist depend on the installed binary and on any template repositories the
// user has added (see the repository_add tool), so a hardcoded list here
// would reject legitimate values. The prompt instructs the agent to verify
// them against the func://languages and func://templates resources instead,
// and create reports plainly when either is wrong.
func newOnboardParams(r *mcp.GetPromptRequest, readonly bool) (p onboardParams, err error) {
	p = onboardParams{
		// Values handed to func are only trimmed, never case-folded: a
		// registry is a case-sensitive image reference prefix, and a runtime
		// or template name is matched against a directory name in a template
		// repository.
		Language: strings.TrimSpace(rawPromptArg(r, "language")),
		Template: strings.TrimSpace(rawPromptArg(r, "template")),
		Registry: strings.TrimSpace(rawPromptArg(r, "registry")),
		Cluster:  promptArg(r, "cluster"),
		Readonly: readonly,
	}

	if err = validateChoice("cluster", p.Cluster, onboardClusters); err != nil {
		return
	}

	switch {
	case p.Template == "":
		p.Template = defaultOnboardTemplate
	case strings.EqualFold(p.Template, "cloudevent"):
		// "cloudevent" is how the format is spelled elsewhere (e.g. invoke's
		// --format), but no runtime ships a template by that name: the one on
		// disk is "cloudevents". Correcting it is safe precisely because the
		// value given cannot be valid as-is.
		p.Template = "cloudevents"
	}

	if p.Cluster == "" {
		p.Cluster = defaultOnboardCluster
	}

	// Single source of truth for which runtimes the host builder supports.
	p.HostBuilder = oci.IsSupported(strings.ToLower(p.Language))
	return
}

package mcp

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// getPromptText invokes the named prompt and returns the text of its single
// message, failing the test if the prompt does not have exactly one text
// message.
func getPromptText(t *testing.T, client *mcp.ClientSession, name string, args map[string]string) string {
	t.Helper()

	result, err := client.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("GetPrompt(%q) failed: %v", name, err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}
	m := result.Messages[0]
	if m.Role != "user" {
		t.Errorf("expected role 'user', got %q", m.Role)
	}
	content, ok := m.Content.(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", m.Content)
	}
	if strings.TrimSpace(content.Text) == "" {
		t.Fatal("prompt text is empty")
	}
	return content.Text
}

// TestPrompt_OnboardListed ensures the onboarding prompt is advertised by the
// server, with all four of its documented (and optional) arguments.
func TestPrompt_OnboardListed(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	result, err := client.ListPrompts(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}

	var (
		onboard mcp.Prompt
		found   bool
	)
	for _, p := range result.Prompts {
		if p.Name == "onboard" {
			onboard, found = *p, true
			break
		}
	}
	if !found {
		t.Fatal("prompt 'onboard' not found")
	}
	if onboard.Description == "" {
		t.Error("prompt 'onboard' has no description")
	}

	want := map[string]bool{"language": false, "template": false, "registry": false, "cluster": false}
	for _, a := range onboard.Arguments {
		if _, ok := want[a.Name]; !ok {
			t.Errorf("unexpected argument %q", a.Name)
			continue
		}
		want[a.Name] = true
		if a.Description == "" {
			t.Errorf("argument %q has no description", a.Name)
		}
		// All arguments are optional: any omitted value is gathered from
		// the user by the agent as the steps run.
		if a.Required {
			t.Errorf("argument %q should be optional", a.Name)
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing argument %q", name)
		}
	}
}

// TestPrompt_OnboardSteps ensures every documented step of the onboarding
// workflow is present, and that each names the tool or resource it depends
// on. The prompt is the contract with the agent, so a step silently
// disappearing from the markdown must fail the build.
func TestPrompt_OnboardSteps(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	text := getPromptText(t, client, "onboard", nil)

	for _, want := range []string{
		"Step 1 — Prerequisites",
		"Step 2 — Language selection",
		"Step 3 — Scaffold",
		"Step 4 — Local run and invoke",
		"Step 5 — Registry configuration",
		"Step 6 — Deploy",
		"Step 7 — Remote invoke",
		"Step 8 — Summary",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("prompt missing %q", want)
		}
	}

	// Each step must reference the tool or resource that implements it.
	for _, want := range []string{
		"`version`",        // step 1
		"func://languages", // step 2
		"`create`",         // step 3
		"`run`",            // step 4
		"`invoke`",         // steps 4 and 7
		"`run_stop`",       // step 4
		"`deploy`",         // step 6
		"`describe`",       // step 6
		`target: "local"`,  // step 4
		`target: "remote"`, // step 7
	} {
		if !strings.Contains(text, want) {
			t.Errorf("prompt missing reference to %q", want)
		}
	}
}

// TestPrompt_OnboardDefaults ensures omitted arguments produce their
// documented defaults, and that the arguments with no default instruct the
// agent to ask the user rather than guessing.
func TestPrompt_OnboardDefaults(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	text := getPromptText(t, client, "onboard", nil)

	if !strings.Contains(text, "| template  | `http` |") {
		t.Error("template did not default to 'http'")
	}
	if !strings.Contains(text, "| cluster   | `local`") {
		t.Error("cluster did not default to 'local'")
	}
	// Language and registry have no sane default; the agent must ask.
	if strings.Count(text, "**not provided**") != 2 {
		t.Errorf("expected language and registry to be marked 'not provided', got:\n%s", text)
	}
	if !strings.Contains(text, "ask the user in step 2") {
		t.Error("prompt does not instruct the agent to ask for the language")
	}
	if !strings.Contains(text, "ask the user in step 5") {
		t.Error("prompt does not instruct the agent to ask for the registry")
	}
}

// TestPrompt_OnboardParameterized ensures supplied arguments are rendered
// into the prompt and are not re-asked for.
func TestPrompt_OnboardParameterized(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	text := getPromptText(t, client, "onboard", map[string]string{
		"language": "node",
		"template": "cloudevents",
		"registry": "ghcr.io/alice",
		"cluster":  "remote",
	})

	for _, want := range []string{
		"| language  | `node` |",
		"| template  | `cloudevents` |",
		"| registry  | `ghcr.io/alice` |",
		"| cluster   | `remote`",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	if strings.Contains(text, "**not provided**") {
		t.Error("prompt asks for a parameter which was supplied")
	}
	// A remote cluster changes the step 1 guidance.
	if !strings.Contains(text, "which cluster and\nnamespace") {
		t.Error("remote cluster guidance missing from step 1")
	}
}

// TestPrompt_OnboardNormalization ensures argument values are normalized:
// case and surrounding whitespace are insignificant for the enumerated
// arguments, the "cloudevent" alias resolves to the template name which
// actually exists, and the registry (an image reference prefix) keeps its
// case.
func TestPrompt_OnboardNormalization(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	text := getPromptText(t, client, "onboard", map[string]string{
		"language": "  GO ",
		"template": "CloudEvent",
		"registry": " docker.io/Alice ",
		"cluster":  "Local",
	})

	if !strings.Contains(text, "| language  | `go` |") {
		t.Error("language was not normalized to 'go'")
	}
	if !strings.Contains(text, "| template  | `cloudevents` |") {
		t.Error("'cloudevent' was not normalized to the 'cloudevents' template name")
	}
	if !strings.Contains(text, "| cluster   | `local`") {
		t.Error("cluster was not normalized to 'local'")
	}
	if !strings.Contains(text, "| registry  | `docker.io/Alice` |") {
		t.Error("registry case was not preserved (image references are case-sensitive)")
	}
}

// TestPrompt_OnboardBuilder ensures the deploy step recommends the host
// builder for the runtimes which default to it, and stays neutral otherwise.
func TestPrompt_OnboardBuilder(t *testing.T) {
	tests := []struct {
		language string
		want     bool
	}{
		{"go", true},
		{"python", true},
		{"node", false},
		{"rust", false},
	}

	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range tests {
		t.Run(tt.language, func(t *testing.T) {
			text := getPromptText(t, client, "onboard", map[string]string{
				"language": tt.language,
			})
			got := strings.Contains(text, "`builder`: `host`")
			if got != tt.want {
				t.Errorf("host builder recommended = %v, want %v for %s", got, tt.want, tt.language)
			}
		})
	}
}

// TestPrompt_OnboardInvalidArguments ensures unrecognized values are rejected
// with an actionable error rather than being rendered into the prompt, where
// they would send the agent off to run a command that cannot succeed.
func TestPrompt_OnboardInvalidArguments(t *testing.T) {
	tests := []struct {
		name string
		args map[string]string
	}{
		{"language", map[string]string{"language": "cobol"}},
		{"template", map[string]string{"template": "grpc"}},
		{"cluster", map[string]string{"cluster": "kind"}},
	}

	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.GetPrompt(t.Context(), &mcp.GetPromptParams{
				Name:      "onboard",
				Arguments: tt.args,
			})
			if err == nil {
				t.Fatalf("expected an error for invalid %s", tt.name)
			}
			if !strings.Contains(err.Error(), tt.name) {
				t.Errorf("error does not name the offending argument %q: %v", tt.name, err)
			}
			if !strings.Contains(err.Error(), "must be one of") {
				t.Errorf("error does not list the valid values: %v", err)
			}
		})
	}
}

// TestPrompt_OnboardReadonly ensures the prompt adapts to read-only mode: the
// steps which mutate cluster state are omitted rather than sent to an agent
// that would be refused, and the user is told how to enable them.
func TestPrompt_OnboardReadonly(t *testing.T) {
	client, _, err := newTestPairWithReadonly(t, true)
	if err != nil {
		t.Fatal(err)
	}

	text := getPromptText(t, client, "onboard", nil)

	if !strings.Contains(text, EnvMCPWrite) {
		t.Errorf("readonly prompt does not mention %s", EnvMCPWrite)
	}
	for _, unwanted := range []string{
		"Step 5 — Registry configuration",
		"Step 6 — Deploy",
		"Step 7 — Remote invoke",
	} {
		if strings.Contains(text, unwanted) {
			t.Errorf("readonly prompt should not include %q", unwanted)
		}
	}
	// The local half of onboarding still applies, and so does the summary.
	for _, want := range []string{
		"Step 1 — Prerequisites",
		"Step 4 — Local run and invoke",
		"Step 8 — Summary",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("readonly prompt missing %q", want)
		}
	}
}

// TestPrompt_OnboardNilParams ensures the argument helpers tolerate a request
// carrying no arguments at all, which is how the prompt is invoked by a
// client that offers no argument entry.
func TestPrompt_OnboardNilParams(t *testing.T) {
	p, err := newOnboardParams(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if p.Template != defaultOnboardTemplate {
		t.Errorf("expected template %q, got %q", defaultOnboardTemplate, p.Template)
	}
	if p.Cluster != defaultOnboardCluster {
		t.Errorf("expected cluster %q, got %q", defaultOnboardCluster, p.Cluster)
	}
	if p.Language != "" || p.Registry != "" {
		t.Errorf("expected empty language and registry, got %q and %q", p.Language, p.Registry)
	}
}

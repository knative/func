package mcp

import (
	"fmt"
	"regexp"
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

// paramValue returns the value the prompt's session-parameter table reports
// for the named parameter, or "" when the parameter has no row. Matching
// ignores the table's column padding, so realigning the markdown does not
// break these tests: only the rendered value is asserted on.
func paramValue(t *testing.T, text, name string) string {
	t.Helper()

	re := regexp.MustCompile(fmt.Sprintf(`(?m)^\|\s*%s\s*\|\s*(.*?)\s*\|\s*$`, regexp.QuoteMeta(name)))
	m := re.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	return m[1]
}

// assertParam fails unless the session-parameter table reports want for the
// named parameter.
func assertParam(t *testing.T, text, name, want string) {
	t.Helper()

	if got := paramValue(t, text, name); got != want {
		t.Errorf("parameter %q = %q, want %q", name, got, want)
	}
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
// workflow is present, in order, and that each names the tool or resource it
// depends on. The prompt is the contract with the agent, so a step silently
// disappearing from the markdown must fail the build.
func TestPrompt_OnboardSteps(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	text := getPromptText(t, client, "onboard", nil)

	// The registry precedes the local run deliberately: a containerized
	// build has to name an image, so `run` fails without a registry.
	steps := []string{
		"Step 1 — Prerequisites",
		"Step 2 — Language selection",
		"Step 3 — Scaffold",
		"Step 4 — Registry configuration",
		"Step 5 — Local run and invoke",
		"Step 6 — Deploy",
		"Step 7 — Remote invoke",
		"Step 8 — Summary",
	}
	at := -1
	for _, want := range steps {
		i := strings.Index(text, want)
		if i < 0 {
			t.Errorf("prompt missing %q", want)
			continue
		}
		if i < at {
			t.Errorf("step %q is out of order", want)
		}
		at = i
	}

	// Each step must reference the tool or resource that implements it.
	for _, want := range []string{
		"`version`",         // step 1
		"func://languages",  // step 2
		"`create`",          // step 3
		"func://templates",  // step 3
		"`run`",             // step 5
		"`invoke`",          // steps 5 and 7
		"`run_stop`",        // step 5
		"`deploy`",          // step 6
		"`describe`",        // step 6
		`target: "local"`,   // step 5
		`target: "remote"`,  // step 7
		"registry required", // step 4, on why it precedes the run
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

	assertParam(t, text, "template", "`"+defaultOnboardTemplate+"`")
	if got := paramValue(t, text, "cluster"); !strings.HasPrefix(got, "`"+defaultOnboardCluster+"`") {
		t.Errorf("cluster = %q, want it to start with %q", got, "`"+defaultOnboardCluster+"`")
	}

	// Language and registry have no sane default; the agent must ask, and
	// must be pointed at the step which actually gathers the value.
	for _, tt := range []struct{ param, step string }{
		{"language", "step 2"},
		{"registry", "step 4"},
	} {
		got := paramValue(t, text, tt.param)
		if !strings.Contains(got, "**not provided**") {
			t.Errorf("%s = %q, want it marked 'not provided'", tt.param, got)
		}
		if !strings.Contains(got, "ask the user in "+tt.step) {
			t.Errorf("%s = %q, want it to point at %s", tt.param, got, tt.step)
		}
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

	assertParam(t, text, "language", "`node`")
	assertParam(t, text, "template", "`cloudevents`")
	assertParam(t, text, "registry", "`ghcr.io/alice`")
	if got := paramValue(t, text, "cluster"); !strings.HasPrefix(got, "`remote`") {
		t.Errorf("cluster = %q, want it to start with `remote`", got)
	}
	if strings.Contains(text, "**not provided**") {
		t.Error("prompt asks for a parameter which was supplied")
	}
	// A remote cluster changes the step 1 guidance.
	if !strings.Contains(text, "Because `cluster` is `remote`") {
		t.Error("remote cluster guidance missing from step 1")
	}
}

// TestPrompt_OnboardNormalization ensures surrounding whitespace is
// insignificant, that values handed to func keep their case (a registry is a
// case-sensitive image reference prefix, and runtime and template names are
// matched against directory names in a template repository), and that the
// prompt-internal cluster argument is case-insensitive.
func TestPrompt_OnboardNormalization(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	text := getPromptText(t, client, "onboard", map[string]string{
		"language": "  go ",
		"template": " MyTemplate ",
		"registry": " docker.io/Alice ",
		"cluster":  "Local",
	})

	assertParam(t, text, "language", "`go`")
	assertParam(t, text, "template", "`MyTemplate`")
	assertParam(t, text, "registry", "`docker.io/Alice`")
	if got := paramValue(t, text, "cluster"); !strings.HasPrefix(got, "`local`") {
		t.Errorf("cluster = %q, want it lowercased to `local`", got)
	}
}

// TestPrompt_OnboardCloudeventAlias ensures "cloudevent" — how the format is
// spelled by invoke's --format, but not the name of any template on disk —
// resolves to the "cloudevents" template which does exist, regardless of case.
func TestPrompt_OnboardCloudeventAlias(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	for _, given := range []string{"cloudevent", "CloudEvent", " cloudevent "} {
		t.Run(given, func(t *testing.T) {
			text := getPromptText(t, client, "onboard", map[string]string{"template": given})
			assertParam(t, text, "template", "`cloudevents`")
		})
	}
}

// TestPrompt_OnboardPassesThroughUnknownValues ensures runtimes and templates
// the prompt has never heard of are still rendered. Which ones exist depends
// on the installed binary and on any template repositories the user has added
// (see the repository_add tool), so rejecting them here would make the prompt
// unusable for exactly those users.
func TestPrompt_OnboardPassesThroughUnknownValues(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	text := getPromptText(t, client, "onboard", map[string]string{
		"language": "mylang",
		"template": "mytemplate",
	})

	assertParam(t, text, "language", "`mylang`")
	assertParam(t, text, "template", "`mytemplate`")
	// The agent is told to check the value against the authoritative list
	// rather than trusting it.
	if !strings.Contains(text, "func://languages") {
		t.Error("prompt does not point the agent at func://languages")
	}
}

// TestPrompt_OnboardBuilder ensures the deploy step recommends the host
// builder for the runtimes which support it, stays silent for those which do
// not, and gives generic guidance when the language is not yet known. The
// authority is pkg/oci; the prompt must not carry its own copy of the list.
func TestPrompt_OnboardBuilder(t *testing.T) {
	tests := []struct {
		name     string
		language string
		want     bool
	}{
		{"go", "go", true},
		{"python", "python", true},
		{"node", "node", false},
		{"rust", "rust", false},
		{"mixed case", "Go", true},
		{"unset", "", false},
	}

	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]string{}
			if tt.language != "" {
				args["language"] = tt.language
			}
			text := getPromptText(t, client, "onboard", args)

			got := strings.Contains(text, "`builder`: `host`")
			if got != tt.want {
				t.Errorf("host builder recommended = %v, want %v for %q", got, tt.want, tt.language)
			}
			// With no language yet, the agent gets generic guidance instead
			// of a naked recommendation it cannot evaluate.
			if tt.language == "" && !strings.Contains(text, "`builder`: omit it unless") {
				t.Error("expected generic builder guidance when no language is supplied")
			}
		})
	}
}

// TestPrompt_OnboardInvalidCluster ensures an unrecognized cluster — the one
// argument whose valid values the prompt itself enumerates, because it only
// selects guidance and is never passed to func — is rejected with an
// actionable error rather than rendered.
func TestPrompt_OnboardInvalidCluster(t *testing.T) {
	client, _, err := newTestPair(t)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.GetPrompt(t.Context(), &mcp.GetPromptParams{
		Name:      "onboard",
		Arguments: map[string]string{"cluster": "kind"},
	})
	if err == nil {
		t.Fatal("expected an error for an invalid cluster")
	}
	if !strings.Contains(err.Error(), "cluster") {
		t.Errorf("error does not name the offending argument: %v", err)
	}
	if !strings.Contains(err.Error(), "must be one of") {
		t.Errorf("error does not list the valid values: %v", err)
	}
}

// TestPrompt_OnboardReadonly ensures the prompt adapts to read-only mode: the
// deploy and the remote invoke which depends on it are omitted rather than
// sent to an agent that would be refused, the user is told how to enable
// them, and nothing which survives refers to a step which no longer exists.
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
		"Step 6 — Deploy",
		"Step 7 — Remote invoke",
	} {
		if strings.Contains(text, unwanted) {
			t.Errorf("readonly prompt should not include %q", unwanted)
		}
	}
	// The local half of onboarding still applies, and so does the summary.
	// The registry step in particular is NOT skipped: the local build in
	// step 5 has to name an image, whether or not it is ever pushed.
	for _, want := range []string{
		"Step 1 — Prerequisites",
		"Step 4 — Registry configuration",
		"Step 5 — Local run and invoke",
		"Step 8 — Summary",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("readonly prompt missing %q", want)
		}
	}
	// Every step the prompt still points the agent at must exist in it.
	for _, ref := range regexp.MustCompile(`step (\d)`).FindAllStringSubmatch(text, -1) {
		if strings.Contains(text, "(step "+ref[1]+")") {
			continue // the read-only notice names the omitted steps on purpose
		}
		if !strings.Contains(text, "## Step "+ref[1]+" — ") {
			t.Errorf("readonly prompt refers to %q, which it omits", ref[0])
		}
	}
	// Nor may it close by telling the user to do the thing that is refused.
	if strings.Contains(text, "re-run `deploy`") {
		t.Error("readonly prompt tells the user to re-run deploy, which is refused")
	}
}

// TestPrompt_OnboardNilParams ensures the argument helpers tolerate a request
// carrying no params at all. No client sends that — an argumentless GetPrompt
// arrives with an empty map — so this covers the defensive nil guards in the
// helpers rather than a reachable code path.
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
	if p.HostBuilder {
		t.Error("expected HostBuilder to be false with no language")
	}
}

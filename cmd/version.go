package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/ory/viper"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	"knative.dev/func/pkg/config"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/k8s"
)

func NewVersionCmd(version Version) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Function client version information",
		Long: `
NAME
	{{rootCmdUse}} version - function version information.

SYNOPSIS
	{{rootCmdUse}} version [-v|--verbose] [-o|--output]

DESCRIPTION
	Print version information.  Use the --verbose option to see date stamp and
	associated git source control hash if available.  Use the --output option
	to specify the output format (human|json|yaml).

	o Print the functions version
	  $ {{rootCmdUse}} version

	o Print the functions version along with source git commit hash and other
	  metadata.
	  $ {{rootCmdUse}} version -v

	o Print the version information in JSON format
	  $ {{rootCmdUse}} version --output json

	o Print verbose version information in YAML format
	  $ {{rootCmdUse}} version -v -o yaml

`,
		SuggestFor: []string{"vers", "version"}, //nolint:misspell
		PreRunE:    bindEnv("verbose", "output"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runVersion(cmd, version)
		},
	}
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	// Add flags
	cmd.Flags().StringP("output", "o", "human", "Output format (human|json|yaml) ($FUNC_OUTPUT)")
	addVerboseFlag(cmd, cfg.Verbose)

	// Add flag completion
	if err := cmd.RegisterFlagCompletionFunc("output", CompleteOutputFormatList); err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}

	return cmd
}

// Run
func runVersion(cmd *cobra.Command, v Version) error {
	verbose := viper.GetBool("verbose")
	output := viper.GetString("output")

	// Set verbose flag
	v.Verbose = verbose

	// Initialize the default value to the zero semver with a descriptive
	// metadata tag indicating this must have been built from source if
	// undefined:
	if v.Vers == "" {
		v.Vers = DefaultVersion
	}

	// Kver and Hash are already set from build via ldflags,
	// injected into the Version struct at startup (see pkg/app/app.go).
	// Populate image fields from k8s package constants
	v.SocatImage = k8s.SocatImage
	v.TarImage = k8s.TarImage

	latestMW, err := fn.LatestMiddlewareVersions()
	if err != nil {
		return fmt.Errorf("error fetching latest middleware versions: %w", err)
	}
	v.MiddlewareVersions = latestMW

	return write(cmd.OutOrStdout(), v, output)
}

// Version information populated on build.
type Version struct {
	// Version tag of the git commit, or 'tip' if no tag.
	Vers string `json:"version,omitempty" yaml:"version,omitempty"`
	// Kver is the version of knative in which func was most recently
	// If the build is not tagged as being released with a specific Knative
	// build, this is the most recent version of knative along with a suffix
	// consisting of the number of commits which have been added since it was
	// included in Knative.
	Kver string `json:"knative,omitempty" yaml:"knative,omitempty"`
	// Hash of the currently active git commit on build.
	Hash string `json:"commit,omitempty" yaml:"commit,omitempty"`
	// SocatImage is the socat image used by the function.
	SocatImage string `json:"socatImage,omitempty" yaml:"socatImage,omitempty"`
	// TarImage is the tar image used by the function.
	TarImage string `json:"tarImage,omitempty" yaml:"tarImage,omitempty"`
	// MiddlewareVersions provides information about the latest middleware version
	// for a given platform and invokeType
	MiddlewareVersions MiddlewareVersions `json:"middlewareVersions,omitempty" yaml:"middlewareVersions,omitempty"`

	// Verbose printing enabled for the string representation.
	Verbose bool `json:"-" yaml:"-"`
}

// Return the stringification of the Version struct.
func (v Version) String() string {
	if v.Verbose {
		return v.StringVerbose()
	}
	_ = semver.MustParse(v.Vers)
	return v.Vers
}

// StringVerbose returns the version along with extended version metadata.
// Fields with empty values are omitted.
func (v Version) StringVerbose() string {
	var sb strings.Builder
	sb.WriteString("Version: " + v.Vers + "\n")
	if v.Kver != "" {
		sb.WriteString("Knative: " + strings.TrimPrefix(v.Kver, "knative-") + "\n")
	}
	if v.Hash != "" {
		sb.WriteString("Commit: " + v.Hash + "\n")
	}
	if v.SocatImage != "" {
		sb.WriteString("SocatImage: " + v.SocatImage + "\n")
	}
	if v.TarImage != "" {
		sb.WriteString("TarImage: " + v.TarImage + "\n")
	}
	if mw := v.MiddlewareVersions.String(); mw != "" {
		sb.WriteString("Middleware Versions:\n" + mw)
	}
	// Trim trailing newlines: String methods should return bare content; the
	// caller is responsible for adding output termination. Every field write
	// ends in "\n", so the result would otherwise produce a double newline
	// when a caller (such as Human) adds its own.
	return strings.TrimRight(sb.String(), "\n")
}

// Human prints version information in human-readable format.
func (v Version) Human(w io.Writer) error {
	if v.Verbose {
		_, err := fmt.Fprintln(w, v.StringVerbose())
		return err
	}
	_, err := fmt.Fprintf(w, "%s\n", v.Vers)
	return err
}

// Plain prints version information in plain format (same as human for version).
func (v Version) Plain(w io.Writer) error {
	return v.Human(w)
}

// JSON prints version information in JSON format.
func (v Version) JSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// YAML prints version information in YAML format.
func (v Version) YAML(w io.Writer) error {
	enc := yaml.NewEncoder(w)
	defer enc.Close()
	return enc.Encode(v)
}

// URL is not supported for version command.
func (v Version) URL(w io.Writer) error {
	return fmt.Errorf("URL format not supported for version command")
}

type MiddlewareVersions map[string]map[string]string

func (mv MiddlewareVersions) String() string {
	sb := strings.Builder{}
	for platform, pInfo := range mv {
		sb.WriteString("  " + platform + ":\n")
		for invokeType, version := range pInfo {
			sb.WriteString("    " + invokeType + ": " + version + "\n")
		}
	}

	return sb.String()
}

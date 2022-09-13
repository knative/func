// Copyright © 2020 The Knative Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package templates

import (
	"strings"
	"unicode"
)

// Templates for help & usage messages. These have been initially taken over from
// https://github.com/kubernetes/kubectl/blob/f9e4fa6b9cff11b6e2949b76680b8cd5b8192eab/pkg/util/templates/templates.go
// and adapted to the specific needs of `kn`

const (
	// sectionUsage is the help template section that displays the command's usage.
	sectionUsage = `{{if (ne .UseLine "")}}Usage:
  {{useLine .}}

{{end}}`

	// sectionAliases is the help template section that displays the command's aliases.
	sectionAliases = `{{if ne (len .Aliases) 0}}Aliases:
  {{.NameAndAliases}}

{{end}}`

	// sectionExamples is the help template section that displays command examples.
	sectionExamples = `{{if .HasExample}}Examples:
{{trimRight .Example}}

{{end}}`

	// sectionCommandGroups is the grouped help message
	sectionCommandGroups = `{{if isRootCmd .}}{{cmdGroupsString}}

{{end}}`

	// sectionSubCommands is the help template section that displays the command's subcommands.
	sectionSubCommands = `{{if and (not (isRootCmd .)) .HasAvailableSubCommands}}{{subCommandsString .}}

{{end}}`

	// sectionFlags is the help template section that displays the command's flags.
	sectionFlags = `{{$visibleFlags := visibleFlags .}}{{ if $visibleFlags.HasFlags}}Flags:
{{trimRight (flagsUsages $visibleFlags)}}

{{end}}`

	// sectionGlobalFlags is the help template section that displays inherited flags.
	sectionGlobalFlags = `{{if .HasInheritedFlags}}Global Flags:
{{trimRight (flagsUsages .InheritedFlags)}}

{{end}}`

	// sectionTipsHelp is the help template section that displays the '--help' hint.
	sectionTipsHelp = `{{if .HasSubCommands}}Use "{{rootCmdName}} [command] --help" for more information about a given command.
{{end}}`
)

// usageTemplate if the template for 'usage' used by most commands.
func usageTemplate() string {
	sections := []string{
		sectionUsage,
		sectionAliases,
		sectionExamples,
		sectionCommandGroups,
		sectionSubCommands,
		sectionFlags,
		sectionGlobalFlags,
		sectionTipsHelp,
	}
	return strings.TrimRightFunc(strings.Join(sections, ""), unicode.IsSpace) + "\n"
}

// helpTemplate is the template for 'help' used by most commands.
func helpTemplate(message string) string {
	if len(message) == 0 {
		message = `{{with or .Long .Short }}{{. | trim}}{{end}}`
	}
	return message + "\n\n" + `{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`
}

package e2e

import "strings"

// CleanOutput Some commands, such as deploy command, spans spinner chars and cursor shifts at output which are captured and merged
// regular output messages. This functions is meant to remove these chars in order to facilitate tests assertions and data extraction from output
func CleanOutput(stdOutput string) string {
	toRemove := []string{
		"🕛 ",
		"🕐 ",
		"🕑 ",
		"🕒 ",
		"🕓 ",
		"🕔 ",
		"🕕 ",
		"🕖 ",
		"🕗 ",
		"🕘 ",
		"🕙 ",
		"🕚 ",
		"\033[1A",
		"\033[1B",
		"\033[K",
	}
	for _, c := range toRemove {
		stdOutput = strings.ReplaceAll(stdOutput, c, "")
	}
	return stdOutput
}

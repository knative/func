package utils

import (
	"bufio"
	"regexp"
	"strings"
)

// StringLines split string
func StringLines(s string) []string {
	return strings.Split(s, "\n")
}

// StringHead return first line of a string with multiple lines
func StringHead(s string) string {
	sl := strings.SplitN(s, "\n", 1)
	if len(sl) > 0 {
		return sl[0]
	}
	return ""
}

// StringExtractLineMatching Extract the first line matching the given regexp condition
func StringExtractLineMatching(s string, matchRexp string) string {

	r := regexp.MustCompile(matchRexp)
	scanner := bufio.NewScanner(strings.NewReader(s))
	res := ""
	for scanner.Scan() {
		line := scanner.Text()
		if r.MatchString(line) {
			res = line
			break
		}
	}
	return res
}

// StringExtractLineFieldsMatching Extract fields which line field matches a given regexp condition
func StringExtractLineFieldsMatching(s string, fieldIndex int, matchRexp string) []string {

	r := regexp.MustCompile(matchRexp)
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) >= fieldIndex {
			if r.MatchString(fields[fieldIndex]) {
				return fields
			}
		}
	}
	return []string{}

}

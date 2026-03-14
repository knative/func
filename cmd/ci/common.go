package ci

import "fmt"

func determineRunner(selfHosted bool) string {
	if selfHosted {
		return "self-hosted"
	}
	return "ubuntu-latest"
}

func secretsPrefix(s string) string {
	return "secrets." + s
}

func varsPrefix(s string) string {
	return "vars." + s
}

func newSecret(key string) string {
	return fmt.Sprintf("${{ %s }}", secretsPrefix(key))
}

func newVariable(key string) string {
	return fmt.Sprintf("${{ %s }}", varsPrefix(key))
}

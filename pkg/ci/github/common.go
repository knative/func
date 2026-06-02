package github

import "fmt"

func determineBuilder(runtime string, remote bool) (string, error) {
	switch runtime {
	case "go":
		if remote {
			return "pack", nil
		}
		return "host", nil

	case "node", "typescript", "rust", "quarkus", "springboot":
		return "pack", nil

	case "python":
		if remote {
			return "s2i", nil
		}
		return "host", nil

	default:
		return "", fmt.Errorf("no builder support for runtime: %s", runtime)
	}
}

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

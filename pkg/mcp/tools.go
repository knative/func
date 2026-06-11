package mcp

// appendStringFlag adds a string flag to args only if the value is non-nil and non-empty.
// This ensures we only pass flags that were explicitly provided by the user.
func appendStringFlag(args []string, flag string, value *string) []string {
	if value != nil && *value != "" {
		return append(args, flag, *value)
	}
	return args
}

// appendBoolFlag adds a boolean flag to args only if the value is non-nil.
// When true, appends the flag (e.g. --push). When false, appends
// flag=false (e.g. --push=false) to allow explicit disabling.
func appendBoolFlag(args []string, flag string, value *bool) []string {
	if value != nil {
		if *value {
			return append(args, flag)
		}
		return append(args, flag+"=false")
	}
	return args
}

// ptr returns a pointer to the given value.
// Useful for setting optional annotation fields.
func ptr[T any](v T) *T {
	return &v
}

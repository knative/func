package scaffolding

type Signature int

const (
	UnknownSignature Signature = iota
	InstancedHTTP
	InstancedCloudevent
	StaticHTTP
	StaticCloudevent
)

func (s Signature) String() string {
	return []string{
		"unknown",
		"instanced-http",
		"instanced-cloudevent",
		"static-http",
		"static-cloudevent",
	}[s]
}

var signatureMap = map[bool]map[string]Signature{
	true: {
		"http":       InstancedHTTP,
		"cloudevent": InstancedCloudevent},
	false: {
		"http":       StaticHTTP,
		"cloudevent": StaticCloudevent},
}

// toSignature converts an instanced boolean and invocation hint into
// a Signature enum.
func toSignature(instanced bool, invoke string) Signature {
	if invoke == "" {
		invoke = "http"
	}
	s, ok := signatureMap[instanced][invoke]
	if !ok {
		return UnknownSignature
	}
	return s
}

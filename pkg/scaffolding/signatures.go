package scaffolding

type Signature int

const (
	UnknownSignature Signature = iota
	InstancedHTTP
	InstancedCloudevents
	StaticHTTP
	StaticCloudevents
)

func (s Signature) String() string {
	return []string{
		"unknown",
		"instanced-http",
		"instanced-cloudevents",
		"static-http",
		"static-cloudevents",
	}[s]
}

// Note that in all places other than the invocation hint (where singular
// is logically correct) "cloudevents" is plural in all places (variables,
// imports, enums etc) to match the Cloudevents library and organization's
// choice.
var signatureMap = map[bool]map[string]Signature{
	true: {
		"http":       InstancedHTTP,
		"cloudevent": InstancedCloudevents},
	false: {
		"http":       StaticHTTP,
		"cloudevent": StaticCloudevents},
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

package function

import (
	"github.com/google/uuid"
)

// InvokeMesage is the message used by the convenience method Invoke to provide
// a simple way to trigger the execution of a Function during development.
type InvokeMessage struct {
	ID     string
	Source string
	Type   string
	Data   string
}

// NewInvokeMessage creates a new InvokeMessage with fields populated
func NewInvokeMessage() InvokeMessage {
	return InvokeMessage{
		ID:     uuid.NewString(),
		Source: "/example/source",
		Type:   "example.fn",
		Data:   "exampleData",
	}
}

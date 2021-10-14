package invoker

import (
	"context"

	"github.com/cloudevents/sdk-go/v2/event"
)

type HTTPInvoker struct {
	Endpoint    string
	ContentType string
}

func NewHTTPInvoker() *HTTPInvoker {
	return &HTTPInvoker{
		ContentType: event.TextPlain,
	}
}

func (e *HTTPInvoker) Send(ctx context.Context, endpoint string) (err error) {
	return
}

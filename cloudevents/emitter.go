package cloudevents

import (
	"context"
	"fmt"
	nethttp "net/http"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/client"
	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/cloudevents/sdk-go/v2/protocol/http"
	"github.com/cloudevents/sdk-go/v2/types"
	"github.com/google/uuid"
)

const (
	DefaultSource = "/boson/fn"
	DefaultType   = "boson.fn"
)

type Emitter struct {
	Endpoint    string
	Source      string
	Type        string
	Id          string
	Data        string
	ContentType string
	Transport   nethttp.RoundTripper
}

func NewEmitter() *Emitter {
	return &Emitter{
		Source:      DefaultSource,
		Type:        DefaultType,
		Id:          uuid.NewString(),
		Data:        "",
		ContentType: event.TextPlain,
		Transport:   nethttp.DefaultTransport,
	}
}

func (e *Emitter) Emit(ctx context.Context, endpoint string) (err error) {
	p, err := http.New(http.WithTarget(endpoint), http.WithRoundTripper(e.Transport))
	if err != nil {
		return err
	}

	c, err := client.New(p)
	if err != nil {
		return err
	}

	evt := event.Event{
		Context: event.EventContextV1{
			Type:   e.Type,
			Source: *types.ParseURIRef(e.Source),
			ID:     e.Id,
		}.AsV1(),
	}
	if err = evt.SetData(e.ContentType, e.Data); err != nil {
		return
	}
	event, result := c.Request(ctx, evt)
	if !cloudevents.IsACK(result) {
		return fmt.Errorf(result.Error())
	}
	if event != nil {
		fmt.Printf("%v", event)
	}
	return nil
}

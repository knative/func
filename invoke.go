package function

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
)

const (
	DefaultInvokeSource      = "/boson/fn"
	DefaultInvokeType        = "boson.fn"
	DefaultInvokeContentType = "text/plain"
	DefaultInvokeData        = "example data"
)

// InvokeMesage is the message used by the convenience method Invoke to provide
// a simple way to trigger the execution of a Function during development.
type InvokeMessage struct {
	ID          string
	Source      string
	Type        string
	ContentType string
	Data        string
}

// NewInvokeMessage creates a new InvokeMessage with fields populated
func NewInvokeMessage() InvokeMessage {
	return InvokeMessage{
		ID:          uuid.NewString(),
		Source:      DefaultInvokeSource,
		Type:        DefaultInvokeType,
		ContentType: DefaultInvokeContentType,
		Data:        DefaultInvokeData,
	}
}

func invoke(ctx context.Context, c *Client, f Function, target string, m InvokeMessage) error {
	route, err := invocationRoute(ctx, c, f, target) // choose instance to invoke
	if err != nil {
		return err
	}

	switch f.Invocation.Format {
	case "cloudevent":
		return sendEvent(ctx, route, m) // invoke the f with m via CloudEvent
	default:
		return sendPost(ctx, route, m) // invoke the f with m via standard HTTP (Form POST)
		// NOTE: The default case ('http') is to always fall back to attempting
		// a simple HTTP POST with form values.
	}
}

// invocationRoute returns a route to the named target instance of a Func:
// 'local': locally running Function (error if not running)
// 'remote': first available remote instance (error if none available)
// '<url>': The exact URL passed as an override.
// '': Default if no target is passed is to first use local, then remote.
//     errors if neither are available.
func invocationRoute(ctx context.Context, c *Client, f Function, target string) (string, error) {
	info, err := c.Info(ctx, f.Name, f.Root)
	if err != nil {
		return "", err
	}
	if len(info.Routes) == 0 {
		return "", errors.New("no route to Function found")
	}
	return info.Routes[0], nil
}

// sendEvent to the route populated with data in the invoke message.
// with the data from the invoke message.
func sendEvent(ctx context.Context, route string, m InvokeMessage) (err error) {
	event := cloudevents.NewEvent()
	event.SetID(m.ID)
	event.SetSource(m.Source)
	event.SetType(m.Type)
	event.SetData(m.ContentType, m.Data)

	c, err := cloudevents.NewClientHTTP()
	if err != nil {
		return
	}

	result := c.Send(cloudevents.ContextWithTarget(ctx, route), event)
	if cloudevents.IsUndelivered(result) {
		err = fmt.Errorf("unable to invoke: %v", result)
	}
	return
}

// sendPost to the route populated with data in the invoke message.
func sendPost(ctx context.Context, route string, m InvokeMessage) error {
	resp, err := http.PostForm(route, url.Values{
		"ID":     {m.ID},
		"Source": {m.Source},
		"Type":   {m.Type},
		"Data":   {m.Data},
	})
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("failure invoking '%v' (HTTP %v)", route, resp.StatusCode)
	}
	return nil
}

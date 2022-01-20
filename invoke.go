package function

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
)

const (
	DefaultInvokeSource      = "/boson/fn"
	DefaultInvokeType        = "boson.fn"
	DefaultInvokeContentType = "text/plain"
	DefaultInvokeData        = "Hello World"
	DefaultInvokeFormat      = "http"
)

// InvokeMesage is the message used by the convenience method Invoke to provide
// a simple way to trigger the execution of a Function during development.
type InvokeMessage struct {
	ID          string
	Source      string
	Type        string
	ContentType string
	Data        string
	Format      string //optional override for Function-defined message format
}

// NewInvokeMessage creates a new InvokeMessage with fields populated
func NewInvokeMessage() InvokeMessage {
	return InvokeMessage{
		ID:          uuid.NewString(),
		Source:      DefaultInvokeSource,
		Type:        DefaultInvokeType,
		ContentType: DefaultInvokeContentType,
		Data:        DefaultInvokeData,
		// Format override not set by default: value from Function being preferred.
	}
}

// invoke the Function instance in the target environment with the
// invocation message.
func invoke(ctx context.Context, c *Client, f Function, target string, m InvokeMessage) error {

	// Get the first available route from 'local', 'remote', a named environment
	// or treat target
	route, err := invocationRoute(ctx, c, f, target) // choose instance to invoke
	if err != nil {
		return err
	}

	// Format" either 'http' or 'cloudevent'
	// TODO: discuss if providing a Format on Message should a) update the
	// Function to use the new format if none is defined already (backwards
	// compatibility fix) or b) always update the Function, even if it was already
	// set. Once decided, codify in a test.
	format := DefaultInvokeFormat
	if f.Invocation.Format != "" {
		// Prefer the format set during Function creation if defined.
		format = f.Invocation.Format
	}
	if m.Format != "" {
		// Use the override specified on the message if provided
		format = m.Format
	}

	switch format {
	case "http":
		return sendPost(ctx, route, m, c.transport)
	case "cloudevent":
		return sendEvent(ctx, route, m, c.transport)
	default:
		return fmt.Errorf("format '%v' not supported.", format)
	}
}

// invocationRoute returns a route to the named target instance of a Func:
// 'local': local environment; locally running Function (error if not running)
// 'remote': remote environment; first available instance (error if none)
// '<environment>': A valid alternate target which contains instances.
// '<url>': An explicit URL
// '': Default if no target is passed is to first use local, then remote.
//     errors if neither are available.
func invocationRoute(ctx context.Context, c *Client, f Function, target string) (string, error) {
	// TODO: this function has code-smell;  will de-smellify it in next pass.
	if target == EnvironmentLocal {
		instance, err := c.Instances().Get(ctx, f, target)
		if err != nil {
			if errors.Is(err, ErrEnvironmentNotFound) {
				return "", errors.New("not running locally")
			}
			return "", err
		}
		return instance.Route, nil

	} else if target == EnvironmentRemote {
		instance, err := c.Instances().Get(ctx, f, target)
		if err != nil {
			if errors.Is(err, ErrEnvironmentNotFound) {
				return "", errors.New("not running in remote")
			}
			return "", err
		}
		return instance.Route, nil

	} else if target == "" { // target blank, check local first then remote.
		instance, err := c.Instances().Get(ctx, f, EnvironmentLocal)
		if err != nil && !errors.Is(err, ErrNotRunning) {
			return "", err // unexpected errors are anything other than ErrNotRunning
		}
		if err == nil {
			return instance.Route, nil // found instance in local environment
		}
		instance, err = c.Instances().Get(ctx, f, EnvironmentRemote)
		if errors.Is(err, ErrNotRunning) {
			return "", errors.New("not running locally or in the remote")
		}
		if err != nil {
			return "", err // unexpected error
		}
		return instance.Route, nil
	} else { // treat an unrecognized target as an ad-hoc verbatim endpoint
		return target, nil
	}
}

// sendEvent to the route populated with data in the invoke message.
func sendEvent(ctx context.Context, route string, m InvokeMessage, t http.RoundTripper) (err error) {
	event := cloudevents.NewEvent()
	event.SetID(m.ID)
	event.SetSource(m.Source)
	event.SetType(m.Type)
	if err = event.SetData(m.ContentType, m.Data); err != nil {
		return
	}

	c, err := cloudevents.NewClientHTTP(
		cloudevents.WithTarget(route),
		cloudevents.WithRoundTripper(t))
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
func sendPost(ctx context.Context, route string, m InvokeMessage, t http.RoundTripper) error {
	client := http.Client{
		Transport: t,
		Timeout:   10 * time.Second,
	}
	resp, err := client.PostForm(route, url.Values{
		"ID":          {m.ID},
		"Source":      {m.Source},
		"Type":        {m.Type},
		"ContentType": {m.ContentType},
		"Data":        {m.Data},
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("failure invoking '%v' (HTTP %v)", route, resp.StatusCode)
	}
	return nil
}

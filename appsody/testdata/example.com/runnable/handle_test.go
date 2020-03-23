package function

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go"
)

// TestHandle ensures that Handle accepts a valid CloudEvent without error.
func TestHandle(t *testing.T) {
	// A minimal, but valid, event.
	event := cloudevents.NewEvent()
	event.SetID("TEST-EVENT-01")
	event.SetType("com.example.cloudevents.test")
	event.SetSource("http://localhost:8080/")

	// Invoke the defined handler.
	if err := Handle(context.Background(), event); err != nil {
		t.Fatal(err)
	}
}

// TestHandleInvalid ensures that an invalid input event generates an error.
func TestInvalidInput(t *testing.T) {
	invalidEvent := cloudevents.NewEvent() // missing required fields

	// Attempt to handle the invalid event, ensuring that the handler validats events.
	if err := Handle(context.Background(), invalidEvent); err == nil {
		t.Fatalf("handler did not generate error on invalid event.  Missing .Validate() check?")
	}
}

// TestE2E also tests the Handle function, but does so by creating an actual
// CloudEvents HTTP sending and receiving clients.  This is a bit redundant
// with TestHandle, but illustrates how clients are configured and used.
func TestE2E(t *testing.T) {
	var (
		receiver cloudevents.Client
		address  string             // at which the receiver beings listening (os-chosen port)
		sender   cloudevents.Client // sends an event to the receiver via HTTP
		handler  = Handle           // test the user-defined Handler
		err      error
	)

	if receiver, address, err = newReceiver(t); err != nil {
		t.Fatal(err)
	}

	if sender, err = newSender(t, address); err != nil {
		t.Fatal(err)
	}

	go func() {
		if err := receiver.StartReceiver(context.Background(), handler); err != nil {
			t.Fatal(err)
		}
	}()

	_, resp, err := sender.Send(context.Background(), newEvent(t, TestData{Sequence: 1, Message: "test message"}))
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("OK:\n%v\n", resp)
}

type TestData struct {
	Sequence int    `json:"id"`
	Message  string `json:"message"`
}

func newReceiver(t *testing.T) (c cloudevents.Client, address string, err error) {
	t.Helper()
	transport, err := cloudevents.NewHTTPTransport(
		cloudevents.WithPort(0), // use an OS-chosen unused port.
		cloudevents.WithPath("/"))
	if err != nil {
		return
	}
	address = fmt.Sprintf("http://127.0.0.1:%v/", transport.GetPort())
	c, err = cloudevents.NewClient(transport)
	return
}

func newSender(t *testing.T, address string) (c cloudevents.Client, err error) {
	t.Helper()
	transport, err := cloudevents.NewHTTPTransport(
		cloudevents.WithTarget(address),
		cloudevents.WithEncoding(cloudevents.HTTPStructuredV01))
	if err != nil {
		return
	}
	return cloudevents.NewClient(transport, cloudevents.WithTimeNow())
}

func newEvent(t *testing.T, data TestData) (event cloudevents.Event) {
	source, err := url.Parse("https://example.com/cloudfunction/cloudevent/cmd/runner")
	if err != nil {
		t.Fatal(err)
	}
	contentType := "application/json"
	event = cloudevents.Event{
		Context: cloudevents.EventContextV01{
			EventID:     "test-event-01",
			EventType:   "com.cloudevents.sample.sent",
			Source:      cloudevents.URLRef{URL: *source},
			ContentType: &contentType,
		}.AsV01(),
		Data: &data,
	}
	return
}

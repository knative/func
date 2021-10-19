package invoker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cloudevents/sdk-go/v2/client"
	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/cloudevents/sdk-go/v2/protocol/http"
	"github.com/google/go-cmp/cmp"
)

func makeClient(t *testing.T) (c client.Client, p *http.Protocol) {
	p, err := http.New()
	if err != nil {
		t.Fatal(err)
	}
	c, err = client.New(p)
	if err != nil {
		t.Errorf("failed to make client %s", err.Error())
	}
	return
}

func receiveEvents(t *testing.T, ctx context.Context, events chan<- event.Event) (p *http.Protocol) {
	c, p := makeClient(t)
	go func() {
		err := c.StartReceiver(ctx, func(ctx context.Context, event event.Event) error {
			go func() {
				events <- event
			}()
			return nil
		})
		if err != nil {
			t.Errorf("failed to start receiver %s", err.Error())
		}
	}()
	time.Sleep(1 * time.Second) // let the server start
	return
}

func TestEventInvokerDefaults(t *testing.T) {
	events := make(chan event.Event)
	ctx, cancel := context.WithCancel(context.Background())

	// start a cloudevent client that receives events
	// and sends them to a channel
	p := receiveEvents(t, ctx, events)

	emitter := NewEventInvoker()
	if err := emitter.Send(ctx, fmt.Sprintf("http://localhost:%v", p.GetListeningPort())); err != nil {
		t.Fatalf("Error emitting event: %v\n", err)
	}

	// received event
	got := <-events

	cancel()                    // stop the client
	time.Sleep(1 * time.Second) // let the server stop

	if got.Source() != "/boson/fn" {
		t.Fatal("Expected /boson/fn as default source")
	}
	if got.Type() != "boson.fn" {
		t.Fatal("Expected boson.fn as default type")
	}
}

func TestEventInvoker(t *testing.T) {
	testCases := map[string]struct {
		cesource string
		cetype   string
		ceid     string
		cedata   string
	}{
		"with-source": {
			cesource: "/my/source",
		},
		"with-type": {
			cetype: "my.type",
		},
		"with-id": {
			ceid: "11223344",
		},
		"with-data": {
			cedata: "Some event data",
		},
	}
	for n, tc := range testCases {
		t.Run(n, func(t *testing.T) {
			events := make(chan event.Event)
			ctx, cancel := context.WithCancel(context.Background())

			// start a cloudevent client that receives events
			// and sends them to a channel
			p := receiveEvents(t, ctx, events)

			emitter := NewEventInvoker()

			if tc.cesource != "" {
				emitter.Source = tc.cesource
			}
			if tc.cetype != "" {
				emitter.Type = tc.cetype
			}
			if tc.ceid != "" {
				emitter.Id = tc.ceid
			}
			if tc.cedata != "" {
				emitter.Data = tc.cedata
			}
			if err := emitter.Send(ctx, fmt.Sprintf("http://localhost:%v", p.GetListeningPort())); err != nil {
				t.Fatalf("Error emitting event: %v\n", err)
			}

			// received event
			got := <-events

			cancel()                           // stop the client
			time.Sleep(100 * time.Millisecond) // let the server stop

			if tc.cesource != "" && got.Source() != tc.cesource {
				t.Fatalf("%s: Expected %s as source, got %s", n, tc.cesource, got.Source())
			}
			if tc.cetype != "" && got.Type() != tc.cetype {
				t.Fatalf("%s: Expected %s as type, got %s", n, tc.cetype, got.Type())
			}
			if tc.ceid != "" && got.ID() != tc.ceid {
				t.Fatalf("%s: Expected %s as id, got %s", n, tc.ceid, got.ID())
			}
			if tc.cedata != "" {
				if diff := cmp.Diff(tc.cedata, string(got.Data())); diff != "" {
					t.Errorf("Unexpected difference (-want, +got): %v", diff)
				}
			}
		})
	}
}

//go:build e2e && linux

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"k8s.io/apimachinery/pkg/util/rand"
	"knative.dev/func/test/oncluster"
	"knative.dev/func/test/testhttp"

	common "knative.dev/func/test/common"
)

type FuncSubscribeTestType struct {
	T                    *testing.T
	TestBrokerName       string
	TestBrokerUrl        string
	FuncProducerUrl      string
	SubscribeToEventType string
}

func (f *FuncSubscribeTestType) newKnCli() *common.TestExecCmd {
	knCli := &common.TestExecCmd{
		Binary:              "kn",
		ShouldFailOnError:   true,
		ShouldDumpCmdLine:   true,
		ShouldDumpOnSuccess: true,
		T:                   f.T,
	}
	return knCli
}

func (f *FuncSubscribeTestType) newKubectlCli() *common.TestExecCmd {
	kubectl := &common.TestExecCmd{
		Binary:              "kubectl",
		ShouldDumpCmdLine:   true,
		ShouldDumpOnSuccess: false,
		T:                   f.T,
	}
	return kubectl
}

func (f *FuncSubscribeTestType) setupBroker() {
	f.TestBrokerName = "broker-" + rand.String(5)

	knCli := f.newKnCli()
	kubectl := f.newKubectlCli()
	knCli.Exec("broker", "create", f.TestBrokerName, "--class", "MTChannelBasedBroker")
	kubectl.Exec("wait", "broker/"+f.TestBrokerName, "--for=condition=TriggerChannelReady", "--timeout=15s")
	cmd := knCli.Exec("broker", "describe", f.TestBrokerName, "-o", "url")

	f.TestBrokerUrl = cmd.Out
	f.TestBrokerUrl = strings.TrimRight(f.TestBrokerUrl, "\n")
	assert.Assert(f.T, strings.HasPrefix(f.TestBrokerUrl, "http"))

	f.T.Cleanup(func() {
		f.newKnCli().Exec("broker", "delete", f.TestBrokerName)
	})
}

// setupProducerFunc creates and deploy a knative function that produces events
// It will take 'type' and 'message' from query string to build and send an event to a TARGET_SINK (env var)
// Example: https://func-producer.default.127.0.0.1.sslip.io?type=HelloEvent&message=HELLO+EVENT+1
func (f *FuncSubscribeTestType) setupProducerFunc() {

	var funcProducerName = "func-producer"
	var funcProducerPath = filepath.Join(f.T.TempDir(), funcProducerName)

	knFunc := common.NewKnFuncShellCli(f.T)
	knFunc.Exec("create", "--language", "node", "--template", "http", funcProducerPath)
	knFunc.SourceDir = funcProducerPath

	indexJsContent := `
const { httpTransport, emitterFor, CloudEvent } = require("cloudevents");
const handle = async (context, body) => {
  const ce = new CloudEvent({
    source: "test.source",
    type: context.query.type,
    data: { message: context.query.message }
  });
  const emit = emitterFor(httpTransport(process.env.TARGET_SINK));
  emit(ce);
}
module.exports = { handle };
`
	err := os.WriteFile(filepath.Join(funcProducerPath, "index.js"), []byte(indexJsContent), 0644)
	oncluster.AssertNoError(f.T, err)

	knFunc.Exec("config", "env", "add", "--name", "TARGET_SINK", "--value", f.TestBrokerUrl, "-p", funcProducerPath)
	knFunc.Exec("deploy", "-r", common.GetRegistry(), "-p", funcProducerPath)
	f.FuncProducerUrl = knFunc.Exec("describe", "-o", "url", "-p", funcProducerPath).Out
	f.FuncProducerUrl = strings.TrimRight(f.FuncProducerUrl, "\n")

	f.T.Cleanup(func() {
		knFunc.Exec("delete", funcProducerName)
	})
}

// setupConsumerFunc creates and deploy the function that subscribe to events of type HelloEvent
func (f *FuncSubscribeTestType) setupConsumerFunc() {
	var funcConsumerName = "func-consumer"
	var funcConsumerPath = filepath.Join(f.T.TempDir(), funcConsumerName)

	knFunc := common.NewKnFuncShellCli(f.T)
	knFunc.Exec("create", "--language", "node", "--template", "cloudevents", funcConsumerPath)
	knFunc.SourceDir = funcConsumerPath

	indexJsContent := `
const { CloudEvent } = require('cloudevents');
const handle = async (context, event) => {
  context.log.warn(event);
  console.log(event);
  return new CloudEvent({
    source: 'consumer.processor',
    type: 'consumer.processed'
  })
};
module.exports = { handle };
`
	err := os.WriteFile(filepath.Join(funcConsumerPath, "index.js"), []byte(indexJsContent), 0644)
	oncluster.AssertNoError(f.T, err)

	knFunc.Exec("subscribe", "--filter", "type="+f.SubscribeToEventType, "--source", f.TestBrokerName)
	knFunc.Exec("deploy", "-r", common.GetRegistry(), "-p", funcConsumerPath)

	f.T.Cleanup(func() {
		knFunc.Exec("delete", funcConsumerName)
	})
}

// TestFunctionSubscribeEvents tests the func integration with Kn Events by subscribing to events
// In other words, it tests `func subscribe` command
// To accomplish that the test steps consists in:
//   - Deploy a function that produces events and emits to the broker
//   - Deploy a function that subscribes to a specific Event Type (HelloEvent)
//   - Make the producer func to send events of the expected (HelloEvent) and unexpected (DiscardEvent) CE Type
//   - Assert the consumer function only receives the event it has subscribed to
func TestFunctionSubscribeEvents(t *testing.T) {

	funcSubTest := &FuncSubscribeTestType{T: t, SubscribeToEventType: "HelloEvent"}

	// ----------------------------------
	// 1. Setup test Broker
	// ----------------------------------
	funcSubTest.setupBroker()

	// ----------------------------------
	// 2. Deploy test functions
	// -----------------------------------
	deploymentChan := make(chan string)

	go func() {
		funcSubTest.setupProducerFunc() // "kn function" that emits test events
		deploymentChan <- "producer"
	}()
	go func() {
		funcSubTest.setupConsumerFunc() // "kn function" that subscribe to events
		deploymentChan <- "consumer"
	}()
	<-deploymentChan
	<-deploymentChan

	// ----------------------------------
	//  3. Test
	//    ON WHEN a new event of a specific type is received by the broker
	//    ASSERT THAT the func-consumer receives the event it has subscribed to
	// ----------------------------------

	// Watch the logs of func-consumer and inspects for received Events

	var gotEventA, gotEventB, c bool
	var podReached, podNotFound bool
	var doCheck = true

	waitChan := make(chan bool)
	go func() {
		kubectl := funcSubTest.newKubectlCli()
		for i := 0; doCheck; i++ {
			result := kubectl.Exec("logs", "-l", "function.knative.dev/name=func-consumer", "-c", "user-container")

			podNotFound = strings.Contains(result.Out, "No resources found")
			podReached = podReached || !podNotFound
			gotEventA = gotEventA || strings.Contains(result.Out, "EVENT_A_CATCH_ME")
			gotEventB = gotEventB || strings.Contains(result.Out, "EVENT_B_DISCARD_ME")
			doCheck = !(i > 20 || (podReached && podNotFound)) // check until function pod is Terminated
			if doCheck {
				if gotEventA && !c {
					c = true
					t.Log("Expected EVENT_A received. Watching for non-EVENT_B until function pod is Terminated")
				}
				kubectl.ShouldDumpCmdLine = false
				time.Sleep(6 * time.Second) // 1.5 minutes max wait.
			}
		}
		waitChan <- true
	}()
	time.Sleep(2 * time.Second)

	// Invoke Producer func to force Event A to be emitted. The event should be received by func
	testhttp.TestGet(t, funcSubTest.FuncProducerUrl+"?type="+funcSubTest.SubscribeToEventType+"&message=EVENT_A_CATCH_ME")

	// Invoke Producer func to force Event B to be emitted. The event should NOT be received by func
	testhttp.TestGet(t, funcSubTest.FuncProducerUrl+"?type=DiscardEvent&message=EVENT_B_DISCARD_ME")

	<-waitChan

	assert.Assert(t, gotEventA, "Event A was not received by the consumer function")
	assert.Assert(t, !gotEventB, "Event B was received but it should not be")

}

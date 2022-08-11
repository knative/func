# Golang Function Developer's Guide

When creating a Go (Golang) function using the `func` CLI, the project directory
looks like a typical Go project. Both HTTP and Event functions have the same
template structure.

```
❯ func create -l go fn
Project path: /home/developer/projects/fn
Function name: fn
Runtime: go

❯ tree
fn
├── README.md
├── func.yaml
├── go.mod
├── go.sum
├── handle.go
└── handle_test.go
```

Aside from the `func.yaml` file, this looks like the beginning of just about
any Go project. For now, we will ignore the `func.yaml` file, and just
say that it is a configuration file that is used when building your project.
If you're really interested, check out the [reference doc](func_yaml.md).
To learn more about the CLI and the details for each supported command, see
the [CLI Commands document](../reference/commands.txt).

## Running the function locally

To run a function, you'll first need to build it. This step creates an OCI
container image that can be run locally on your computer, or on a Kubernetes
cluster.

```
❯ func build
```

After the function has been built, it can be run locally.

```
❯ func run
```

Functions can be invoked with a simple HTTP request. 
You can test to see if the function is working by using your browser to visit
http://localhost:8080. You can also access liveness and readiness
endpoints at http://localhost:8080/health/liveness and
http://localhost:8080/health/readiness. These two endpoints are used
by Kubernetes to determine the health of your function. If everything
is good, both of these will return `{"ok":true}`.

## Deploying the function to a cluster

To deploy your function to a Kubernetes cluster, use the `deploy` command.

```
❯ func deploy
```

You can get the URL for your deployed function with the `info` command.

```
❯ func info
```

## Testing a function locally


Go functions can be tested locally on your computer. In the project there is
a `handle_test.go` file which contains simple test which can be extended as needed. 
Yo can run this test locally as you would do with any Go project.

```
❯ go test
```

## Function reference

Boson Go functions have very few restrictions. You can add any required dependencies
in `go.mod` and you may include additional local Go files. The only real requirement are 
that your project is defined in a `function` module and exports the function `Handle()` 
(supported contracts of this function will be discussed more deeply later).
In this section, we will look in a little more detail at how Boson functions are invoked,
and what APIs are available to you as a developer.

### Invocation parameters

When using the `func` CLI to create a function project, you may choose to generate a project
that responds to a `CloudEvent` or simple HTTP. `CloudEvents` in Knative are transported over
HTTP as a `POST` request, so in many ways, the two types of functions are very much the same.
They each will listen and respond to incoming HTTP events.

#### Function triggered by HTTP request

When an incoming request is received, your function will be invoked with a standard
Golang [Context](https://golang.org/pkg/context/) as the first parameter followed by
two parameters: Golang's [http.ResponseWriter](https://golang.org/pkg/net/http/#ResponseWriter)
and [http.Request](https://golang.org/pkg/net/http/#Request). 

Then you can use standard Golang techniques to access the request (eg. read the body)
and set a proper HTTP response of your function, as you can see on the following example:

```go
func Handle(ctx context.Context, res http.ResponseWriter, req *http.Request) {

  // Read body
  body, err := ioutil.ReadAll(req.Body)
  defer req.Body.Close()
  if err != nil {
	http.Error(res, err.Error(), 500)
	return
  }

  // Process body & function logic
  // ...
}
```

#### Function triggered by CloudEvent

If the incoming request is a `CloudEvent`, the event is provided via
[CloudEvents Golang SDK](https://cloudevents.github.io/sdk-go/) and its `Event` type
as a parameter. There's possibility to leverage Golang's
[Context](https://golang.org/pkg/context/) as the optional parameter in the function contract,
as you can see in the list of supported function signatures:

```go
Handle()
Handle() error
Handle(context.Context)
Handle(context.Context) error
Handle(cloudevents.Event)
Handle(cloudevents.Event) error
Handle(context.Context, cloudevents.Event)
Handle(context.Context, cloudevents.Event) error
Handle(cloudevents.Event) *cloudevents.Event
Handle(cloudevents.Event) (*cloudevents.Event, error)
Handle(context.Context, cloudevents.Event) *cloudevents.Event
Handle(context.Context, cloudevents.Event) (*cloudevents.Event, error)
```

For example, a `CloudEvent` is received which contains a JSON string such as this in its data property, 

```json
{ 
  "customerId": "0123456",
  "productId": "6543210"
}
```

to access this data, we need to define `Purchase` structure, which maps properties in `CloudEvents`
data and retrieve it from the incoming event:

```go
type Purchase struct {
  CustomerId string `json:"customerId"`
  ProductId  string `json:"productId"`
}

func Handle(ctx context.Context, event cloudevents.Event) err error {
	  
  purchase := &Purchase{}
  if err = cloudevents.DataAs(purchase); err != nil {
	fmt.Fprintf(os.Stderr, "failed to parse incoming CloudEvent %s\n", err)
	return
  }

  // ...
}
```

Or we can use Golang's `encoding/json` package to access the `CloudEvent` directly as
a JSON in form of bytes array:

```golang
func Handle(ctx context.Context, event cloudevents.Event) {

  bytes, err := json.Marshal(event)

  // ...
}
```

### Return Values
As mentioned above, HTTP triggered functions can set the response directly via
Golang's [http.ResponseWriter](https://golang.org/pkg/net/http/#ResponseWriter).

```go
func Handle(ctx context.Context, res http.ResponseWriter, req *http.Request) {

  // Set response
  res.Header().Add("Content-Type", "text/plain")
  res.Header().Add("Content-Length", "3")
  res.WriteHeader(200)

  _, err := fmt.Fprintf(res, "OK\n")
  if err != nil {
	fmt.Fprintf(os.Stderr, "error or response write: %v", err)
  }
}
```

Functions triggered by `CloudEvent` may return nothing, `error` and/or `CloudEvent` in order
to push events into the Knative eventing system. In this case, the developer is required
to set a unique `ID`, proper `Source` and a `Type` of the CloudEvent. The data can be populated
from a defined structure or from a `map`.

```go
func Handle(ctx context.Context, event cloudevents.Event) (resp *cloudevents.Event, err error) {

  // ...

  response := cloudevents.NewEvent()
  response.SetID("example-uuid-32943bac6fea")
  response.SetSource("purchase/getter")
  response.SetType("purchase")

  // Set the data from Purchase type
  response.SetData(cloudevents.ApplicationJSON, Purchase{
	CustomerId: custId,
	ProductId:  prodId,
  })

  // OR set the data directly from map
  response.SetData(cloudevents.ApplicationJSON, map[string]string{"customerId": custId, "productId": prodId})

  // Validate the response
  resp = &response
  if err = resp.Validate(); err != nil {
	fmt.Printf("invalid event created. %v", err)
  }

  return
}
```

## Dependencies
Developers are not restricted to the dependencies provided in the template
`go.mod` file. Additional dependencies can be added as they would be in any
other Golang project.

### Example
```console
go get gopkg.in/yaml.v2@v2.4.0
```

When the project is built for deployment, these dependencies will be included
in the resulting runtime container image.

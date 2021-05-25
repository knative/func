# Node.js Function Developer's Guide

When creating a Node.js function using the `func` CLI, the project directory
looks like a typical Node.js project. Both HTTP and Event functions have the same
template structure.

```
❯ func create fn
Project path: /home/developer/projects/fn
Function name: fn
Runtime: node
Trigger: http

❯ tree fn
fn
├── func.yaml
├── index.js
├── package.json
├── README.md
└── test
    ├── integration.js
    └── unit.js
```

Aside from the `func.yaml` file, this looks like the beginning of just about
any Node.js project. For now, we will ignore the `func.yaml` file, and just
say that it is a configuration file that is used when building your project.
If you're really interested, check out the [reference doc](config-reference.doc).
To learn more about the CLI and the details for each supported command, see
the [CLI Commands document](commands.md#cli-commands).

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
is good, both of these will return `OK`.

## Deploying the function to a cluster

To deploy your function to a Kubenetes cluster, use the `deploy` command.

```
❯ func deploy
```

You can get the URL for your deployed function with the `describe` command.

```
❯ func describe
```

## Testing a function locally


Node.js functions can be tested locally on your computer. In the project there is
a `test` folder which contains some simple unit and integration tests. To run these
locally, you'll need to install the required dependencies. You do this as you would
with any Node.js project.

```
❯ npm install
```

Once you have done this, you can run the provided tests with `npm test`. The default
test framework for Node.js functions is [`tape`](https://www.npmjs.com/package/tape).
If you prefer another, that's no problem. Just remove the `tape` dependency from
`package.json` and install a test framework more to your liking.

## Function reference

Boson Node.js functions have very few restrictions. You can add any
required dependencies in `package.json`, and you may include
additional local JavaScript files. The only real requirements are that
your project contain an `index.js` file which exports a single
function.  In this section, we will look in a little more detail at
how Boson functions are invoked, and what APIs are available to you as
a developer.

### Invocation parameters

When using the `func` CLI to create a function project, you may choose
to generate a project that responds to a `CloudEvent` or simple
HTTP. `CloudEvents` in Knative are transported over HTTP as a `POST`
request, so in many ways, the two types of functions are very much the
same.  They each will listen and respond to incoming HTTP events.

When an incoming request is received, your function will be invoked
with a `Context` object as the first parameter. If the incoming
request is a `CloudEvent`, it will be provided as the second
parameter. For example, a `CloudEvent` is received which contains a
JSON string such as this in its data property,

```json
{ 
  "customerId": "0123456",
  "productId": "6543210"
}
```

So to log that string from your function, you might do this:

```js
function handle(context, event) {
  console.log(JSON.stringify(event.data, null, 2));
}
```
### Return Values
Functions may return any valid JavaScript type, or nothing at all. When a
function returns nothing, and no failure is indicated, the caller will receive
a `204 No Content"` response. Functions may also return a `CloudEvent`, or a
`Message` object in order to push events into the Knative eventing system. In
this case, the developer is not required to understand or implement the
CloudEvent messaging specification. Headers and other relevant information from
the returned values are extracted and sent with the response.

#### Example
```js
function handle(context, event) {
  return processCustomer(event.data)
}
function processCustomer(customer) {
  // process customer and return a new CloudEvent
  return new CloudEvent({
    source: 'customer.processor',
    type: 'customer.processed'
  })
}
```

### Response headers
Functions may additionally set headers to be sent with the response by adding a
`headers` property to the object being returned. These headers will be
extracted and sent with the response to the caller.

#### Example
```js
function processCustomer(customer) {
  // process customer and return custom headers
  // the response will be '204 No content'
  return { headers: { customerid: customer.id } }; 
}
```

### Response codes
Developers may set the response code returned to the caller by adding a
`statusCode` property to the response.

#### Example
```js
function processCustomer(customer) {
  // process customer
  if (customer.restricted) {
    return { statusCode: 451 }
  } 
}
```

This also works with `Error` objects thrown from the function.

#### Example
```js
function processCustomer(customer) {
  // process customer
  if (customer.restricted) {
    const err = new Error(‘Unavailable for legal reasons’);
    err.statusCode = 451;
    throw err;
  } 
}
```

## The Context Object
Functions are invoked with a context object as the first parameter. This object
provides access to the incoming request information. Developers can get the
HTTP request method, any query strings sent with the request, the headers, the
HTTP version, the request body. If the incoming request is a `CloudEvent`, the
`CloudEvent` itself will also be found on the context object.

The `Context` object has several properties that may be accessed by the
function developer.

### `log`
Provides a logging object that can be used to write output to the cluster logs.
The log adheres to the Pino logging API (https://getpino.io/#/docs/api).

#### Example
```js
Function handle(context) {
  context.log.info(“Processing customer”);
}
```

Access the function via `curl` to invoke it.

```sh
curl http://example.com
```

The function will log 

```console
{"level":30,"time":1604511655265,"pid":3430203,"hostname":"localhost.localdomain","reqId":1,"msg":"Processing customer"}
```

### `query`
Returns the query string for the request, if any, as key value pairs. These
attributes are also found on the context object itself.

#### Example
```js
Function handle(context) {
  // Log the 'name' query parameter
  context.log.info(context.query.name);
  // Query parameters also are attached to the context
  context.log.info(context.name);
}
```

Access the function via `curl` to invoke it.

```sh
curl http://example.com?name=tiger
```
The function will log 

```console
{"level":30,"time":1604511655265,"pid":3430203,"hostname":"localhost.localdomain","reqId":1,"msg":"tiger"}
{"level":30,"time":1604511655265,"pid":3430203,"hostname":"localhost.localdomain","reqId":1,"msg":"tiger"}
```

### `body`
Returns the request body if any. If the request body contains JSON, this will
be parsed so that the attributes are directly available.

#### Example
```js
Function handle(context) {
  // log the incoming request body's 'hello' parameter
  context.log.info(context.body.hello);
}
```

Access the function via `curl` to invoke it.

```console
curl -X POST -d '{"hello": "world"}'  -H'Content-type: application/json' http://example.com
```

The function will log 
```console
{"level":30,"time":1604511655265,"pid":3430203,"hostname":"localhost.localdomain","reqId":1,"msg":"world"}
```

### `headers`
Returns the HTTP request headers as an object.

#### Example
```js
Function handle(context) {
  context.log.info(context.headers[custom-header]);
}
```
Access the function via `curl` to invoke it.

```console
curl -H'x-custom-header: some-value’' http://example.com
```
The function will log 
```console
{"level":30,"time":1604511655265,"pid":3430203,"hostname":"localhost.localdomain","reqId":1,"msg":"some-value"}
```

### `method`

Returns the HTTP request method as a string.


### `httpVersion`
Returns the HTTP version as a string.

### `httpVersionMajor`

Returns the HTTP major version number as a string.

### `httpVersionMinor`
Returns the HTTP minor version number as a string.

### `httpVersionMinor`
Returns the HTTP minor version number as a string.

## Context Methods
There is a single method on the `Context` object which is a convenience
function for returning a `CloudEvent` object. In Knative systems, if a function
service is invoked by an event broker with a `CloudEvent`, the broker will
examine the response. If the response is a `CloudEvent`, this event will then
be handled by the broker just as with any other event it receives.

### cloudEventResponse()
A function which accepts a data value and returns a CloudEvent.

#### Example
```js
// Expects to receive a CloudEvent with customer data
function handle(context, event) {
  // process the customer
  const processed = processCustomer(event.data);
  return context.cloudEventResponse(processed);
}
```

## Dependencies
Developers are not restricted to the dependencies provided in the template
`package.json` file. Additional dependencies can be added as they would be in any
other Node.js project.

### Example
```console
npm install --save opossum
```

When the project is built for deployment, these dependencies will be included
in the resulting runtime container image.

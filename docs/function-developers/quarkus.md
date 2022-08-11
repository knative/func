# Quarkus Function Developer's Guide

When creating a Quarkus function using the `func` CLI, the project directory
looks like a typical `maven` project. Both HTTP and Event functions have the same
template structure.

```
❯ func create -l quarkus fn
Project path: /home/developer/projects/fn
Function name: fn
Runtime: quarkus

❯ tree         
fn
├── func.yaml
├── mvnw
├── mvnw.cmd
├── pom.xml
├── README.md
└── src
    ├── main
    │   ├── java
    │   │   └── functions
    │   │       ├── Function.java
    │   │       ├── Input.java
    │   │       └── Output.java
    │   └── resources
    │       └── application.properties
    └── test
        └── java
            └── functions
                ├── FunctionTest.java
                └── NativeFunctionIT.java
```

Aside from the `func.yaml` file, this looks like the beginning of just about
any Java Maven project. For now, we will ignore the `func.yaml` file, and just
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
You might be asked about docker registry during the build step.
After the function has been built, it can be run locally.

```
❯ func run
```

Functions can be invoked with a simple HTTP request. 
You can test to see if the function is working by using your browser to visit
http://localhost:8080.

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


Quarkus functions can be tested locally on your computer.
The project contains tests which you can run.
You do this as you would with any Maven project.

```
❯ mvn test
```

## Function reference

Boson Quarkus functions have very few restrictions. You can work with it as with any Java Maven project.
The only real requirements is that your project contain a method annotated with `@Funq`.
In this section, we will look in a little more detail at how Boson functions are invoked,
and what APIs are available to you as a developer.

### Invocation parameters

When using the `func` CLI to create a function project, you may choose to generate a project
that responds to a `CloudEvent` or simple HTTP. `CloudEvents` in Knative are transported over
HTTP as a `POST` request, so in many ways, the two types of functions are very much the same.
They each will listen and respond to incoming HTTP events.

When an incoming request is received, your function will be invoked with an instance of type of your choice, see [Types](#types).

The instance will contain one of:
* `data` of `CloudEvent`
* Body of HTTP POST request
* Query parameters of HTTP GET request

For example, imagine a function that is meant to receive and process purchase data.
Your function signature may look like this:
```java
public class Functions {
    @Funq
    public void processPurchase(Purchase purchase) {
        // do stuff
    }
}
```
With a `Purchase` bean that looks like this:
```java
public class Purchase {
    private long customerId;
    private long productId;
    // getters and setter here
}
```
Expecting data that may be represented in JSON like this:
```json
{
  "customerId": "0123456",
  "productId": "6543210"
}
```
Depending on the deployment, this function will be invoked in one of three ways:
* An incoming `CloudEvent` with a JSON object such as above in its `data` property.
* An ordinary HTTP POST with a JSON object such as above in the body of the request
* An ordinary HTTP GET with a query string like `?customerId=0123456&productId=6543210`

### Return Values
Functions may return an instance of type satisfying condition described in [Types](#types).

You can also return `Uni<T>`.
Note that the type parameter of `Uni<T>` must satisfy conditions described in [Types](#types).
This is useful when the function calls asynchronous APIs (e.g. Vert.x HTTP client).

The object you return will be serialized in the same format as the object you received.

If the function received `CloudEvent` in binary encoding,
then the object you return will be sent in the `data` property of a binary encoded `CloudEvent`.

If the function received vanilla HTTP,
then the object you return will be sent as the HTTP response body. In the example below, an invocation of this function
through an incoming `CloudEvent`, will receive a response with a `CloudEvent` containing a list of purchases as its 
`data` property. If the invocation was via an ordinary HTTP request, the response will contain the same list of purchases
in the HTTP response body, but no `CloudEvent` headers will be included.

#### Example
```java
public class Functions {
    @Funq
    public List<Purchase> getPurchasesByName(String name);
}
```

### Types

The input and output types of a function can be:
* `void`
* `String`,
* `byte[]`
* Primitive types and their wrappers (e.g. `int`, `Double`),
* JavaBeans (the attributes of the bean must also follow the rules defined here)
* Map, List or Array of the above

In addition to above, there is one special type: `CloudEvents<T>` described below [CloudEvent attributes](#cloudevent-attributes).

#### Example
```java
public class Functions {
    public List<Integer> getIds();
    public Purchase[] getPurchasesByName(String name);
    public String getNameById(int id);
    public Map<String,Integer> getNameIdMapping();
    public void processImage(byte[] img);
}
```

### CloudEvent attributes

Above we described cases where we handle just the `data` portion of a `CloudEvent`.
In many cases this is all we need.
However sometimes you may need to also read or write the attributes of a `CloudEvent`, such as `type` or `subject`.

For this purpose `Funqy` offers the generic interface `CloudEvent<T>` and a builder, `CloudEventBuilder`.
Note that the type parameter of `CloudEvent<T>` must satisfy the conditions described in [Types](#types).

#### Example
```java
public class Functions {
    
    private boolean _processPurchase(Purchase purchase) {
        // do stuff
    }
    
    public CloudEvent<Void> processPurchase(CloudEvent<Purchase> purchaseEvent) {
        System.out.println("subject is: ", purchaseEvent.subject());
        
        if (!_processPurchase(purchaseEvent.data())) {
            return CloudEventBuilder.create()
                    .type("purchase.error")
                    .build();
        }
        return CloudEventBuilder.create()
                .type("purchase.success")
                .build();
    }
}
```

### Invocation examples

Sample code:

```java
import io.quarkus.funqy.Funq;
import io.quarkus.funqy.knative.events.CloudEvent;

public class Input {
    private String message;

    // getters and setters...
}

public class Output {
    private String message;

    // getters and setters...
}

public class Functions {
    @Funq
    public Output withBeans(Input in) {
        // code here
    }

    @Funq
    public CloudEvent<Output> withCloudEvent(CloudEvent<Input> in) {
        // code here
    }

    @Funq
    void withBinary(byte[] in) {
        // code here
    }
}

```

The `Functions::withBeans` function above can be invoked by:

* Simple HTTP POST with JSON body
```shell
curl "http://localhost:8080/" -X POST \
  -H "Content-Type: application/json" \
  -d '{"message": "Hello there."}'
```

* Simple HTTP GET with query parameters
```shell
curl "http://localhost:8080?message=Hello%20there." -X GET
```

* CloudEvent in binary encoding:
```shell
curl "http://localhost:8080/" -X POST \
  -H "Content-Type: application/json" \
  -H "Ce-SpecVersion: 1.0" \
  -H "Ce-Type: my-type" \
  -H "Ce-Source: cURL" \
  -H "Ce-Id: 42" \
  -d '{"message": "Hello there."}'
```

* CloudEvent in structured (JSON) encoding:
```shell
curl http://localhost:8080/ \
  -H "Content-Type: application/cloudevents+json" \
  -d '{ "data": {"message":"Hello there."},
        "datacontenttype": "application/json",
        "id": "42",
        "source": "curl",
        "type": "my-type",
        "specversion": "1.0"}'
```

The `Functions::withCloudEvent` function can be invoked similarly to `Functions::withBeans`,
however only the last two examples are correct, the first two (raw HTTP) would result in error.

The `Functions::withBinary` function can be invoked by:

* CloudEvent in binary encoding:
```shell
curl "http://localhost:8080/" -X POST \
  -H "Content-Type: application/octet-stream" \
  -H "Ce-SpecVersion: 1.0"\
  -H "Ce-Type: my-type" \
  -H "Ce-Source: cURL" \
  -H "Ce-Id: 42" \
  --data-binary '@img.jpg'

```

* CloudEvent in structured (JSON) encoding:
```shell
curl http://localhost:8080/ \
  -H "Content-Type: application/cloudevents+json" \
  -d "{ \"data_base64\": \"$(base64 img.jpg)\",
        \"datacontenttype\": \"application/octet-stream\",
        \"id\": \"42\",
        \"source\": \"curl\",
        \"type\": \"my-type\",
        \"specversion\": \"1.0\"}"
```

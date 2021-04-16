# Quarkus Developer's Guide

When creating a Quarkus function using the `func` CLI, the project directory
looks like a typical `maven` project. Both HTTP and Event functions have the same
template structure.

```
❯ func create my-fn --runtime quarkus 
Project path: /home/mvasek/devel/my-fn
Function name: my-fn
Runtime: quarkus
Trigger: http

❯ tree my-fn         
my-fn
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

8 directories, 11 files
```

Aside from the `func.yaml` file, this looks like the beginning of just about
any Java maven project. For now, we will ignore the `func.yaml` file, and just
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
You might be asked about docker registry during the build step.
After the function has been built, it can be run locally.

```
❯ func run
```

Functions can be invoked with a simple HTTP request. 
You can test to see if the function is working by using your browser to visit
http://localhost:8080.

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


Quarkus functions can be tested locally on your computer.
The project contains tests which you can run.
You do this as you would with any maven project.

```
❯ mvn test
```

## Function reference

Boson Quarkus functions have very few restrictions. You can work with it as with any Java maven project.
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

For example, if `CloudEvent` contains a JSON object such as this in its data property
(or if an ordinary HTTP POST that contains such an object in body),

```json
{
  "customerId": "0123456",
  "productId": "6543210"
}
```

you might want to use JavaBean similar to this as an input parameter:

```java
public class Purchase {
    private long customerId;
    private long productId;
    
    // getters and setter here
}
```

and function signature could look like this:

```java
public class Functions {
    @Funq
    public void processPurchase(Purchase purchase) {
        // do stuff
    }
}
```

### Return Values
Functions may return an instance of type satisfying condition described in [Types](#types).


You can also return `Uni<T>`.
Note that the type parameter of `Uni<T>` must satisfy conditions described in [Types](#types).
This is useful when the function calls asynchronous APIs (e.g. vertx http client).

The object you return will be serialized in the same format as the object you received.

If the function received `CloudEvent` in binary encoding,
then the object you return will be sent as `data` of binary encoded `CloudEvent`.

If the function received vanilla HTTP,
then the object you return will be sent as body in HTTP response.

#### Example
```java
public class Functions {
    @Funq
    public List<Purchase> getPurchasesByName(String name);
}
```

### Types

Input or output type of function can be:
* `void`
* `String`,
* `byte[]`
* primitive types and their wrappers (e.g. `int`, `Double`),
* JavaBeans (the attributes of the JavaBeans also must follow rules defined here)
* Map, List or Array of the above

There is one special type: `CloudEvents<T>` described below [CloudEvent attributes](#cloudevent-attributes).

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

Above we described cases where we handle just `data` portion of `CloudEvent`.
In many cases this is all we need.
However sometimes you may need to also read or write `CloudEvent`'s attributes, such as `type` or `subject`.

For this purpose `Funqy` offers generic interface `CloudEvent<T>` and builder `CloudEventBuilder`.
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

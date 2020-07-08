# Quarkus Cloud Events Function

Welcome to your new Quarkus function project! This sample project contains three functions:
* org.funqy.demo.GreetingFunctions.greet()
* org.funqy.demo.PrimitiveFunctions.toLowerCase()
* org.funqy.demo.PrimitiveFunctions.doubleIt()

Only one of the function is active at the time.
Which one is determined by the `quarkus.funqy.export` property in `src/main/resources/application.properties`.

The `greet` function demonstrates work with java beans,
it accepts `Identity` bean (data of incoming cloud event)
and it returns `Greeting` bean (data of outgoing cloud event). 

The `toLowerCase` function accepts string and returns string.
As its name suggests it returns input string with all characters in lowercase.

The `doubleIt` function accepts integer and returns its value doubled.

You can test those functions by using `curl` cmd utility.
Parameters for `curl` can be found at [Cloud Event emulation](#cloud-event-emulation).

## Local execution
Make sure that `maven 3.6.2` and `Java 11 SDK` is installed.

To start server locally run `mvn quarkus:dev`.
The command starts http server and automatically watches for changes of source code.
If source code changes the change will be propagated to running server. It also opens debugging port `5005`
so debugger can be attached if needed.

To run test locally run `mvn test`.

## Cloud Event emulation

sample cloud event for the `greet` function
```shell script
curl -v "localhost:8080" \
  -X POST \
  -H "Ce-Id: 536808d3-88be-4077-9d7a-a3f162705f79" \
  -H "Ce-specversion: 0.3" \
  -H "Ce-Type: dev.nodeshift.samples.quarkus-funqy" \
  -H "Ce-Source: dev.nodeshift.samples/quarkus-funqy-source" \
  -H "Content-Type: application/json" \
  -d "{\"name\": \"$(whoami)\"}\"";

```

sample event for the `toLowerCase` function
```shell script
curl -v "localhost:8080" \
  -X POST \
  -H "Ce-Id: 536808d3-88be-4077-9d7a-a3f162705f79" \
  -H "Ce-specversion: 0.3" \
  -H "Ce-Type: dev.nodeshift.samples.quarkus-funqy" \
  -H "Ce-Source: dev.nodeshift.samples/quarkus-funqy-source" \
  -H "Content-Type: application/json" \
  -d '"wEiRDly CaPiTaLisEd sTrInG"'
```

sample event for the `doubleIt` function
```shell script
curl -v "localhost:8080" \
  -X POST \
  -H "Ce-Id: 536808d3-88be-4077-9d7a-a3f162705f79" \
  -H "Ce-specversion: 0.3" \
  -H "Ce-Type: dev.nodeshift.samples.quarkus-funqy" \
  -H "Ce-Source: dev.nodeshift.samples/quarkus-funqy-source" \
  -H "Content-Type: application/json" \
  -d '21'
```

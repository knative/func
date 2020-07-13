# Quarkus Http Function

Welcome to your new Quarkus function project! This sample project contains three functions:
* org.funqy.demo.GreetingFunctions.greet()
* org.funqy.demo.PrimitiveFunctions.toLowerCase()
* org.funqy.demo.PrimitiveFunctions.doubleIt()

The `greet` function demonstrates work with java beans,
it accepts `Identity` bean (data of incoming http request),
and it returns `Greeting` bean (data of http response). 

The `toLowerCase` function accepts string and returns string.
As its name suggests it returns input string with all characters in lowercase.

The `doubleIt` function accepts integer and returns its value doubled.

You can test those functions by using `curl` cmd utility.
Parameters for `curl` can be found at [Request emulation](#request-emulation).

## Local execution
Make sure that `maven 3.6.2` and `Java 11 SDK` is installed.

To start server locally run `mvn quarkus:dev`.
The command starts http server and automatically watches for changes of source code.
If source code changes the change will be propagated to running server. It also opens debugging port `5005`
so debugger can be attached if needed.

To run test locally run `mvn test`.

## Request emulation

sample http request for the `greet` function
```shell script
curl -v "localhost:8080/greet" \
  -X POST \
  -H "Content-Type: application/json" \
  -d "{\"name\": \"$(whoami)\"}\"";

```

sample http request for the `toLowerCase` function
```shell script
curl -v "localhost:8080/toLowerCase" \
  -X POST \
  -H "Content-Type: application/json" \
  -d '"wEiRDly CaPiTaLisEd sTrInG"'
```

sample http request for the `doubleIt` function
```shell script
curl -v "localhost:8080/doubleIt" \
  -X POST \
  -H "Content-Type: application/json" \
  -d '21'
```

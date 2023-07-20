# Spring Boot Function Developer's Guide

When creating a Spring Boot function using the `func` CLI, the project
directory looks like a typical Boot project.
To create a HTTP function that echos the input run the following command:

```
❯ func create -l springboot fn
```

This is the HTTP function template structure:

```
Project path: /home/developer/projects/fn
Function name: fn
Language Runtime: springboot
Template: http

❯ tree
fn
├── func.yaml
├── mvnw
├── mvnw.cmd
├── pom.xml
├── README.md
└── src
    ├── main
    │   ├── java
    │   │   └── functions
    │   │       └── CloudFunctionApplication.java
    │   └── resources
    │       └── application.properties
    └── test
        └── java
            └── functions
                └── EchoCaseFunctionTest.java
```

This is a full-featured, self-contained Spring Boot application that uses [Spring Cloud Function](https://spring.io/projects/spring-cloud-function) and [Spring WebFlux](https://docs.spring.io/spring-framework/docs/current/reference/html/web-reactive.html) web framework to listen for HTTP requests on port 8080.

See the generated [README.md](../../templates/springboot/http/README.md) for
details on building, testing, and deploying the HTTP app.

You may have noticed the `func.yaml` file. This is a configuration
file used by `func` to deploy your project as a service in your
kubernetes cluster.

For an event-triggered function, pass the `-t cloudevents` option to
generate an app capable of responding to
[CloudEvents](https://cloudevents.io):

```
❯ func create -l springboot -t cloudevents fn
```

A CloudEvent function have a similar template structure to the above HTTP function:

```
Project path: /home/developer/projects/fn
Function name: fn
Language Runtime: springboot
Template: cloudevents

❯ tree
fn
├── func.yaml
├── mvnw
├── mvnw.cmd
├── pom.xml
├── README.md
└── src
    ├── main
    │   ├── java
    │   │   └── echo
    │   │       ├── EchoFunction.java
    │   │       └── SpringCloudEventsApplication.java
    │   └── resources
    │       └── application.properties
    └── test
        └── java
            └── echo
                └── SpringCloudEventsApplicationTests.java
```

The handlers of each app differ slightly. See its generated
[README.md](../../templates/springboot/cloudevents/README.md) for more details.

Have fun!

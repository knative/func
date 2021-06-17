# Rust Function Developer's Guide

When creating a Rust function using the `func` CLI, the project
directory looks like a typical Rust project. Both HTTP and Event
functions have the same template structure:

```
❯ func create -l rust fn
Project path: /home/jim/src/func/fn
Function name: fn
Runtime: rust

❯ tree
fn
├── Cargo.lock
├── Cargo.toml
├── func.yaml
├── README.md
└── src
    ├── handler.rs
    └── main.rs

```

This is a full-featured, self-contained Rust application that uses the
[actix](https://actix.rs/) web framework to listen for HTTP requests on port 8080. 

See the generated [README.md](../../templates/rust/http/README.md) for
details on building, testing, and deploying the app.

You may have noticed the `func.yaml` file. This is a configuration
file used by `func` to deploy your project as a service in your
kubernetes cluster. 

For an event-triggered function, pass the `-t events` option to
generate an app capable of responding to
[CloudEvents](https://cloudevents.io):

```
❯ func create -l rust -t events fn
```

The handlers of each app differ slightly. See its generated
[README.md](../../templates/rust/events/README.md) for more details.

Have fun!

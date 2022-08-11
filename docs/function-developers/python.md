# Python Function Developer's Guide

When creating a Python function using the `func` CLI, the project directory
looks like a typical Python project. Both HTTP and Event functions have the same
template structure.

```
❯ func create -l python fn
Project path: /home/developer/src/fn
Function name: fn
Runtime: python

❯ tree
fn
├── func.py
├── func.yaml
├── requirements.txt
└── test_func.py

```

Aside from the `func.yaml` file, this looks like the beginning of just about
any Python project. For now, we will ignore the `func.yaml` file, and just
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
is good, both of these will return `OK`.

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


Python functions can be tested locally on your computer. In the project there is
a `test_func.py` file which contains a simple unit test. To run the test locally,
you'll need to install the required dependencies. You do this as you would
with any Python project.

```
❯ pip install -r requirements.txt
```

Once you have done this, you can run the provided tests with `python3 test_func.py`.
The default test framework for Python functions is `unittest`. If you prefer another,
that's no problem. Just install a test framework more to your liking.

## Function reference

Boson Python functions have very few restrictions. You can add any required dependencies
in `requirements.txt`, and you may include additional local Python files. The only real
requirements are that your project contain a `func.py` file which contains a `main()` function.
In this section, we will look in a little more detail at how Boson functions are invoked,
and what APIs are available to you as a developer.

### Invocation parameters

When using the `func` CLI to create a function project, you may choose to generate a project
that responds to a `CloudEvent` or simple HTTP. `CloudEvents` in Knative are transported over
HTTP as a `POST` request, so in many ways, the two types of functions are very much the same.
They each will listen and respond to incoming HTTP events.

When an incoming request is received, your function will be invoked with a `Context`
object as the first parameter. This object is a Python class with two attributes. The
`request` attribute will always be present, and contains the Flask `request` object.
The second attribute, `cloud_event`, will be populated if the incoming request is a
`CloudEvent`. Developers may access any `CloudEvent` data from the context object.
For example:

```python
def main(context: Context):
    """ 
    The context parameter contains the Flask request object and any
    CloudEvent received with the request.
    """
    print(f"Method: {context.request.method}")
    print(f"Event data {context.cloud_event.data})
    # ... business logic here
```

### Return Values
Functions may return any value supported by Flask, as the invocation framework
proxies these values directly to the Flask server. See the Flask 
[documentation](https://flask.palletsprojects.com/en/1.1.x/quickstart/#about-responses)
for more information.

#### Example
```python
def main(context: Context):
    data = { "message": "Howdy!" }
    headers = { "content-type": "application/json" }
    return body, 200, headers
```

Note that functions may set both headers and response codes as secondary
and tertiary response values from function invocation.

### CloudEvents
All event messages in Knative are sent as `CloudEvents` over HTTP. As noted
above, function developers may access an event through the `context` parameter
when the function is invoked. Additionally, developers may use an `@event`
decorator to inform the invoker that this function's return value should be
converted to a `CloudEvent` before sending the response. For example:

```python
@event("event_source"="/my/function", "event_type"="my.type")
def main(context):
    # business logic here
    data = do_something()
    # more data processing
    return data
```

This will result in a `CloudEvent` as the response value, with a type of
`"my.type"`, a source of `"/my/function"`, and the data property set to `data`.
Both the `event_source` and `event_type` decorator attributes are optional.
If not supplied, the CloudEvent's source attribute will be set to
`"/parliament/function"` and the type will be set to `"parliament.response"`.

## Dependencies
Developers are not restricted to the dependencies provided in the template
`requirements.txt` file. Additional dependencies can be added as they would be
in any other project by simply adding them to the `requirements.txt` file.
When the project is built for deployment, these dependencies will be included
in the container image.

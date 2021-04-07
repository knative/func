# Python Developer's Guide

When creating a Python function using the `func` CLI, the project directory
looks like a typical Python project. Both HTTP and Event functions have the same
template structure.

```
~/src/pfunk
❯ kn func create -l python
Project path: /home/lanceball/src/pfunk
Function name: pfunk
Runtime: python
Trigger: http

~/src/pfunk via 🐍 v3.8.5
❯ tree
.
├── func.py
├── func.yaml
├── requirements.txt
└── test_func.py

```

Aside from the `func.yaml` file, this looks like the beginning of just about
any Python project. For now, we will ignore the `func.yaml` file, and just
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

When an inacoming request is received, your function will be invoked with a `Context` object as the first parameter. This object is simply a Python `dict` with two keys, `request` and
optionally, `cloud_event`, if the incoming request is a `CloudEvent`. The `request` value
is the Flask request objext. Developers may access any event data from the context objext.
For example: 

```python
def main(context: Context):
  """ 
  The context parameter contains the Flask request object and any
  CloudEvent received with the request.
  """
  print(f"Method: {context['request'].method}")
  print(f"Event data {context['cloud_event'].data})
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
  attributes = {
    "type": "com.example.fn",
    "source": "https://example.com/fn"
  }
  data = { "message": "Howdy!" }
  event = CloudEvent(attributes, data)
  headers, body = to_binary(event)
  return body, 200, headers
```

Note that functions may set both headers and response codes as secondary
and tertiary response values from function invocation. 

## Dependencies
Developers are not restricted to the dependencies provided in the template
`requirements.txt` file. Additional dependencies can be added as they would be
in any other project by simply adding them to the `requirements.txt` file.

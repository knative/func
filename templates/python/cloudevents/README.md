# Python CloudEvents Function

Welcome to your new Python function project! The boilerplate function
code can be found in [`func.py`](./func.py). This function is meant
to respond to [Cloud Events](https://cloudevents.io/).

## Endpoints

Running this function will expose three endpoints.

  * `/` The endpoint for your function.
  * `/health/readiness` The endpoint for a readiness health check
  * `/health/liveness` The endpoint for a liveness health check

The health checks can be accessed in your browser at
[http://localhost:8080/health/readiness]() and
[http://localhost:8080/health/liveness]().

You can use `func invoke` to send an event to the function endpoint.


## Testing

This function project includes a [unit test](./test_func.py). Update this
as you add business logic to your function in order to test its behavior.

```console
python test_func.py
```

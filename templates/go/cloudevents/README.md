# Go Cloud Events Function

Welcome to your new Go Function! The boilerplate function code can be found in [`function.go`](function.go). This Function responds to [Cloud Events](https://cloudevents.io/).

## Development

Develop new features by adding a test to [`function_test.go`](function_test.go) for each feature, and confirm it works with `go test`.

Deploy your changes using `func deploy`.  You can also invoke your function
directly by running it with `func run` and then using `curl`:

```console
curl -v -X POST -d '{"message": "hello"}' \
  -H'Content-type: application/json' \
  -H'Ce-id: 1' \
  -H'Ce-source: cloud-event-example' \
  -H'Ce-subject: Echo content' \
  -H'Ce-type: MyEvent' \
  -H'Ce-specversion: 1.0' \
  http://localhost:8080/
```

For more, see [the complete documentation]('https://github.com/knative/func/tree/main/docs')


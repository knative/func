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

### Import Private Go Modules
If you want to use a module that is in a private `git` repository,
you can do it by mounting credentials and by setting appropriate environment variable.

This is done by setting the `build.volumes` and `build.buildEnvs` properties in the `func.yaml` config file.

#### pack
For the `pack` builder have to use [paketo bindings](https://github.com/paketo-buildpacks/git?tab=readme-ov-file#bindings):
```yaml
# $schema: https://raw.githubusercontent.com/knative/func/refs/heads/main/schema/func_yaml-schema.json
specVersion: 0.36.0
name: go-fn
runtime: go
created: 2025-03-17T02:02:34.196208671+01:00
build:
  buildEnvs:
    - name: GOPRIVATE
      value: example.com
    - name: SERVICE_BINDING_ROOT
      value: /bindings
  volumes:
    - hostPath: /tmp/git-binding
      path: /bindings/git-binding
```

#### s2i
For the `s2i` builder you have to mount credentials in `.netrc` format.
```yaml
# $schema: https://raw.githubusercontent.com/knative/func/refs/heads/main/schema/func_yaml-schema.json
specVersion: 0.36.0
name: go-fn
runtime: go
created: 2025-03-17T02:02:34.196208671+01:00
build:
  buildEnvs:
    - name: GOPRIVATE
      value: example.com
  volumes:
    - hostPath: /home/jdoe/.netrc
      path: /opt/app-root/src/.netrc
```

For more, see [the complete documentation]('https://github.com/knative/func/tree/main/docs')


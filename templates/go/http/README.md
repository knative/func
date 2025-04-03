# Go HTTP Function

Welcome to your new Go Function! The boilerplate function code can be found in
[`handle.go`](handle.go). This Function responds to HTTP requests.

## Development

Develop new features by adding a test to [`handle_test.go`](handle_test.go) for
each feature, and confirm it works with `go test`.

Update the running analog of the function using the `func` CLI or client
library, and it can be invoked from your browser or from the command line:

```console
curl http://myfunction.example.com/
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
  volumes:
    - hostPath: /home/jdoe/.netrc
      path: /opt/app-root/src/.netrc
```

For more, see [the complete documentation]('https://github.com/knative/func/tree/main/docs')



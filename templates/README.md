# Templates

## Packaging

When updates are made to these templates, they must be packaged (serialized as a Go byte array)
by running `make zz_filesystem_generated.go`, and checking in the resultant `zz_filesystem_generated.go` file.

## How it works

The `./generate/templates` directory contains Go program that generates `zz_filesystem_generated.go`.
The file defines byte array variable named `templatesZip`.
The variable contains ZIP representation of the templates directory.
The byte array variable is then used to instantiate exported global variable `function.EmbeddedTemplatesFS`,
which implements standard Go interfaces `fs.ReadDirFS` and `fs.StatFS`.

## Rationale

Until such time as embedding static assets in binaries is included in the
base `go build` functionality (see https://github.com/golang/go/issues/35950)
we need to use our custom serialization script (`./generate/templates/main.go`).

Native Go embedding introduced in Go 1.16 could be used for executable binary,
however it cannot be used for library.
For a library we need to generate a Go source code containing the templates.

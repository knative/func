# Templates

## Packaging

When updates are made to these templates, they must be packaged (serialized as
a Go struture) by running `make`, and checking in the resultant `pkged.go` file.

## How it works

running `make` in turn installs the `pkger` binary, which can be installed via:
`go get github.com/markbates/pkger/cmd/pkger`
Make then invokes `pkger` before `go build`.

The resulting `pkged.go` file includes the contents of the templates directory,
encoded as a Go strucutres which is then makde available in code using an API
similar to the standard library's `os` package.

## Rationale

Until such time as embedding static assets in binaries is included in the
base `go build` functionality (see https://github.com/golang/go/issues/35950)
a third-party tool is required and pkger provides an API very similar  
to the `os` package.


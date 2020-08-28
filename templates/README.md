# Templates

## Updating

When any templates are modified, `../pkged.go` should be regenerated.

```
go get github.com/markbates/pkger/cmd/pkger
pkger
```
Generates a pkged.go containing serialized versions of the contents of 
the templates directory, made accessible at runtime. 

Until such time as embedding static assets in binaries is included in the
base go build functionality (see https://github.com/golang/go/issues/35950)
a third-party tool is required and pkger provides an API largely compatible 
with the os package.


package s2i

import (
	"fmt"

	fn "knative.dev/func/pkg/functions"
)

// GoAssembler
//
// Adapted from /usr/libexec/s2i/assemble within the UBI-8 go-toolchain
// such that the "go build" command builds subdirectory .s2i/builds/last
// (where main resides) rather than the root.
// TODO: many apps use the pattern of having main in a subdirectory, for
// example the idiomatic "./cmd/myapp/main.go".  It would therefore be
// beneficial to submit a patch to the go-toolchain source allowing this
// path to be customized with an environment variable instead
const GoAssembler = `
#!/bin/bash
set -e
pushd /tmp/src
if [[ $(go list -f {{.Incomplete}}) == "true" ]]; then
    INSTALL_URL=${INSTALL_URL:-$IMPORT_URL}
    if [[ ! -z "$IMPORT_URL" ]]; then
        popd
        echo "Assembling GOPATH"
        export GOPATH=$(realpath $HOME/go)
        mkdir -p $GOPATH/src/$IMPORT_URL
        mv /tmp/src/* $GOPATH/src/$IMPORT_URL
        if [[ -d /tmp/artifacts/pkg ]]; then
            echo "Restoring previous build artifacts"
            mv /tmp/artifacts/pkg $GOPATH
        fi
        # Resolve dependencies, ignore if vendor present
        if [[ ! -d $GOPATH/src/$INSTALL_URL/vendor ]]; then
            echo "Resolving dependencies"
            pushd $GOPATH/src/$INSTALL_URL
            go get
            popd
        fi
        # lets build
        pushd $GOPATH/src/$INSTALL_URL
        echo "Building"
        go install -i $INSTALL_URL
        mv $GOPATH/bin/* /opt/app-root/gobinary
        popd
        exit
    fi
    exec /$STI_SCRIPTS_PATH/usage
else
    pushd .s2i/builds/last
    go build -o /opt/app-root/gobinary
    popd
    popd
fi
`

func assembler(f fn.Function) (string, error) {
	switch f.Runtime {
	case "go":
		return GoAssembler, nil
	default:
		return "", fmt.Errorf("no assembler defined for runtime %q", f.Runtime)
	}
}

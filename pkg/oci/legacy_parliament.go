package oci

// LEGACY PYTHON: the host builder can't build the old parliament layout, so it is
// rejected. Delete this file on parliament sunset.

import "errors"

// ErrLegacyParliamentHost rejects a parliament function on the host builder.
var ErrLegacyParliamentHost = errors.New("the host builder cannot build legacy parliament Python functions (def main(context)); use --builder=pack or --builder=s2i")

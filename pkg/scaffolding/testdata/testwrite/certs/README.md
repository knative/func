# Test Certs

This directory mocks the existence of a 'certs' directory peer to the various
languages in the templates directory.  This directory is required for the
error-free execution of Scaffolding Write tests, since it is used to
create the certificates container layer.

The actual certs used are located in templates/certs, and are refreshed
by running `make certs` or by running a `make clean`
before a `make`.


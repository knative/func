module s

replace f => ./f

go 1.19

require (
	f v0.0.0-00010101000000-000000000000
	knative.dev/func-go v0.20.0
)

require (
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/rs/zerolog v1.32.0 // indirect
	golang.org/x/sys v0.17.0 // indirect
)

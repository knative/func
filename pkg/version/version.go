package version

// Default is the fallback version used when no build-time version was
// injected (e.g. source builds that bypass the Makefile).
const Default = "v0.0.0+source"

var (
	Vers = Default // overwritten by ldflags when make actually has a describe
	Kver string
	Hash string
)

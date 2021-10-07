package function

// Template
type Template struct {
	// Name (short name) of this template within the repository.
	// See .Fullname for the calculated field wich is the unique primary id.
	Name string `yaml:"-"` // use filesystem for name, not yaml
	// Runtime for which this template applies.
	Runtime string
	// Repository within which this template is contained.
	Repository string
	// BuildConfig defines builders and buildpacks.  the denormalized view of
	// members which can be defined per repo or per runtime first.
	BuildConfig `yaml:",inline"`
	// HealthEndpoints.  The denormalized view of members which can be defined
	// first per repo or per runtime.
	HealthEndpoints `yaml:"healthEndpoints,omitempty"`
}

// Fullname is a caluclated field of [repo]/[name] used
// to uniquely reference a template which may share a name
// with one in another repository.
func (t Template) Fullname() string {
	return t.Repository + "/" + t.Name
}

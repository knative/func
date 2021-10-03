package function

// Template
type Template struct {
	// Name (short name) of this template within the repository.
	// See .Fullname for the calculated field wich is the unique primary id.
	Name string
	// Runtime for which this template applies.
	Runtime string
	// Repository within which this template is contained.
	Repository string
}

// Fullname is a caluclated field of [repo]/[name] used
// to uniquely reference a template which may share a name
// with one in another repository.
func (t Template) Fullname() string {
	return t.Repository + "/" + t.Name
}

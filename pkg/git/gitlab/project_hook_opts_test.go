package gitlab

import "testing"

func TestProjectHookOpts_SSLVerificationEnabled(t *testing.T) {
	opts := projectHookOpts("https://example.invalid/hook", "tok")
	if opts == nil {
		t.Fatal("nil options")
	}
	if opts.EnableSSLVerification == nil {
		t.Fatal("EnableSSLVerification is nil")
	}
	if !*opts.EnableSSLVerification {
		t.Fatal("EnableSSLVerification must be true")
	}
	if opts.PushEvents == nil || !*opts.PushEvents {
		t.Fatal("PushEvents must be true for compatibility with prior behavior")
	}
	if opts.URL == nil || *opts.URL != "https://example.invalid/hook" {
		t.Fatalf("got URL %#v", strPtr(opts.URL))
	}
	if opts.Token == nil || *opts.Token != "tok" {
		t.Fatalf("got Token %#v", strPtr(opts.Token))
	}
}

func strPtr(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

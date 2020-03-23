package client_test

import (
	"path/filepath"
	"testing"

	"github.com/lkingland/faas/client"
	"github.com/lkingland/faas/client/mock"
)

// TestInvalidDomain ensures that creating from a directory strucutre
// where the domain can not be derived while simultaneously failing to provide
// an explicit name fails.
func TestInvalidDomain(t *testing.T) {
	_, err := client.New(
		// Ensure no domain is found even if we are actually running within a path
		// from which a domain could be derived.
		client.WithDomainSearchLimit(0),
	)
	if err == nil {
		t.Fatal("no error generated for unspecified and underivable name")
	}
}

// TestCreate ensures that instantiation completes without error when provided with a
// language.  A single client instance services a single Service Function instance
// and as such requires the desired effective DNS for the function.  This is an optional
// parameter, as it is intended to be derived from directory path.
func TestCreate(t *testing.T) {
	client, err := client.New(client.WithName("my.example.com"))
	if err != nil {
		t.Fatal(err)
	}
	// missing language shold error
	if err := client.Create(""); err == nil {
		t.Fatal("missing language did not generate error")
	}
	// Any language provided works by default, with the concrete implementation
	// of the initializer being the decider if the language provided is supported.
	if err := client.Create("go"); err != nil {
		t.Fatal(err)
	}
}

// TestCreateInitializes ensures that a call to Create invokes the Service
// Function Initializer with correct parameters.
func TestCreateInitializes(t *testing.T) {
	initializer := mock.NewInitializer()
	client, err := client.New(
		client.WithRoot("./testdata/example.com/admin"), // set function root
		client.WithInitializer(initializer),             // will receive the final value
	)
	if err != nil {
		t.Fatal(err)
	}
	initializer.InitializeFn = func(name, language, path string) error {
		if name != "admin.example.com" {
			t.Fatalf("initializer expected name 'admin.example.com', got '%v'", name)
		}
		if language != "go" {
			t.Fatalf("initializer expected language 'go', got '%v'", language)
		}
		expectedPath, err := filepath.Abs("./testdata/example.com/admin")
		if err != nil {
			t.Fatal(err)
		}
		if path != expectedPath {
			t.Fatalf("initializer expected path '%v', got '%v'", expectedPath, path)
		}
		return nil
	}
	if err := client.Create("go"); err != nil {
		t.Fatal(err)
	}
	if !initializer.InitializeInvoked {
		t.Fatal("initializer was not invoked")
	}
}

// TestDeploy ensures that a call to Deploy invokes the Sevice Function
// Deployer with the correct parameters.
func TestDeploy(t *testing.T) {
	deployer := mock.NewDeployer()
	client, err := client.New(
		client.WithRoot("./testdata/example.com/admin"), // set function root
		client.WithDeployer(deployer),                   // will receive the final value
	)
	if err != nil {
		t.Fatal(err)
	}
	deployer.DeployFn = func(name, path string) (address string, err error) {
		if name != "admin.example.com" {
			t.Fatalf("deployer expected name 'admin.example.com', got '%v'", name)
		}
		expectedPath, err := filepath.Abs("./testdata/example.com/admin")
		if err != nil {
			t.Fatal(err)
		}
		if path != expectedPath {
			t.Fatalf("deployer expected path '%v', got '%v'", expectedPath, path)
		}
		return
	}
	if err := client.Deploy(); err != nil {
		t.Fatal(err)
	}
	if !deployer.DeployInvoked {
		t.Fatal("deployer was not invoked")
	}
}

// TestDeployDomain ensures that the effective domain is dervied from
// directory structure.  See the unit tests for pathToDomain for details.
func TestDeployDomain(t *testing.T) {
	// the mock dns provider does nothing but receive the caluclated
	// domain name via it's Provide(domain) method, which is the value
	// being tested here.
	dnsProvider := mock.NewDNSProvider()

	client, err := client.New(
		client.WithRoot("./testdata/example.com"), // set function root
		client.WithDomainSearchLimit(1),           // Limit recursion to one level
		client.WithDNSProvider(dnsProvider),       // will receive the final value
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := client.Deploy(); err != nil {
		t.Fatal(err)
	}
	if !dnsProvider.ProvideInvoked {
		t.Fatal("dns provider was not invoked")
	}
	if dnsProvider.NameRequested != "example.com" {
		t.Fatalf("expected 'example.com', got '%v'", dnsProvider.NameRequested)
	}
}

// TestDeploySubdomain ensures that a subdirectory is interpreted as a subdomain
// when calculating final domain.  See the unit tests for pathToDomain for the
// details and edge cases of this caluclation.
func TestDeploySubdomain(t *testing.T) {
	dnsProvider := mock.NewDNSProvider()
	client, err := client.New(
		client.WithRoot("./testdata/example.com/admin"),
		client.WithDomainSearchLimit(2),
		client.WithDNSProvider(dnsProvider),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Deploy(); err != nil {
		t.Fatal(err)
	}
	if !dnsProvider.ProvideInvoked {
		t.Fatal("dns provider was not invoked")
	}
	if dnsProvider.NameRequested != "admin.example.com" {
		t.Fatalf("expected 'admin.example.com', got '%v'", dnsProvider.NameRequested)
	}
}

// TestRun ensures that the runner is invoked with the absolute path requested.
func TestRun(t *testing.T) {
	root := "./testdata/example.com/admin"
	runner := mock.NewRunner()
	client, err := client.New(
		client.WithRoot(root),
		client.WithRunner(runner),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Run(); err != nil {
		t.Fatal(err)
	}
	if !runner.RunInvoked {
		t.Fatal("run did not invoke the runner")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if runner.RootRequested != absRoot {
		t.Fatalf("expected path '%v', got '%v'", absRoot, runner.RootRequested)
	}

}

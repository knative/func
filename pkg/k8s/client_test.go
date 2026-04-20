package k8s

import (
	"encoding/base64"
	"testing"

	"k8s.io/client-go/rest"

	fn "knative.dev/func/pkg/functions"
)

func TestBuildRestConfig_NoAuth(t *testing.T) {
	cfg, err := BuildRestConfig("https://example.com", fn.ClusterVerify{}, fn.UserAuth{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Host != "https://example.com" {
		t.Fatalf("expected host https://example.com, got %s", cfg.Host)
	}
	if cfg.BearerToken != "" {
		t.Fatalf("expected empty token, got %s", cfg.BearerToken)
	}
}

func TestBuildRestConfig_Token(t *testing.T) {
	cfg, err := BuildRestConfig("https://example.com:6443", fn.ClusterVerify{}, fn.UserAuth{
		Token: "my-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "https://example.com:6443" {
		t.Fatalf("expected host https://example.com:6443, got %s", cfg.Host)
	}
	if cfg.BearerToken != "my-token" {
		t.Fatalf("expected token my-token, got %s", cfg.BearerToken)
	}
}

func TestBuildRestConfig_ClientCert(t *testing.T) {
	certPEM := base64.StdEncoding.EncodeToString([]byte("CERT"))
	keyPEM := base64.StdEncoding.EncodeToString([]byte("KEY"))

	cfg, err := BuildRestConfig("https://example.com", fn.ClusterVerify{}, fn.UserAuth{
		ClientCertificateData: certPEM,
		ClientKeyData:         keyPEM,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(cfg.CertData) != "CERT" {
		t.Fatalf("unexpected CertData: %s", cfg.CertData)
	}
	if string(cfg.KeyData) != "KEY" {
		t.Fatalf("unexpected KeyData: %s", cfg.KeyData)
	}
}

func TestBuildRestConfig_Exec(t *testing.T) {
	cfg, err := BuildRestConfig("https://example.com", fn.ClusterVerify{}, fn.UserAuth{
		Exec: &fn.ExecAuth{
			Command:    "gke-gcloud-auth-plugin",
			Args:       []string{"--region", "us-central1"},
			APIVersion: "client.authentication.k8s.io/v1beta1",
			Env: []fn.ExecEnv{
				{Name: "USE_GKE_GCLOUD_AUTH_PLUGIN", Value: "True"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ExecProvider == nil {
		t.Fatal("expected ExecProvider to be set")
	}
	if cfg.ExecProvider.Command != "gke-gcloud-auth-plugin" {
		t.Fatalf("unexpected command: %s", cfg.ExecProvider.Command)
	}
	if cfg.ExecProvider.APIVersion != "client.authentication.k8s.io/v1beta1" {
		t.Fatalf("unexpected apiVersion: %s", cfg.ExecProvider.APIVersion)
	}
	if len(cfg.ExecProvider.Args) != 2 || cfg.ExecProvider.Args[0] != "--region" {
		t.Fatalf("unexpected args: %v", cfg.ExecProvider.Args)
	}
	if len(cfg.ExecProvider.Env) != 1 || cfg.ExecProvider.Env[0].Name != "USE_GKE_GCLOUD_AUTH_PLUGIN" {
		t.Fatalf("unexpected env: %v", cfg.ExecProvider.Env)
	}
}

func TestBuildRestConfig_TLS(t *testing.T) {
	caData := base64.StdEncoding.EncodeToString([]byte("CA-CERT"))

	cfg, err := BuildRestConfig("https://example.com",
		fn.ClusterVerify{
			CertificateAuthorityData: caData,
			InsecureSkipTLSVerify:    true,
		},
		fn.UserAuth{
			Token: "tok",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if string(cfg.CAData) != "CA-CERT" {
		t.Fatalf("unexpected CAData: %s", cfg.CAData)
	}
	if !cfg.Insecure {
		t.Fatal("expected Insecure=true")
	}
}

func TestBuildRestConfig_BadBase64(t *testing.T) {
	_, err := BuildRestConfig("https://example.com",
		fn.ClusterVerify{
			CertificateAuthorityData: "not-valid-base64!@#$",
		},
		fn.UserAuth{
			Token: "tok",
		},
	)
	if err == nil {
		t.Fatal("expected error for bad base64 certificate-authority-data")
	}

	_, err = BuildRestConfig("https://example.com", fn.ClusterVerify{}, fn.UserAuth{
		ClientCertificateData: "not-valid!@#$",
		ClientKeyData:         base64.StdEncoding.EncodeToString([]byte("KEY")),
	})
	if err == nil {
		t.Fatal("expected error for bad base64 client-certificate-data")
	}

	_, err = BuildRestConfig("https://example.com", fn.ClusterVerify{}, fn.UserAuth{
		ClientCertificateData: base64.StdEncoding.EncodeToString([]byte("CERT")),
		ClientKeyData:         "not-valid!@#$",
	})
	if err == nil {
		t.Fatal("expected error for bad base64 client-key-data")
	}
}

func TestBuildRestConfig_AllCredentialsApplied(t *testing.T) {
	certPEM := base64.StdEncoding.EncodeToString([]byte("CERT"))
	keyPEM := base64.StdEncoding.EncodeToString([]byte("KEY"))

	cfg, err := BuildRestConfig("https://example.com", fn.ClusterVerify{}, fn.UserAuth{
		Token:                 "my-token",
		ClientCertificateData: certPEM,
		ClientKeyData:         keyPEM,
		Exec: &fn.ExecAuth{
			Command:    "some-plugin",
			APIVersion: "client.authentication.k8s.io/v1beta1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BearerToken != "my-token" {
		t.Fatalf("expected token my-token, got %s", cfg.BearerToken)
	}
	if string(cfg.CertData) != "CERT" {
		t.Fatal("expected CertData to be set alongside token")
	}
	if string(cfg.KeyData) != "KEY" {
		t.Fatal("expected KeyData to be set alongside token")
	}
	if cfg.ExecProvider == nil {
		t.Fatal("expected ExecProvider to be set alongside token")
	}
}

func TestSetClusterOverride(t *testing.T) {
	cleanup, err := SetClusterOverride("https://override.example.com", "", fn.ClusterVerify{}, fn.UserAuth{
		Token: "override-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	cc := GetClientConfig()
	cfg, err := cc.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "https://override.example.com" {
		t.Fatalf("expected override host, got %s", cfg.Host)
	}
	if cfg.BearerToken != "override-token" {
		t.Fatalf("expected override token, got %s", cfg.BearerToken)
	}

	ns, _, err := cc.Namespace()
	if err != nil {
		t.Fatal(err)
	}
	if ns != "default" {
		t.Fatalf("expected default, got %s", ns)
	}
}

func TestSetClusterOverride_WithNamespace(t *testing.T) {
	cleanup, err := SetClusterOverride("https://override.example.com", "my-namespace", fn.ClusterVerify{}, fn.UserAuth{
		Token: "override-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	cc := GetClientConfig()
	ns, _, err := cc.Namespace()
	if err != nil {
		t.Fatal(err)
	}
	if ns != "my-namespace" {
		t.Fatalf("expected my-namespace, got %s", ns)
	}
}

func TestSetClusterOverride_NoAuth(t *testing.T) {
	cleanup, err := SetClusterOverride("https://example.com", "", fn.ClusterVerify{}, fn.UserAuth{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cleanup == nil {
		t.Fatal("expected cleanup function")
	}
	cleanup()
}

func TestClearClusterOverride(t *testing.T) {
	cleanup, err := SetClusterOverride("https://override.example.com", "test-ns", fn.ClusterVerify{}, fn.UserAuth{
		Token: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}
	cleanup()

	if restConfigOverride != nil {
		t.Fatal("expected restConfigOverride to be nil after cleanup")
	}
	if namespaceOverride != "" {
		t.Fatal("expected namespaceOverride to be empty after cleanup")
	}
}

func TestStaticClientConfig_ReturnsCopy(t *testing.T) {
	cleanup, err := SetClusterOverride("https://example.com", "", fn.ClusterVerify{}, fn.UserAuth{
		Token: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	cc := GetClientConfig()
	cfg1, _ := cc.ClientConfig()
	cfg2, _ := cc.ClientConfig()
	cfg1.Host = "mutated"
	if cfg2.Host == "mutated" {
		t.Fatal("ClientConfig() should return independent copies")
	}
}

func TestStaticClientConfig_RawConfig(t *testing.T) {
	cleanup, err := SetClusterOverride("https://example.com", "my-ns", fn.ClusterVerify{}, fn.UserAuth{
		Token: "my-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	cc := GetClientConfig()
	raw, err := cc.RawConfig()
	if err != nil {
		t.Fatal(err)
	}
	if raw.CurrentContext == "" {
		t.Fatal("expected non-empty CurrentContext")
	}
	ctx := raw.Contexts[raw.CurrentContext]
	if ctx == nil {
		t.Fatal("expected context entry")
	}
	cluster := raw.Clusters[ctx.Cluster]
	if cluster == nil || cluster.Server != "https://example.com" {
		t.Fatalf("expected server https://example.com, got %v", cluster)
	}
	authInfo := raw.AuthInfos[ctx.AuthInfo]
	if authInfo == nil || authInfo.Token != "my-token" {
		t.Fatalf("expected token my-token, got %v", authInfo)
	}
	if ctx.Namespace != "my-ns" {
		t.Fatalf("expected namespace my-ns, got %s", ctx.Namespace)
	}
}

func TestStaticClientConfig_ConfigAccess(t *testing.T) {
	s := &staticClientConfig{cfg: &rest.Config{}}
	if s.ConfigAccess() != nil {
		t.Fatal("expected nil ConfigAccess")
	}
}

func TestGetClientConfig_FallsBackToKubeconfig(t *testing.T) {
	ClearClusterOverride()
	cc := GetClientConfig()
	_, ok := cc.(*staticClientConfig)
	if ok {
		t.Fatal("expected kubeconfig-based ClientConfig when no override is set")
	}
}

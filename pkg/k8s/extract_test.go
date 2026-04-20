package k8s

import (
	"encoding/base64"
	"path/filepath"
	"testing"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"k8s.io/client-go/tools/clientcmd"
)

// writeTestKubeconfig serialises a clientcmdapi.Config to a temp file and
// points the KUBECONFIG env var at it for the duration of the test.
func writeTestKubeconfig(t *testing.T, cfg clientcmdapi.Config) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")
	if err := clientcmd.WriteToFile(cfg, path); err != nil {
		t.Fatalf("failed to write test kubeconfig: %v", err)
	}
	t.Setenv("KUBECONFIG", path)
}

func TestExtractClusterAuth_Token(t *testing.T) {
	writeTestKubeconfig(t, clientcmdapi.Config{
		CurrentContext: "test-ctx",
		Contexts: map[string]*clientcmdapi.Context{
			"test-ctx": {Cluster: "test-cluster", AuthInfo: "test-user"},
		},
		Clusters: map[string]*clientcmdapi.Cluster{
			"test-cluster": {
				Server:                   "https://example.com:6443",
				CertificateAuthorityData: []byte("CA-DATA"),
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"test-user": {Token: "my-token"},
		},
	})

	// Ensure no cluster override interferes.
	ClearClusterOverride()

	url, clusterTLS, user, err := ExtractClusterAuth()
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://example.com:6443" {
		t.Fatalf("expected https://example.com:6443, got %s", url)
	}
	if user.Token != "my-token" {
		t.Fatalf("expected token my-token, got %s", user.Token)
	}

	// CertificateAuthorityData should be base64-encoded.
	decoded, err := base64.StdEncoding.DecodeString(clusterTLS.CertificateAuthorityData)
	if err != nil {
		t.Fatalf("CertificateAuthorityData is not valid base64: %v", err)
	}
	if string(decoded) != "CA-DATA" {
		t.Fatalf("unexpected CertificateAuthorityData: %s", string(decoded))
	}
}

func TestExtractClusterAuth_ClientCert(t *testing.T) {
	writeTestKubeconfig(t, clientcmdapi.Config{
		CurrentContext: "ctx",
		Contexts: map[string]*clientcmdapi.Context{
			"ctx": {Cluster: "c", AuthInfo: "u"},
		},
		Clusters: map[string]*clientcmdapi.Cluster{
			"c": {Server: "https://cert.example.com"},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"u": {
				ClientCertificateData: []byte("CERT-PEM"),
				ClientKeyData:         []byte("KEY-PEM"),
			},
		},
	})
	ClearClusterOverride()

	url, _, user, err := ExtractClusterAuth()
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://cert.example.com" {
		t.Fatalf("unexpected URL: %s", url)
	}
	if user.Token != "" {
		t.Fatal("expected empty token for client cert auth")
	}

	certDecoded, _ := base64.StdEncoding.DecodeString(user.ClientCertificateData)
	if string(certDecoded) != "CERT-PEM" {
		t.Fatalf("unexpected ClientCertificateData: %s", string(certDecoded))
	}
	keyDecoded, _ := base64.StdEncoding.DecodeString(user.ClientKeyData)
	if string(keyDecoded) != "KEY-PEM" {
		t.Fatalf("unexpected ClientKeyData: %s", string(keyDecoded))
	}
}

func TestExtractClusterAuth_Exec(t *testing.T) {
	writeTestKubeconfig(t, clientcmdapi.Config{
		CurrentContext: "ctx",
		Contexts: map[string]*clientcmdapi.Context{
			"ctx": {Cluster: "c", AuthInfo: "u"},
		},
		Clusters: map[string]*clientcmdapi.Cluster{
			"c": {Server: "https://exec.example.com"},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"u": {
				Exec: &clientcmdapi.ExecConfig{
					Command:    "gke-gcloud-auth-plugin",
					Args:       []string{"--region", "us-central1"},
					APIVersion: "client.authentication.k8s.io/v1beta1",
					Env: []clientcmdapi.ExecEnvVar{
						{Name: "USE_GKE_GCLOUD_AUTH_PLUGIN", Value: "True"},
					},
				},
			},
		},
	})
	ClearClusterOverride()

	_, _, user, err := ExtractClusterAuth()
	if err != nil {
		t.Fatal(err)
	}
	if user.Exec == nil {
		t.Fatal("expected Exec to be set")
	}
	if user.Exec.Command != "gke-gcloud-auth-plugin" {
		t.Fatalf("unexpected command: %s", user.Exec.Command)
	}
	if user.Exec.APIVersion != "client.authentication.k8s.io/v1beta1" {
		t.Fatalf("unexpected apiVersion: %s", user.Exec.APIVersion)
	}
	if len(user.Exec.Args) != 2 || user.Exec.Args[0] != "--region" {
		t.Fatalf("unexpected args: %v", user.Exec.Args)
	}
	if len(user.Exec.Env) != 1 || user.Exec.Env[0].Name != "USE_GKE_GCLOUD_AUTH_PLUGIN" {
		t.Fatalf("unexpected env: %v", user.Exec.Env)
	}
}

func TestExtractClusterAuth_InsecureSkipTLS(t *testing.T) {
	writeTestKubeconfig(t, clientcmdapi.Config{
		CurrentContext: "ctx",
		Contexts: map[string]*clientcmdapi.Context{
			"ctx": {Cluster: "c", AuthInfo: "u"},
		},
		Clusters: map[string]*clientcmdapi.Cluster{
			"c": {
				Server:                "https://insecure.example.com",
				InsecureSkipTLSVerify: true,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"u": {Token: "tok"},
		},
	})
	ClearClusterOverride()

	_, clusterTLS, _, err := ExtractClusterAuth()
	if err != nil {
		t.Fatal(err)
	}
	if !clusterTLS.InsecureSkipTLSVerify {
		t.Fatal("expected InsecureSkipTLSVerify=true")
	}
}

func TestExtractClusterAuth_NoCurrentContext(t *testing.T) {
	writeTestKubeconfig(t, clientcmdapi.Config{})
	ClearClusterOverride()

	_, _, _, err := ExtractClusterAuth()
	if err == nil {
		t.Fatal("expected error for no current context")
	}
}

func TestExtractClusterAuth_MissingContext(t *testing.T) {
	writeTestKubeconfig(t, clientcmdapi.Config{
		CurrentContext: "does-not-exist",
	})
	ClearClusterOverride()

	_, _, _, err := ExtractClusterAuth()
	if err == nil {
		t.Fatal("expected error for missing context")
	}
}

func TestExtractClusterAuth_MissingCluster(t *testing.T) {
	writeTestKubeconfig(t, clientcmdapi.Config{
		CurrentContext: "ctx",
		Contexts: map[string]*clientcmdapi.Context{
			"ctx": {Cluster: "no-such-cluster", AuthInfo: "u"},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"u": {Token: "tok"},
		},
	})
	ClearClusterOverride()

	_, _, _, err := ExtractClusterAuth()
	if err == nil {
		t.Fatal("expected error for missing cluster")
	}
}

func TestExtractClusterAuth_MissingUser(t *testing.T) {
	writeTestKubeconfig(t, clientcmdapi.Config{
		CurrentContext: "ctx",
		Contexts: map[string]*clientcmdapi.Context{
			"ctx": {Cluster: "c", AuthInfo: "no-such-user"},
		},
		Clusters: map[string]*clientcmdapi.Cluster{
			"c": {Server: "https://example.com"},
		},
	})
	ClearClusterOverride()

	_, _, _, err := ExtractClusterAuth()
	if err == nil {
		t.Fatal("expected error for missing user")
	}
}

func TestExtractClusterAuth_AllCredentialsExtracted(t *testing.T) {
	// When both token and client cert are present, both are extracted.
	writeTestKubeconfig(t, clientcmdapi.Config{
		CurrentContext: "ctx",
		Contexts: map[string]*clientcmdapi.Context{
			"ctx": {Cluster: "c", AuthInfo: "u"},
		},
		Clusters: map[string]*clientcmdapi.Cluster{
			"c": {Server: "https://example.com"},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"u": {
				Token:                 "my-token",
				ClientCertificateData: []byte("CERT"),
				ClientKeyData:         []byte("KEY"),
			},
		},
	})
	ClearClusterOverride()

	_, _, user, err := ExtractClusterAuth()
	if err != nil {
		t.Fatal(err)
	}
	if user.Token != "my-token" {
		t.Fatalf("expected token, got %s", user.Token)
	}
	if user.ClientCertificateData == "" {
		t.Fatal("expected ClientCertificateData to be extracted alongside token")
	}
}

func TestExtractClusterAuth_NoCAData(t *testing.T) {
	writeTestKubeconfig(t, clientcmdapi.Config{
		CurrentContext: "ctx",
		Contexts: map[string]*clientcmdapi.Context{
			"ctx": {Cluster: "c", AuthInfo: "u"},
		},
		Clusters: map[string]*clientcmdapi.Cluster{
			"c": {Server: "https://example.com"},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"u": {Token: "tok"},
		},
	})
	ClearClusterOverride()

	_, clusterTLS, _, err := ExtractClusterAuth()
	if err != nil {
		t.Fatal(err)
	}
	if clusterTLS.CertificateAuthorityData != "" {
		t.Fatalf("expected empty CertificateAuthorityData, got %s", clusterTLS.CertificateAuthorityData)
	}
}

func TestExtractClusterAuth_IgnoresFileKubeconfig(t *testing.T) {
	// Verify that a non-existent KUBECONFIG file causes a sensible error
	// (not a panic).
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "nonexistent"))
	ClearClusterOverride()

	_, _, _, err := ExtractClusterAuth()
	if err == nil {
		// We expect an error because the kubeconfig doesn't exist or is empty,
		// so there's no current context.
		t.Fatal("expected error for missing kubeconfig file")
	}
}

func TestExtractClusterAuth_RealEnvIsolated(t *testing.T) {
	// Ensure KUBECONFIG override isolates us from the real config.
	writeTestKubeconfig(t, clientcmdapi.Config{
		CurrentContext: "isolated",
		Contexts: map[string]*clientcmdapi.Context{
			"isolated": {Cluster: "c", AuthInfo: "u"},
		},
		Clusters: map[string]*clientcmdapi.Cluster{
			"c": {Server: "https://isolated.example.com"},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"u": {Token: "isolated-token"},
		},
	})
	ClearClusterOverride()

	url, _, user, err := ExtractClusterAuth()
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://isolated.example.com" {
		t.Fatalf("expected isolated URL, got %s", url)
	}
	if user.Token != "isolated-token" {
		t.Fatalf("expected isolated-token, got %s", user.Token)
	}

}

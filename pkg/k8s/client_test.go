package k8s

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v2"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	fn "knative.dev/func/pkg/functions"
)

// writeTestKubeconfig serializes a clientcmdapi.Config to a temp file and
// points the KUBECONFIG env var at it for the duration of the test.
func writeTestKubeconfig(t *testing.T, cfg clientcmdapi.Config) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")
	if err := clientcmd.WriteToFile(cfg, path); err != nil {
		t.Fatalf("failed to write test kubeconfig: %v", err)
	}
	t.Setenv("KUBECONFIG", path)
	return path
}

func testKubeconfig() clientcmdapi.Config {
	return clientcmdapi.Config{
		CurrentContext: "test-ctx",
		Contexts: map[string]*clientcmdapi.Context{
			"test-ctx": {Cluster: "test-cluster", AuthInfo: "test-user", Namespace: "test-ns"},
		},
		Clusters: map[string]*clientcmdapi.Cluster{
			"test-cluster": {
				Server:                   "https://kubeconfig.example.com:6443",
				CertificateAuthorityData: []byte("-----BEGIN CERTIFICATE-----\nCA-DATA\n-----END CERTIFICATE-----"),
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"test-user": {
				Token: "kubeconfig-token",
			},
		},
	}
}

func testKubeconfigWithCerts() clientcmdapi.Config {
	return clientcmdapi.Config{
		CurrentContext: "cert-ctx",
		Contexts: map[string]*clientcmdapi.Context{
			"cert-ctx": {Cluster: "cert-cluster", AuthInfo: "cert-user", Namespace: "cert-ns"},
		},
		Clusters: map[string]*clientcmdapi.Cluster{
			"cert-cluster": {
				Server:                   "https://cert.example.com:6443",
				CertificateAuthorityData: []byte("-----BEGIN CERTIFICATE-----\nCA-PEM\n-----END CERTIFICATE-----"),
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"cert-user": {
				ClientCertificateData: []byte("-----BEGIN CERTIFICATE-----\nCLIENT-CERT-PEM\n-----END CERTIFICATE-----"),
				ClientKeyData:         []byte("-----BEGIN RSA PRIVATE KEY-----\nCLIENT-KEY-PEM\n-----END RSA PRIVATE KEY-----"),
			},
		},
	}
}

// --- BuildClientConfig tests ---

func TestBuildClientConfig_NoOverride_UsesKubeconfig(t *testing.T) {
	writeTestKubeconfig(t, testKubeconfig())

	cc, _ := BuildClientConfig("", "", "", fn.Local{})
	cfg, err := cc.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "https://kubeconfig.example.com:6443" {
		t.Fatalf("expected kubeconfig host, got %s", cfg.Host)
	}
	if cfg.BearerToken != "kubeconfig-token" {
		t.Fatalf("expected kubeconfig token, got %s", cfg.BearerToken)
	}
}

func TestBuildClientConfig_NoOverride_Namespace(t *testing.T) {
	writeTestKubeconfig(t, testKubeconfig())

	cc, _ := BuildClientConfig("", "", "", fn.Local{})
	ns, _, err := cc.Namespace()
	if err != nil {
		t.Fatal(err)
	}
	if ns != "test-ns" {
		t.Fatalf("expected test-ns, got %s", ns)
	}
}

// TestBuildClientConfig_NamespacePin verifies that an explicit namespace (the
// function's f.Deploy.Namespace) pins the client's default namespace, winning
// over the active kubeconfig context. This is the namespace leg of the context
// pin: once set, cluster operations no longer follow `kubens`/active-context
// changes. (The empty case staying on the active "test-ns" is covered above.)
func TestBuildClientConfig_NamespacePin(t *testing.T) {
	writeTestKubeconfig(t, testKubeconfig()) // active context namespace = "test-ns"

	cc, err := BuildClientConfig("", "", "pinned-ns", fn.Local{})
	if err != nil {
		t.Fatal(err)
	}
	ns, _, err := cc.Namespace()
	if err != nil {
		t.Fatal(err)
	}
	if ns != "pinned-ns" {
		t.Fatalf("namespace = %q, want pinned-ns (must beat active context test-ns)", ns)
	}
}

// nsContextKubeconfig has two namespaced contexts: the active one targets
// cluster-a (namespace "active-ns"), a non-active one targets cluster-b
// (namespace "other-ns").
func nsContextKubeconfig() clientcmdapi.Config {
	return clientcmdapi.Config{
		CurrentContext: "active-ctx",
		Contexts: map[string]*clientcmdapi.Context{
			"active-ctx": {Cluster: "cluster-a", AuthInfo: "user-a", Namespace: "active-ns"},
			"other-ctx":  {Cluster: "cluster-b", AuthInfo: "user-b", Namespace: "other-ns"},
		},
		Clusters: map[string]*clientcmdapi.Cluster{
			"cluster-a": {Server: "https://cluster-a.example.com:6443"},
			"cluster-b": {Server: "https://cluster-b.example.com:6443"},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"user-a": {Token: "token-a"},
			"user-b": {Token: "token-b"},
		},
	}
}

// TestBuildClientConfig_NamespacePin_AdoptsMatchedContext verifies that when a
// cluster URL resolves to a (non-active) kubeconfig context, that context's
// namespace is adopted -- not the active context's. This is the kubeconfigAuth-
// Overrides namespace leg.
func TestBuildClientConfig_NamespacePin_AdoptsMatchedContext(t *testing.T) {
	writeTestKubeconfig(t, nsContextKubeconfig())

	// Resolve by cluster-b's URL (the non-active context, namespace "other-ns").
	cc, err := BuildClientConfig("https://cluster-b.example.com:6443", "", "", fn.Local{})
	if err != nil {
		t.Fatal(err)
	}
	ns, _, err := cc.Namespace()
	if err != nil {
		t.Fatal(err)
	}
	if ns != "other-ns" {
		t.Fatalf("namespace = %q, want other-ns (the matched context's namespace, not active active-ns)", ns)
	}
}

// TestBuildClientConfig_NamespacePin_ExplicitBeatsMatchedContext verifies the
// top of the precedence ladder: an explicit namespace wins even over the
// matched context's own namespace.
func TestBuildClientConfig_NamespacePin_ExplicitBeatsMatchedContext(t *testing.T) {
	writeTestKubeconfig(t, nsContextKubeconfig())

	cc, err := BuildClientConfig("https://cluster-b.example.com:6443", "", "explicit-ns", fn.Local{})
	if err != nil {
		t.Fatal(err)
	}
	ns, _, err := cc.Namespace()
	if err != nil {
		t.Fatal(err)
	}
	if ns != "explicit-ns" {
		t.Fatalf("namespace = %q, want explicit-ns (must beat matched context's other-ns)", ns)
	}
}

func TestBuildClientConfig_ClusterOverride_WithStoredAuth(t *testing.T) {
	writeTestKubeconfig(t, testKubeconfig())

	local := fn.Local{
		Auth: []fn.AuthEntry{{
			ClusterURL: "https://override.example.com:6443",
			User:       fn.UserAuth{Token: "override-tok"},
		}},
	}
	cc, _ := BuildClientConfig("https://override.example.com:6443", "", "", local)
	cfg, err := cc.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "https://override.example.com:6443" {
		t.Fatalf("expected override host, got %s", cfg.Host)
	}
}

func TestBuildClientConfig_ClusterOverride_NoAuth_Errors(t *testing.T) {
	writeTestKubeconfig(t, testKubeconfig())

	_, err := BuildClientConfig("https://unknown.example.com:6443", "", "", fn.Local{})
	if err == nil {
		t.Fatal("expected error for cluster URL with no auth source")
	}
}

func TestBuildClientConfig_ClusterOverride_MatchesKubeconfig(t *testing.T) {
	writeTestKubeconfig(t, testKubeconfig())

	cc, err := BuildClientConfig("https://kubeconfig.example.com:6443", "", "", fn.Local{})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := cc.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "https://kubeconfig.example.com:6443" {
		t.Fatalf("expected kubeconfig host, got %s", cfg.Host)
	}
	if cfg.BearerToken != "kubeconfig-token" {
		t.Fatalf("expected kubeconfig token resolved via URL match, got %s", cfg.BearerToken)
	}
}

func TestBuildClientConfig_TokenOverride(t *testing.T) {
	writeTestKubeconfig(t, testKubeconfig())

	cc, _ := BuildClientConfig("https://override.example.com:6443", "my-token", "", fn.Local{})
	cfg, err := cc.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BearerToken != "my-token" {
		t.Fatalf("expected my-token, got %s", cfg.BearerToken)
	}
}

func TestBuildClientConfig_TokenWithoutCluster_AppliedToKubeconfig(t *testing.T) {
	writeTestKubeconfig(t, testKubeconfig())

	cc, _ := BuildClientConfig("", "override-token", "", fn.Local{})
	cfg, err := cc.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BearerToken != "override-token" {
		t.Fatalf("expected override-token to apply even without cluster URL, got %s", cfg.BearerToken)
	}
	if cfg.Host != "https://kubeconfig.example.com:6443" {
		t.Fatalf("expected kubeconfig host unchanged, got %s", cfg.Host)
	}
}

func TestBuildClientConfig_StoredAuth_Token(t *testing.T) {
	writeTestKubeconfig(t, testKubeconfig())

	local := fn.Local{
		Auth: []fn.AuthEntry{{
			ClusterURL: "https://stored.example.com:6443",
			User:       fn.UserAuth{Token: "stored-token"},
		}},
	}

	cc, _ := BuildClientConfig("https://stored.example.com:6443", "", "", local)
	cfg, err := cc.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "https://stored.example.com:6443" {
		t.Fatalf("expected stored host, got %s", cfg.Host)
	}
	if cfg.BearerToken != "stored-token" {
		t.Fatalf("expected stored-token, got %s", cfg.BearerToken)
	}
}

func TestBuildClientConfig_StoredAuth_CertData(t *testing.T) {
	writeTestKubeconfig(t, testKubeconfig())

	caRaw := []byte("-----BEGIN CERTIFICATE-----\nMY-CA\n-----END CERTIFICATE-----")
	certRaw := []byte("-----BEGIN CERTIFICATE-----\nMY-CERT\n-----END CERTIFICATE-----")
	keyRaw := []byte("-----BEGIN RSA PRIVATE KEY-----\nMY-KEY\n-----END RSA PRIVATE KEY-----")

	local := fn.Local{
		Auth: []fn.AuthEntry{{
			ClusterURL: "https://certs.example.com",
			Cluster: fn.ClusterTLS{
				CertificateAuthorityData: base64.StdEncoding.EncodeToString(caRaw),
			},
			User: fn.UserAuth{
				ClientCertificateData: base64.StdEncoding.EncodeToString(certRaw),
				ClientKeyData:         base64.StdEncoding.EncodeToString(keyRaw),
			},
		}},
	}

	cc, _ := BuildClientConfig("https://certs.example.com", "", "", local)
	cfg, err := cc.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	if string(cfg.CAData) != string(caRaw) {
		t.Fatalf("CAData mismatch: got %q", cfg.CAData)
	}
	if string(cfg.CertData) != string(certRaw) {
		t.Fatalf("CertData mismatch: got %q", cfg.CertData)
	}
	if string(cfg.KeyData) != string(keyRaw) {
		t.Fatalf("KeyData mismatch: got %q", cfg.KeyData)
	}
}

func TestBuildClientConfig_StoredAuth_InsecureSkipTLS(t *testing.T) {
	writeTestKubeconfig(t, testKubeconfig())

	local := fn.Local{
		Auth: []fn.AuthEntry{{
			ClusterURL: "https://insecure.example.com",
			Cluster: fn.ClusterTLS{
				InsecureSkipTLSVerify: true,
			},
			User: fn.UserAuth{Token: "tok"},
		}},
	}

	cc, _ := BuildClientConfig("https://insecure.example.com", "", "", local)
	cfg, err := cc.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Insecure {
		t.Fatal("expected Insecure=true")
	}
}

func TestBuildClientConfig_StoredAuth_CertFilePaths(t *testing.T) {
	writeTestKubeconfig(t, testKubeconfig())

	dir := t.TempDir()
	caFile := filepath.Join(dir, "ca.crt")
	certFile := filepath.Join(dir, "client.crt")
	keyFile := filepath.Join(dir, "client.key")

	if err := os.WriteFile(caFile, []byte("CA-FILE-DATA"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certFile, []byte("CERT-FILE-DATA"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, []byte("KEY-FILE-DATA"), 0600); err != nil {
		t.Fatal(err)
	}

	local := fn.Local{
		Auth: []fn.AuthEntry{{
			ClusterURL: "https://filepaths.example.com",
			Cluster: fn.ClusterTLS{
				CertificateAuthority: caFile,
			},
			User: fn.UserAuth{
				ClientCertificate: certFile,
				ClientKey:         keyFile,
				Token:             "file-tok",
			},
		}},
	}

	cc, _ := BuildClientConfig("https://filepaths.example.com", "", "", local)
	cfg, err := cc.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CAFile != caFile {
		t.Fatalf("expected CAFile %s, got %s", caFile, cfg.CAFile)
	}
	if cfg.CertFile != certFile {
		t.Fatalf("expected CertFile %s, got %s", certFile, cfg.CertFile)
	}
	if cfg.KeyFile != keyFile {
		t.Fatalf("expected KeyFile %s, got %s", keyFile, cfg.KeyFile)
	}
}

func TestBuildClientConfig_TokenOverride_BeatsStoredToken(t *testing.T) {
	writeTestKubeconfig(t, testKubeconfig())

	local := fn.Local{
		Auth: []fn.AuthEntry{{
			ClusterURL: "https://stored.example.com",
			User:       fn.UserAuth{Token: "stored-token"},
		}},
	}

	cc, _ := BuildClientConfig("https://stored.example.com", "flag-token", "", local)
	cfg, err := cc.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BearerToken != "flag-token" {
		t.Fatalf("expected flag-token to win, got %s", cfg.BearerToken)
	}
}

func TestBuildClientConfig_ClusterOverride_WithToken_SkipsKubeconfigSearch(t *testing.T) {
	writeTestKubeconfig(t, testKubeconfig())

	// When a token is provided, kubeconfig search is skipped even if URL doesn't match
	cc, err := BuildClientConfig("https://unknown.example.com", "my-token", "", fn.Local{})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := cc.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "https://unknown.example.com" {
		t.Fatalf("expected override host, got %s", cfg.Host)
	}
	if cfg.BearerToken != "my-token" {
		t.Fatalf("expected my-token, got %s", cfg.BearerToken)
	}
}

// --- resolveKubeconfigAuth tests ---

func multiContextKubeconfig() clientcmdapi.Config {
	return clientcmdapi.Config{
		CurrentContext: "active-ctx",
		Contexts: map[string]*clientcmdapi.Context{
			"active-ctx": {Cluster: "cluster-a", AuthInfo: "user-a"},
			"other-ctx":  {Cluster: "cluster-b", AuthInfo: "user-b"},
		},
		Clusters: map[string]*clientcmdapi.Cluster{
			"cluster-a": {Server: "https://cluster-a.example.com:6443"},
			"cluster-b": {Server: "https://cluster-b.example.com:6443"},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"user-a": {Token: "token-a"},
			"user-b": {Token: "token-b"},
		},
	}
}

func TestBuildClientConfig_URLResolution_ActiveContext(t *testing.T) {
	writeTestKubeconfig(t, multiContextKubeconfig())

	cc, err := BuildClientConfig("https://cluster-a.example.com:6443", "", "", fn.Local{})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := cc.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BearerToken != "token-a" {
		t.Fatalf("expected active context token, got %s", cfg.BearerToken)
	}
}

func TestBuildClientConfig_URLResolution_NonActiveContext(t *testing.T) {
	writeTestKubeconfig(t, multiContextKubeconfig())

	cc, err := BuildClientConfig("https://cluster-b.example.com:6443", "", "", fn.Local{})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := cc.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BearerToken != "token-b" {
		t.Fatalf("expected non-active context token, got %s", cfg.BearerToken)
	}
}

func TestBuildClientConfig_URLResolution_MultipleMatches(t *testing.T) {
	writeTestKubeconfig(t, clientcmdapi.Config{
		CurrentContext: "ctx-1",
		Contexts: map[string]*clientcmdapi.Context{
			"ctx-1": {Cluster: "cluster-x", AuthInfo: "user-1"},
			"ctx-2": {Cluster: "cluster-y", AuthInfo: "user-2"},
			"ctx-3": {Cluster: "cluster-z", AuthInfo: "user-3"},
		},
		Clusters: map[string]*clientcmdapi.Cluster{
			"cluster-x": {Server: "https://active.example.com:6443"},
			"cluster-y": {Server: "https://shared.example.com:6443"},
			"cluster-z": {Server: "https://shared.example.com:6443"},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"user-1": {Token: "tok-1"},
			"user-2": {Token: "tok-2"},
			"user-3": {Token: "tok-3"},
		},
	})

	_, err := BuildClientConfig("https://shared.example.com:6443", "", "", fn.Local{})
	if err == nil {
		t.Fatal("expected error for multiple matching contexts")
	}
}

func TestBuildClientConfig_URLResolution_NoMatch(t *testing.T) {
	writeTestKubeconfig(t, multiContextKubeconfig())

	_, err := BuildClientConfig("https://nonexistent.example.com:6443", "", "", fn.Local{})
	if err == nil {
		t.Fatal("expected error for no matching context")
	}
}

// --- YAML round-trip tests for []byte fields ---

func TestAuthEntry_YAMLRoundTrip(t *testing.T) {
	caB64 := base64.StdEncoding.EncodeToString([]byte("CA-DATA"))
	certB64 := base64.StdEncoding.EncodeToString([]byte("CLIENT-CERT"))
	keyB64 := base64.StdEncoding.EncodeToString([]byte("CLIENT-KEY"))

	original := fn.Local{
		Auth: []fn.AuthEntry{{
			ClusterURL: "https://roundtrip.example.com",
			Cluster: fn.ClusterTLS{
				CertificateAuthorityData: caB64,
				InsecureSkipTLSVerify:    true,
			},
			User: fn.UserAuth{
				ClientCertificateData: certB64,
				ClientKeyData:         keyB64,
				Token:                 "my-token",
			},
		}},
	}

	// Marshal to YAML
	data, err := yaml.Marshal(&original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Verify the base64 string is present in YAML output
	yamlStr := string(data)
	if !contains(yamlStr, caB64) {
		t.Fatalf("expected base64 CA data in YAML output, got:\n%s", yamlStr)
	}

	// Unmarshal back
	var restored fn.Local
	if err := yaml.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(restored.Auth) != 1 {
		t.Fatalf("expected 1 auth entry, got %d", len(restored.Auth))
	}
	entry := restored.Auth[0]

	if entry.ClusterURL != "https://roundtrip.example.com" {
		t.Fatalf("ClusterURL mismatch: %s", entry.ClusterURL)
	}
	if entry.Cluster.CertificateAuthorityData != caB64 {
		t.Fatalf("CertificateAuthorityData mismatch: got %q", entry.Cluster.CertificateAuthorityData)
	}
	if !entry.Cluster.InsecureSkipTLSVerify {
		t.Fatal("expected InsecureSkipTLSVerify=true")
	}
	if entry.User.ClientCertificateData != certB64 {
		t.Fatalf("ClientCertificateData mismatch: got %q", entry.User.ClientCertificateData)
	}
	if entry.User.ClientKeyData != keyB64 {
		t.Fatalf("ClientKeyData mismatch: got %q", entry.User.ClientKeyData)
	}
	if entry.User.Token != "my-token" {
		t.Fatalf("Token mismatch: got %s", entry.User.Token)
	}
}

func TestAuthEntry_YAMLRoundTrip_WithFilePaths(t *testing.T) {
	original := fn.Local{
		Auth: []fn.AuthEntry{{
			ClusterURL: "https://paths.example.com",
			Cluster: fn.ClusterTLS{
				CertificateAuthority: "/path/to/ca.crt",
			},
			User: fn.UserAuth{
				ClientCertificate: "/path/to/client.crt",
				ClientKey:         "/path/to/client.key",
				Token:             "path-token",
			},
		}},
	}

	data, err := yaml.Marshal(&original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var restored fn.Local
	if err := yaml.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	entry := restored.Auth[0]
	if entry.Cluster.CertificateAuthority != "/path/to/ca.crt" {
		t.Fatalf("CertificateAuthority mismatch: %s", entry.Cluster.CertificateAuthority)
	}
	if entry.User.ClientCertificate != "/path/to/client.crt" {
		t.Fatalf("ClientCertificate mismatch: %s", entry.User.ClientCertificate)
	}
	if entry.User.ClientKey != "/path/to/client.key" {
		t.Fatalf("ClientKey mismatch: %s", entry.User.ClientKey)
	}
}

// --- Kubeconfig-to-Local conversion test ---

func TestKubeconfigToLocal_CertDataRoundTrip(t *testing.T) {
	kubecfg := testKubeconfigWithCerts()
	writeTestKubeconfig(t, kubecfg)

	cc, _ := BuildClientConfig("", "", "", fn.Local{})
	kc := NewClient(cc)

	// Extract into our types using Auth()
	url, clusterTLS, user, err := kc.Auth()
	if err != nil {
		t.Fatal(err)
	}

	// Verify the base64 decodes back to what we put in
	decoded, _ := base64.StdEncoding.DecodeString(clusterTLS.CertificateAuthorityData)
	if string(decoded) != "-----BEGIN CERTIFICATE-----\nCA-PEM\n-----END CERTIFICATE-----" {
		t.Fatalf("CA data mismatch: %q", decoded)
	}

	// Feed back into BuildClientConfig and verify it produces matching rest.Config
	local := fn.Local{}
	local.SetAuth(url, clusterTLS, user)
	cc2, _ := BuildClientConfig(url, "", "", local)
	restCfg, err := cc2.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}

	if restCfg.Host != url {
		t.Fatalf("host mismatch: got %s", restCfg.Host)
	}
	if string(restCfg.CAData) != "-----BEGIN CERTIFICATE-----\nCA-PEM\n-----END CERTIFICATE-----" {
		t.Fatalf("CAData mismatch after round-trip")
	}
	if string(restCfg.CertData) != "-----BEGIN CERTIFICATE-----\nCLIENT-CERT-PEM\n-----END CERTIFICATE-----" {
		t.Fatalf("CertData mismatch after round-trip")
	}
	if string(restCfg.KeyData) != "-----BEGIN RSA PRIVATE KEY-----\nCLIENT-KEY-PEM\n-----END RSA PRIVATE KEY-----" {
		t.Fatalf("KeyData mismatch after round-trip")
	}
}

// --- Auth() tests ---

func TestAuth_Token(t *testing.T) {
	writeTestKubeconfig(t, testKubeconfig())
	cc, _ := BuildClientConfig("", "", "", fn.Local{})
	kc := NewClient(cc)

	url, clusterTLS, user, err := kc.Auth()
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://kubeconfig.example.com:6443" {
		t.Fatalf("expected cluster URL, got %s", url)
	}
	if user.Token != "kubeconfig-token" {
		t.Fatalf("expected token, got %s", user.Token)
	}
	decoded := decodeBase64(clusterTLS.CertificateAuthorityData)
	if string(decoded) != "-----BEGIN CERTIFICATE-----\nCA-DATA\n-----END CERTIFICATE-----" {
		t.Fatalf("unexpected CA data: %s", decoded)
	}
}

func TestAuth_ClientCerts(t *testing.T) {
	writeTestKubeconfig(t, testKubeconfigWithCerts())
	cc, _ := BuildClientConfig("", "", "", fn.Local{})
	kc := NewClient(cc)

	url, _, user, err := kc.Auth()
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://cert.example.com:6443" {
		t.Fatalf("unexpected URL: %s", url)
	}

	certDecoded := decodeBase64(user.ClientCertificateData)
	if string(certDecoded) != "-----BEGIN CERTIFICATE-----\nCLIENT-CERT-PEM\n-----END CERTIFICATE-----" {
		t.Fatalf("unexpected ClientCertificateData: %s", certDecoded)
	}
	keyDecoded := decodeBase64(user.ClientKeyData)
	if string(keyDecoded) != "-----BEGIN RSA PRIVATE KEY-----\nCLIENT-KEY-PEM\n-----END RSA PRIVATE KEY-----" {
		t.Fatalf("unexpected ClientKeyData: %s", keyDecoded)
	}
}

func TestAuth_RoundTrip(t *testing.T) {
	writeTestKubeconfig(t, testKubeconfigWithCerts())
	cc, _ := BuildClientConfig("", "", "", fn.Local{})
	kc := NewClient(cc)

	url, clusterTLS, user, err := kc.Auth()
	if err != nil {
		t.Fatal(err)
	}

	// Store in Local and rebuild client
	local := fn.Local{}
	local.SetAuth(url, clusterTLS, user)
	cc2, _ := BuildClientConfig(url, "", "", local)
	kc2 := NewClient(cc2)
	restCfg, err := kc2.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}

	if restCfg.Host != "https://cert.example.com:6443" {
		t.Fatalf("host mismatch: %s", restCfg.Host)
	}
	if string(restCfg.CertData) != "-----BEGIN CERTIFICATE-----\nCLIENT-CERT-PEM\n-----END CERTIFICATE-----" {
		t.Fatalf("CertData mismatch after round-trip")
	}
	if string(restCfg.KeyData) != "-----BEGIN RSA PRIVATE KEY-----\nCLIENT-KEY-PEM\n-----END RSA PRIVATE KEY-----" {
		t.Fatalf("KeyData mismatch after round-trip")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && stringContains(s, substr)
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

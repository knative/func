package cmd_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"knative.dev/func/cmd"
	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/oci"
)

// -- envelope shape ---------------------------------------------------------

func TestWriteJSONSuccess_EnvelopeShape(t *testing.T) {
	var buf bytes.Buffer
	type payload struct {
		Name string `json:"name"`
	}
	if err := cmd.WriteJSONSuccess(&buf, payload{Name: "myfunc"}); err != nil {
		t.Fatalf("WriteJSONSuccess returned error: %v", err)
	}

	var resp cmd.JSONResponse
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("success envelope is not valid JSON: %v", err)
	}
	if resp.APIVersion != "v1" {
		t.Errorf("expected apiVersion 'v1', got %q", resp.APIVersion)
	}
	if resp.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", resp.Status)
	}
	if resp.Error != nil {
		t.Errorf("expected nil error in success envelope, got %+v", resp.Error)
	}
	if resp.Data == nil {
		t.Error("expected non-nil data in success envelope")
	}
}

func TestWriteJSONError_EnvelopeShape(t *testing.T) {
	var buf bytes.Buffer
	if err := cmd.WriteJSONError(&buf, errors.New("something broke")); err != nil {
		t.Fatalf("WriteJSONError returned error: %v", err)
	}

	var resp cmd.JSONResponse
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("error envelope is not valid JSON: %v", err)
	}
	if resp.APIVersion != "v1" {
		t.Errorf("expected apiVersion 'v1', got %q", resp.APIVersion)
	}
	if resp.Status != "error" {
		t.Errorf("expected status 'error', got %q", resp.Status)
	}
	if resp.Error == nil {
		t.Fatal("expected non-nil error field in error envelope")
	}
	if resp.Data != nil {
		t.Errorf("expected nil data in error envelope, got %v", resp.Data)
	}
}

func TestAPIVersionAlwaysPresent(t *testing.T) {
	tests := []struct {
		name string
		fn   func(*bytes.Buffer) error
	}{
		{"success", func(b *bytes.Buffer) error { return cmd.WriteJSONSuccess(b, "data") }},
		{"error", func(b *bytes.Buffer) error { return cmd.WriteJSONError(b, errors.New("err")) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := tc.fn(&buf); err != nil {
				t.Fatal(err)
			}
			var resp cmd.JSONResponse
			if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
				t.Fatalf("not valid JSON: %v", err)
			}
			if resp.APIVersion != "v1" {
				t.Errorf("apiVersion: want 'v1' got %q", resp.APIVersion)
			}
		})
	}
}

// -- errorToJSONError category/code mapping ---------------------------------

func TestErrorToJSONError_ClusterNotAccessible(t *testing.T) {
	inner := errors.New("dial tcp: connection refused")
	err := cmd.NewErrClusterNotAccessible(inner)
	assertJSONError(t, err, "CLUSTER_ERROR", "CLUSTER_NOT_ACCESSIBLE", true)
}

func TestErrorToJSONError_InvalidKubeconfig(t *testing.T) {
	inner := errors.New("kubeconfig not found")
	err := cmd.NewErrInvalidKubeconfig(inner)
	assertJSONError(t, err, "CLUSTER_ERROR", "INVALID_KUBECONFIG", false)
}

func TestErrorToJSONError_RegistryRequiredCLI(t *testing.T) {
	inner := fn.ErrRegistryRequired
	err := cmd.NewErrRegistryRequired(inner, "build")
	assertJSONError(t, err, "VALIDATION_ERROR", "REGISTRY_REQUIRED", false)
}

func TestErrorToJSONError_RegistryRequiredCore(t *testing.T) {
	assertJSONError(t, fn.ErrRegistryRequired, "VALIDATION_ERROR", "REGISTRY_REQUIRED", false)
}

func TestErrorToJSONError_ConflictImageRegistry(t *testing.T) {
	inner := fn.ErrConflictingImageAndRegistry
	err := cmd.NewErrConflictImageRegistry(inner, "build")
	assertJSONError(t, err, "VALIDATION_ERROR", "CONFLICTING_IMAGE_REGISTRY", false)
}

func TestErrorToJSONError_InvalidNamespace(t *testing.T) {
	inner := fn.ErrInvalidNamespace
	err := cmd.NewErrInvalidNamespace(inner, "deploy")
	assertJSONError(t, err, "VALIDATION_ERROR", "INVALID_NAMESPACE", false)
}

func TestErrorToJSONError_InvalidDomain(t *testing.T) {
	inner := fn.ErrInvalidDomain
	err := cmd.NewErrInvalidDomain(inner, "deploy")
	assertJSONError(t, err, "VALIDATION_ERROR", "INVALID_DOMAIN", false)
}

func TestErrorToJSONError_PlatformNotSupported(t *testing.T) {
	inner := fn.ErrPlatformNotSupported
	err := cmd.NewErrPlatformNotSupported(inner, "build")
	assertJSONError(t, err, "VALIDATION_ERROR", "PLATFORM_NOT_SUPPORTED", false)
}

func TestErrorToJSONError_NotInitializedCLI(t *testing.T) {
	inner := fn.NewErrNotInitialized("/path")
	err := cmd.NewErrNotInitialized(inner, "deploy")
	assertJSONError(t, err, "VALIDATION_ERROR", "NOT_INITIALIZED", false)
}

func TestErrorToJSONError_NotInitializedCore(t *testing.T) {
	err := fn.NewErrNotInitialized("/some/path")
	assertJSONError(t, err, "VALIDATION_ERROR", "NOT_INITIALIZED", false)
}

func TestErrorToJSONError_DeleteNameRequired(t *testing.T) {
	err := cmd.NewErrDeleteNameRequired(fn.ErrNameRequired)
	assertJSONError(t, err, "VALIDATION_ERROR", "NAME_REQUIRED", false)
}

func TestErrorToJSONError_DeleteNamespaceRequired(t *testing.T) {
	err := cmd.NewErrDeleteNamespaceRequired(fn.ErrNamespaceRequired)
	assertJSONError(t, err, "VALIDATION_ERROR", "NAMESPACE_REQUIRED", false)
}

func TestErrorToJSONError_NotBuilt(t *testing.T) {
	assertJSONError(t, fn.ErrNotBuilt, "BUILD_ERROR", "NOT_BUILT", false)
}

func TestErrorToJSONError_BuildFailed(t *testing.T) {
	err := oci.BuildErr{Err: errors.New("build failed")}
	assertJSONError(t, err, "BUILD_ERROR", "BUILD_FAILED", true)
}

func TestErrorToJSONError_Unknown(t *testing.T) {
	assertJSONError(t, errors.New("totally unknown error"), "UNKNOWN_ERROR", "UNKNOWN", false)
}

func TestErrorToJSONError_MessagePreserved(t *testing.T) {
	msg := "my specific error message"
	var buf bytes.Buffer
	if err := cmd.WriteJSONError(&buf, errors.New(msg)); err != nil {
		t.Fatal(err)
	}
	var resp cmd.JSONResponse
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error.Message != msg {
		t.Errorf("expected message %q, got %q", msg, resp.Error.Message)
	}
}

// -- round-trip validity ----------------------------------------------------

func TestWriteJSONSuccess_RoundTrip(t *testing.T) {
	type payload struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}
	p := payload{Name: "myfunc", Namespace: "prod"}

	var buf bytes.Buffer
	if err := cmd.WriteJSONSuccess(&buf, p); err != nil {
		t.Fatal(err)
	}

	var resp struct {
		APIVersion string  `json:"apiVersion"`
		Status     string  `json:"status"`
		Data       payload `json:"data"`
	}
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("round-trip decode failed: %v", err)
	}
	if resp.Data.Name != p.Name {
		t.Errorf("round-trip name: want %q got %q", p.Name, resp.Data.Name)
	}
	if resp.Data.Namespace != p.Namespace {
		t.Errorf("round-trip namespace: want %q got %q", p.Namespace, resp.Data.Namespace)
	}
	if resp.APIVersion != "v1" {
		t.Errorf("round-trip apiVersion: want 'v1' got %q", resp.APIVersion)
	}
	if resp.Status != "ok" {
		t.Errorf("round-trip status: want 'ok' got %q", resp.Status)
	}
}

// -- helpers ----------------------------------------------------------------

// assertJSONError writes the error as a JSON envelope and checks category/code/retryable.
func assertJSONError(t *testing.T, err error, wantCategory, wantCode string, wantRetryable bool) {
	t.Helper()
	var buf bytes.Buffer
	if werr := cmd.WriteJSONError(&buf, err); werr != nil {
		t.Fatalf("WriteJSONError returned error: %v", werr)
	}
	var resp cmd.JSONResponse
	if derr := json.Unmarshal(buf.Bytes(), &resp); derr != nil {
		t.Fatalf("result is not valid JSON: %v", derr)
	}
	if resp.Error == nil {
		t.Fatal("expected non-nil error field")
	}
	if resp.Error.Category != wantCategory {
		t.Errorf("category: want %q got %q", wantCategory, resp.Error.Category)
	}
	if resp.Error.Code != wantCode {
		t.Errorf("code: want %q got %q", wantCode, resp.Error.Code)
	}
	if resp.Error.Retryable != wantRetryable {
		t.Errorf("retryable: want %v got %v", wantRetryable, resp.Error.Retryable)
	}
}

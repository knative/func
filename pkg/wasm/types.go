package wasm

import (
	wasmv1alpha1 "github.com/cardil/knative-serving-wasm/pkg/apis/wasm/v1alpha1"
)

// Type aliases for WASI network configuration from knative-serving-wasm.
// These map directly to the WasmModule CRD spec.network field.

// WasiNetworkConfig specifies WASI network permissions for a WASM module.
// Alias for NetworkSpec from knative-serving-wasm.
type WasiNetworkConfig = wasmv1alpha1.NetworkSpec

// WasiTCPConfig specifies TCP socket permissions for WASI.
// Alias for TCPSpec from knative-serving-wasm.
type WasiTCPConfig = wasmv1alpha1.TCPSpec

// WasiUDPConfig specifies UDP socket permissions for WASI.
// Alias for UDPSpec from knative-serving-wasm.
type WasiUDPConfig = wasmv1alpha1.UDPSpec

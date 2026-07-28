// Copyright Contributors to the Open Cluster Management project

package config

import (
	"context"
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func newFakeAPIServer(tlsProfile map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "config.openshift.io/v1",
			"kind":       "APIServer",
			"metadata": map[string]interface{}{
				"name": "cluster",
			},
			"spec": map[string]interface{}{},
		},
	}
	if tlsProfile != nil {
		obj.Object["spec"].(map[string]interface{})["tlsSecurityProfile"] = tlsProfile
	}
	return obj
}

func newFakeDynamicClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	s := runtime.NewScheme()
	s.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "config.openshift.io", Version: "v1", Kind: "APIServer"},
		&unstructured.Unstructured{},
	)
	s.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "config.openshift.io", Version: "v1", Kind: "APIServerList"},
		&unstructured.UnstructuredList{},
	)
	return dynamicfake.NewSimpleDynamicClient(s, objects...)
}

func TestGetTLSConfig_NoAPIServer(t *testing.T) {
	client := newFakeDynamicClient()

	cfg, _, ok := GetTLSConfig(context.TODO(), client)

	require.NotNil(t, cfg, "expected non-nil config on Intermediate fallback")
	assert.False(t, ok, "snapshot should be invalid on fallback")
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion, "expected TLS 1.2 fallback")
	assert.NotEmpty(t, cfg.CipherSuites, "expected cipher suites from Intermediate profile")
}

func TestGetTLSConfig_NoProfile(t *testing.T) {
	apiServer := newFakeAPIServer(nil)
	client := newFakeDynamicClient(apiServer)

	cfg, _, ok := GetTLSConfig(context.TODO(), client)

	require.NotNil(t, cfg, "expected non-nil config for default profile")
	assert.True(t, ok, "snapshot should be valid")
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion, "expected TLS 1.2")
	assert.NotEmpty(t, cfg.CipherSuites, "expected cipher suites")
}

func TestGetTLSConfig_IntermediateProfile(t *testing.T) {
	apiServer := newFakeAPIServer(map[string]interface{}{
		"type": "Intermediate",
	})
	client := newFakeDynamicClient(apiServer)

	cfg, snapshot, ok := GetTLSConfig(context.TODO(), client)

	require.NotNil(t, cfg, "expected non-nil config")
	assert.True(t, ok, "snapshot should be valid")
	assert.NotNil(t, snapshot, "expected non-nil snapshot for explicit profile")
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion, "expected TLS 1.2")
	assert.NotEmpty(t, cfg.CipherSuites, "expected cipher suites")
}

func TestGetTLSConfig_OldProfile(t *testing.T) {
	apiServer := newFakeAPIServer(map[string]interface{}{
		"type": "Old",
	})
	client := newFakeDynamicClient(apiServer)

	cfg, _, ok := GetTLSConfig(context.TODO(), client)

	require.NotNil(t, cfg, "expected non-nil config")
	assert.True(t, ok, "snapshot should be valid")
	assert.Equal(t, uint16(tls.VersionTLS10), cfg.MinVersion, "expected TLS 1.0")
}

func TestGetTLSConfig_ModernProfile(t *testing.T) {
	apiServer := newFakeAPIServer(map[string]interface{}{
		"type": "Modern",
	})
	client := newFakeDynamicClient(apiServer)

	cfg, _, ok := GetTLSConfig(context.TODO(), client)

	require.NotNil(t, cfg, "expected non-nil config")
	assert.True(t, ok, "snapshot should be valid")
	assert.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion, "expected TLS 1.3")
}

func TestGetTLSConfig_CustomProfile(t *testing.T) {
	apiServer := newFakeAPIServer(map[string]interface{}{
		"type": "Custom",
		"custom": map[string]interface{}{
			"ciphers":       []interface{}{"ECDHE-RSA-AES256-GCM-SHA384"},
			"minTLSVersion": "VersionTLS13",
		},
	})
	client := newFakeDynamicClient(apiServer)

	cfg, _, ok := GetTLSConfig(context.TODO(), client)

	require.NotNil(t, cfg, "expected non-nil config")
	assert.True(t, ok, "snapshot should be valid")
	assert.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion, "expected TLS 1.3")
}

func TestIntermediateProfileTLSConfig(t *testing.T) {
	cfg := intermediateProfileTLSConfig()

	require.NotNil(t, cfg, "expected non-nil config")
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion, "expected TLS 1.2")
	assert.NotEmpty(t, cfg.CipherSuites, "expected cipher suites")
}

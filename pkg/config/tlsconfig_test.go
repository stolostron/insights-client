// Copyright Contributors to the Open Cluster Management project

package config

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/assert"
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

	cfg := GetTLSConfig(client)

	// Falls back to Intermediate profile
	assert.NotNil(t, cfg)
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	assert.NotEmpty(t, cfg.CipherSuites)
}

func TestGetTLSConfig_NoProfile(t *testing.T) {
	apiServer := newFakeAPIServer(nil)
	client := newFakeDynamicClient(apiServer)

	cfg := GetTLSConfig(client)

	assert.NotNil(t, cfg)
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	assert.NotEmpty(t, cfg.CipherSuites)
}

func TestGetTLSConfig_IntermediateProfile(t *testing.T) {
	apiServer := newFakeAPIServer(map[string]interface{}{
		"type": "Intermediate",
	})
	client := newFakeDynamicClient(apiServer)

	cfg := GetTLSConfig(client)

	assert.NotNil(t, cfg)
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	assert.NotEmpty(t, cfg.CipherSuites)
}

func TestGetTLSConfig_OldProfile(t *testing.T) {
	apiServer := newFakeAPIServer(map[string]interface{}{
		"type": "Old",
	})
	client := newFakeDynamicClient(apiServer)

	cfg := GetTLSConfig(client)

	assert.NotNil(t, cfg)
	assert.Equal(t, uint16(tls.VersionTLS10), cfg.MinVersion)
}

func TestGetTLSConfig_ModernProfile(t *testing.T) {
	apiServer := newFakeAPIServer(map[string]interface{}{
		"type": "Modern",
	})
	client := newFakeDynamicClient(apiServer)

	cfg := GetTLSConfig(client)

	assert.NotNil(t, cfg)
	assert.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
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

	cfg := GetTLSConfig(client)

	assert.NotNil(t, cfg)
	assert.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
}

func TestCipherSuitesFromNames(t *testing.T) {
	names := "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384"
	result := cipherSuitesFromNames(names)

	assert.Len(t, result, 2)
	assert.Equal(t, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, result[0])
	assert.Equal(t, tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384, result[1])
}

func TestCipherSuitesFromNames_Empty(t *testing.T) {
	result := cipherSuitesFromNames("")
	assert.Nil(t, result)
}

func TestCipherSuitesFromNames_UnknownSkipped(t *testing.T) {
	names := "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,BOGUS_CIPHER"
	result := cipherSuitesFromNames(names)

	assert.Len(t, result, 1)
	assert.Equal(t, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, result[0])
}

func TestIntermediateProfileTLSConfig(t *testing.T) {
	cfg := intermediateProfileTLSConfig()

	assert.NotNil(t, cfg)
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	assert.NotEmpty(t, cfg.CipherSuites)
}

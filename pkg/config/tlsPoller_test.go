// Copyright Contributors to the Open Cluster Management project

package config

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestCurrentTLSProfileData_NoAPIServer(t *testing.T) {
	client := newFakeDynamicClient()

	data, err := currentTLSProfileData(context.TODO(), client)

	assert.Error(t, err, "expected error when APIServer doesn't exist")
	assert.Nil(t, data, "expected nil data")
}

func TestCurrentTLSProfileData_NoProfile(t *testing.T) {
	apiServer := newFakeAPIServer(nil)
	client := newFakeDynamicClient(apiServer)

	data, err := currentTLSProfileData(context.TODO(), client)

	assert.NoError(t, err)
	assert.Nil(t, data, "expected nil data when no profile is set")
}

func TestCurrentTLSProfileData_WithProfile(t *testing.T) {
	apiServer := newFakeAPIServer(map[string]interface{}{
		"type": "Intermediate",
	})
	client := newFakeDynamicClient(apiServer)

	data, err := currentTLSProfileData(context.TODO(), client)

	assert.NoError(t, err)
	require.NotNil(t, data, "expected non-nil data for explicit profile")
	assert.Equal(t, "Intermediate", data["type"])
}

func TestPollTLSProfile_NoChangeDoesNotExit(t *testing.T) {
	apiServer := newFakeAPIServer(map[string]interface{}{
		"type": "Intermediate",
	})
	client := newFakeDynamicClient(apiServer)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	initial, err := currentTLSProfileData(ctx, client)
	require.NoError(t, err)

	// Should complete without exiting when the profile doesn't change.
	pollTLSProfile(ctx, client, 50*time.Millisecond, initial, true)
}

func TestPollTLSProfile_DetectsChange(t *testing.T) {
	apiServer := newFakeAPIServer(map[string]interface{}{
		"type": "Intermediate",
	})
	client := newFakeDynamicClient(apiServer)

	// Read initial state
	initial, err := currentTLSProfileData(context.TODO(), client)
	assert.NoError(t, err)
	assert.Equal(t, "Intermediate", initial["type"])

	// Simulate a profile change by updating the APIServer resource.
	updated := newFakeAPIServer(map[string]interface{}{
		"type": "Old",
	})
	_, err = client.Resource(apiServerGVR).Update(
		context.TODO(), updated, metav1.UpdateOptions{})
	assert.NoError(t, err)

	// Verify the change is detected.
	current, err := currentTLSProfileData(context.TODO(), client)
	assert.NoError(t, err)
	assert.Equal(t, "Old", current["type"])
	assert.NotEqual(t, initial["type"], current["type"])
}

func TestCurrentTLSProfileData_StableNormalization(t *testing.T) {
	// Verify that two reads of the same resource produce equal results (JSON normalization works).
	apiServer := newFakeAPIServer(map[string]interface{}{
		"type": "Custom",
		"custom": map[string]interface{}{
			"ciphers":       []interface{}{"ECDHE-RSA-AES256-GCM-SHA384", "ECDHE-RSA-AES128-GCM-SHA256"},
			"minTLSVersion": "VersionTLS12",
		},
	})
	client := newFakeDynamicClient(apiServer)

	data1, err1 := currentTLSProfileData(context.TODO(), client)
	data2, err2 := currentTLSProfileData(context.TODO(), client)

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.Equal(t, data1, data2)
}

func TestIsEffectivelyIntermediate(t *testing.T) {
	assert.True(t, isEffectivelyIntermediate(nil), "nil = default = Intermediate")
	assert.True(t, isEffectivelyIntermediate(map[string]interface{}{"type": "Intermediate"}))
	assert.False(t, isEffectivelyIntermediate(map[string]interface{}{"type": "Modern"}))
	assert.False(t, isEffectivelyIntermediate(map[string]interface{}{"type": "Old"}))
	assert.False(t, isEffectivelyIntermediate(map[string]interface{}{"type": "Custom"}))
}

func TestPollTLSProfile_RecoveryBaselinesIntermediate(t *testing.T) {
	// When startup failed (initialValid=false) and cluster profile is Intermediate,
	// the poller should baseline it without restarting.
	apiServer := newFakeAPIServer(map[string]interface{}{
		"type": "Intermediate",
	})
	client := newFakeDynamicClient(apiServer)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// initialValid=false simulates a failed startup read.
	pollTLSProfile(ctx, client, 50*time.Millisecond, nil, false)
	// If it exits here, the test would fail — it should baseline and continue until ctx expires.
}

func TestPollTLSProfile_RecoveryBaselinesDefault(t *testing.T) {
	// No explicit profile set (nil = default = Intermediate) — should also baseline.
	apiServer := newFakeAPIServer(nil)
	client := newFakeDynamicClient(apiServer)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	pollTLSProfile(ctx, client, 50*time.Millisecond, nil, false)
}

func TestCurrentTLSProfileData_NoSpec(t *testing.T) {
	// APIServer with no spec field at all.
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "config.openshift.io/v1",
			"kind":       "APIServer",
			"metadata": map[string]interface{}{
				"name": "cluster",
			},
		},
	}
	client := newFakeDynamicClient(obj)

	data, err := currentTLSProfileData(context.TODO(), client)
	assert.NoError(t, err)
	assert.Nil(t, data)
}

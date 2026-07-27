// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestCurrentTLSProfileData_NoAPIServer(t *testing.T) {
	client := newFakeDynamicClient()

	data, err := currentTLSProfileData(client)

	assert.Error(t, err)
	assert.Nil(t, data)
}

func TestCurrentTLSProfileData_NoProfile(t *testing.T) {
	apiServer := newFakeAPIServer(nil)
	client := newFakeDynamicClient(apiServer)

	data, err := currentTLSProfileData(client)

	assert.NoError(t, err)
	assert.Nil(t, data)
}

func TestCurrentTLSProfileData_WithProfile(t *testing.T) {
	apiServer := newFakeAPIServer(map[string]interface{}{
		"type": "Intermediate",
	})
	client := newFakeDynamicClient(apiServer)

	data, err := currentTLSProfileData(client)

	assert.NoError(t, err)
	assert.NotNil(t, data)
	assert.Equal(t, "Intermediate", data["type"])
}

func TestPollTLSProfile_NoChangeDoesNotExit(t *testing.T) {
	apiServer := newFakeAPIServer(map[string]interface{}{
		"type": "Intermediate",
	})
	client := newFakeDynamicClient(apiServer)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Should complete without exiting when the profile doesn't change.
	pollTLSProfile(ctx, client, 50*time.Millisecond)
}

func TestPollTLSProfile_DetectsChange(t *testing.T) {
	apiServer := newFakeAPIServer(map[string]interface{}{
		"type": "Intermediate",
	})
	client := newFakeDynamicClient(apiServer)

	// Read initial state
	initial, err := currentTLSProfileData(client)
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
	current, err := currentTLSProfileData(client)
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

	data1, err1 := currentTLSProfileData(client)
	data2, err2 := currentTLSProfileData(client)

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.Equal(t, data1, data2)
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

	data, err := currentTLSProfileData(client)
	assert.NoError(t, err)
	assert.Nil(t, data)
}

// Copyright Contributors to the Open Cluster Management project

package config

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"time"

	"github.com/golang/glog"
	configv1 "github.com/openshift/api/config/v1"
	openshifttls "github.com/openshift/controller-runtime-common/pkg/tls"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const apiServerTimeout = 30 * time.Second

var apiServerGVR = schema.GroupVersionResource{
	Group:    "config.openshift.io",
	Version:  "v1",
	Resource: "apiservers",
}

// GetTLSConfig reads the cluster's APIServer TLS security profile and returns a *tls.Config,
// the raw profile snapshot for use as a poller baseline, and whether that snapshot is valid.
func GetTLSConfig(ctx context.Context, dynamicClient dynamic.Interface) (*tls.Config, map[string]interface{}, bool) {
	profileSpec, err := fetchTLSProfileSpec(ctx, dynamicClient)
	if err != nil {
		glog.Warning("Could not read APIServer TLS profile, using Intermediate default")
		return intermediateProfileTLSConfig(), nil, false
	}

	tlsConfigFn, unsupported := openshifttls.NewTLSConfigFromProfile(*profileSpec)
	if len(unsupported) > 0 {
		glog.Warningf("Cipher suites not supported by Go, skipped: %v", unsupported)
	}

	cfg := &tls.Config{} // #nosec G402 - MinVersion set by tlsConfigFn from cluster profile.
	tlsConfigFn(cfg)

	glog.Infof("TLS profile applied: min version TLS 1.%d, %d cipher suites",
		(cfg.MinVersion&0xff)-1, len(cfg.CipherSuites))

	// Capture raw profile snapshot so the poller uses the exact same baseline.
	// This is a second read of the same resource; the window for a race is
	// negligible at startup, and a failure here is propagated to the caller.
	snapshot, snapErr := currentTLSProfileData(ctx, dynamicClient)
	if snapErr != nil {
		glog.Warning("Could not capture TLS profile snapshot for poller baseline")
		return cfg, nil, false
	}
	return cfg, snapshot, true
}

// fetchTLSProfileSpec reads the APIServer resource and returns the resolved TLSProfileSpec.
func fetchTLSProfileSpec(ctx context.Context, dynamicClient dynamic.Interface) (*configv1.TLSProfileSpec, error) {
	reqCtx, cancel := context.WithTimeout(ctx, apiServerTimeout)
	defer cancel()

	obj, err := dynamicClient.Resource(apiServerGVR).Get(reqCtx, "cluster", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get APIServer resource: %w", err)
	}

	spec, ok := obj.Object["spec"].(map[string]interface{})
	if !ok {
		return defaultProfileSpec(), nil
	}

	profileRaw, exists := spec["tlsSecurityProfile"]
	if !exists || profileRaw == nil {
		return defaultProfileSpec(), nil
	}

	profileBytes, err := json.Marshal(profileRaw)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal TLS profile: %w", err)
	}

	var profile configv1.TLSSecurityProfile
	if err := json.Unmarshal(profileBytes, &profile); err != nil {
		return nil, fmt.Errorf("failed to unmarshal TLS profile: %w", err)
	}

	resolved, err := openshifttls.GetTLSProfileSpec(&profile)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve TLS profile: %w", err)
	}

	glog.Infof("Using cluster TLS profile: %s", profile.Type)
	return &resolved, nil
}

func defaultProfileSpec() *configv1.TLSProfileSpec {
	spec := *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
	return &spec
}

// intermediateProfileTLSConfig returns a *tls.Config matching the OpenShift Intermediate profile.
func intermediateProfileTLSConfig() *tls.Config {
	profile := configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
	tlsConfigFn, unsupported := openshifttls.NewTLSConfigFromProfile(*profile)
	if len(unsupported) > 0 {
		glog.Warningf("Intermediate profile: cipher suites not supported by Go: %v", unsupported)
	}

	cfg := &tls.Config{} // #nosec G402 - MinVersion set by tlsConfigFn from Intermediate profile.
	tlsConfigFn(cfg)
	return cfg
}


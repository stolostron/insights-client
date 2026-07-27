// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"time"

	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

const tlsPollInterval = 60 * time.Second

// PollAPIServerTLSProfile polls the APIServer resource and exits the process if the TLS
// security profile changes. The Deployment controller restarts the pod, which picks up
// the new profile on startup. This avoids the complexity of live TLS config swapping.
func PollAPIServerTLSProfile(ctx context.Context, dynamicClient dynamic.Interface) {
	pollTLSProfile(ctx, dynamicClient, tlsPollInterval)
}

// pollTLSProfile is the testable core of PollAPIServerTLSProfile.
func pollTLSProfile(ctx context.Context, dynamicClient dynamic.Interface, interval time.Duration) {
	initial, err := currentTLSProfileData(dynamicClient)
	initialValid := err == nil
	if !initialValid {
		glog.Warningf("Could not read initial APIServer TLS profile, will keep polling: %v", err)
	}

	glog.Infof("TLS profile poller started, polling every %s", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			glog.Info("TLS profile poller stopped")
			return
		case <-ticker.C:
			current, err := currentTLSProfileData(dynamicClient)
			if err != nil {
				glog.Warningf("Error polling APIServer TLS profile: %v", err)
				continue
			}
			if !initialValid {
				// First successful read — capture baseline instead of restarting.
				initial = current
				initialValid = true
				glog.Info("Captured initial TLS profile baseline")
				continue
			}
			if !reflect.DeepEqual(initial, current) {
				glog.Info("APIServer TLS profile changed, restarting to apply new config")
				os.Exit(1)
			}
		}
	}
}

// currentTLSProfileData reads the raw tlsSecurityProfile from the APIServer resource.
// Returns nil (not an error) when the field is absent, representing the default profile.
func currentTLSProfileData(dynamicClient dynamic.Interface) (map[string]interface{}, error) {
	obj, err := dynamicClient.Resource(apiServerGVR).Get(context.TODO(), "cluster", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	spec, ok := obj.Object["spec"].(map[string]interface{})
	if !ok {
		return nil, nil
	}

	profileRaw, exists := spec["tlsSecurityProfile"]
	if !exists || profileRaw == nil {
		return nil, nil
	}

	// Normalize through JSON round-trip for stable deep-equal comparison.
	profileBytes, err := json.Marshal(profileRaw)
	if err != nil {
		return nil, err
	}

	var normalized map[string]interface{}
	if err := json.Unmarshal(profileBytes, &normalized); err != nil {
		return nil, err
	}

	return normalized, nil
}

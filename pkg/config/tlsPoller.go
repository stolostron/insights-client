// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (
	"context"
	"encoding/json"
	"fmt"
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
// the new profile on startup.
func PollAPIServerTLSProfile(ctx context.Context, dynamicClient dynamic.Interface,
	initialProfile map[string]interface{}, initialValid bool) {
	pollTLSProfile(ctx, dynamicClient, tlsPollInterval, initialProfile, initialValid)
}

// pollTLSProfile is the testable core of PollAPIServerTLSProfile.
func pollTLSProfile(ctx context.Context, dynamicClient dynamic.Interface, interval time.Duration,
	initial map[string]interface{}, initialValid bool) {
	if !initialValid {
		glog.Warning("No valid initial TLS profile baseline, will capture on first successful poll")
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
			current, err := currentTLSProfileData(ctx, dynamicClient)
			if err != nil {
				glog.Warning("Error polling APIServer TLS profile")
				continue
			}
			if !initialValid {
				// Startup read failed; pod is running with Intermediate fallback.
				// If the cluster profile is effectively Intermediate (nil = default,
				// or explicit Intermediate), we're already correct — baseline it.
				// Otherwise restart to apply the actual profile.
				if isEffectivelyIntermediate(current) {
					initial = current
					initialValid = true
					glog.Info("Cluster profile matches Intermediate fallback, baselined")
					continue
				}
				glog.Info("APIServer now reachable, cluster profile differs from Intermediate fallback, restarting")
				os.Exit(1)
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
func currentTLSProfileData(ctx context.Context, dynamicClient dynamic.Interface) (map[string]interface{}, error) {
	reqCtx, cancel := context.WithTimeout(ctx, apiServerTimeout)
	defer cancel()

	obj, err := dynamicClient.Resource(apiServerGVR).Get(reqCtx, "cluster", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get APIServer resource: %w", err)
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
		return nil, fmt.Errorf("failed to marshal TLS profile data: %w", err)
	}

	var normalized map[string]interface{}
	if err := json.Unmarshal(profileBytes, &normalized); err != nil {
		return nil, fmt.Errorf("failed to unmarshal TLS profile data: %w", err)
	}

	return normalized, nil
}

// isEffectivelyIntermediate returns true when the raw profile data represents
// the Intermediate profile (nil means default = Intermediate, or explicit type).
func isEffectivelyIntermediate(data map[string]interface{}) bool {
	if data == nil {
		return true // no explicit profile = default = Intermediate
	}
	profileType, _ := data["type"].(string)
	return profileType == "Intermediate"
}

// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	openshifttls "github.com/openshift/controller-runtime-common/pkg/tls"
	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var apiServerGVR = schema.GroupVersionResource{
	Group:    "config.openshift.io",
	Version:  "v1",
	Resource: "apiservers",
}

// GetTLSConfig reads the cluster's APIServer TLS security profile and returns a *tls.Config.
// Falls back to the OpenShift Intermediate profile if the APIServer resource is unavailable.
func GetTLSConfig(dynamicClient dynamic.Interface) *tls.Config {
	profileSpec, err := fetchTLSProfileSpec(dynamicClient)
	if err != nil {
		glog.Warningf("Could not read APIServer TLS profile, using Intermediate default: %v", err)
		return intermediateProfileTLSConfig()
	}

	tlsConfigFn, unsupported := openshifttls.NewTLSConfigFromProfile(*profileSpec)
	if len(unsupported) > 0 {
		glog.Warningf("Cipher suites not supported by Go, skipped: %v", unsupported)
	}

	cfg := &tls.Config{} // #nosec G402 - MinVersion set by tlsConfigFn from cluster profile.
	tlsConfigFn(cfg)

	glog.Infof("TLS profile applied: min version TLS 1.%d, %d cipher suites",
		(cfg.MinVersion&0xff)-1, len(cfg.CipherSuites))

	return cfg
}

// fetchTLSProfileSpec reads the APIServer resource and returns the resolved TLSProfileSpec.
func fetchTLSProfileSpec(dynamicClient dynamic.Interface) (*configv1.TLSProfileSpec, error) {
	obj, err := dynamicClient.Resource(apiServerGVR).Get(context.TODO(), "cluster", metav1.GetOptions{})
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

// cipherSuitesFromNames resolves IANA cipher suite names to crypto/tls uint16 IDs.
func cipherSuitesFromNames(ciphers string) []uint16 {
	if ciphers == "" {
		return nil
	}

	lookup := map[string]uint16{}
	for _, cs := range tls.CipherSuites() {
		lookup[cs.Name] = cs.ID
	}
	for _, cs := range tls.InsecureCipherSuites() {
		lookup[cs.Name] = cs.ID
	}

	var result []uint16
	var unknown []string
	for _, name := range strings.Split(ciphers, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if id, ok := lookup[name]; ok {
			result = append(result, id)
		} else {
			unknown = append(unknown, name)
		}
	}

	if len(unknown) > 0 {
		glog.Warningf("TLS cipher suites not recognized by Go: %v", unknown)
	}

	return result
}

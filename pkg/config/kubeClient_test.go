// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (
	"reflect"
	"testing"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// should build config from flags if kube config is not empty
func Test_GetConfig(t *testing.T) {
	// Establish the config
	SetupConfig()

	clientConfig := GetConfig()
	var fromFlags *rest.Config
	if Cfg.KubeConfig != "" {
		fromFlags, _ = clientcmd.BuildConfigFromFlags("", Cfg.KubeConfig)
	}
	fromRest, _ := rest.InClusterConfig()

	if Cfg.KubeConfig != "" && !reflect.DeepEqual(*clientConfig, *fromFlags) { // if config is not empty, then get config from flags
		t.Errorf("Failed testing GetConfig()  Expected: %s  Got: %s", fromFlags, clientConfig)
	}
	if Cfg.KubeConfig == "" && !reflect.DeepEqual(*clientConfig, *fromRest) { // if the config is empty, get using rest client
		t.Errorf("Failed testing GetConfig()  Expected: %s  Got: %s", fromRest, clientConfig)
	}

}

// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (

	"testing"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/rest"
)

// should build config from flags if kube config is not empty
func Test_GetConfig(t *testing.T) {

	clientConfig := GetConfig()
	fromFlags, _ := clientcmd.BuildConfigFromFlags("", Cfg.KubeConfig)
    fromRest, _ := rest.InClusterConfig()
	if Cfg.KubeConfig != "" && clientConfig {
		t.Errorf("Failed testing GetConfig()  1Expected: %s  Got: %s", fromRest, clientConfig)
	}
    if Cfg.KubeConfig == "" && clientConfig {
		t.Errorf("Failed testing GetConfig()  2Expected: %s  Got: %s", fromRest, clientConfig)
	}

}

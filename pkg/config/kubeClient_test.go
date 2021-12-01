// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (

    "reflect"
	"testing"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/rest"
)

// should build config from flags if kube config is not empty
func Test_GetConfig(t *testing.T) {

	clientConfig := GetConfig()
	var fromFlags;
	if cfg.kubeconfig != "" {
         fromFlags, _ := clientcmd.BuildConfigFromFlags("", Cfg.KubeConfig)
    }
	fromRest, _ := rest.InClusterConfig()

	if cfg.kubeconfig != "" && !reflect.DeepEqual(*clientConfig, *fromFlags) { // if config is not empty, then get config from flags
		t.Errorf("Failed testing GetConfig()  Expected: %s  Got: %s", fromFlags, clientConfig)
	}
    if Cfg.KubeConfig == "" && !reflect.DeepEqual(*clientConfig, *fromRest) { // if the config is empty, get using rest client
		t.Errorf("Failed testing GetConfig()  Expected: %s  Got: %s", fromRest, clientConfig)
	}

}

// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
package config

import (
	"sync"

	"github.com/golang/glog"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var clientSet *kubernetes.Clientset
var mutex sync.Mutex
var dynamicClient dynamic.Interface

func GetConfig() *rest.Config {
	var clientConfig *rest.Config
	if Cfg.KubeConfig != "" {
		glog.V(2).Infof("Creating k8s client using path: %s", Cfg.KubeConfig)
		clientConfig, _ = clientcmd.BuildConfigFromFlags("", Cfg.KubeConfig)
	} else {
		glog.V(2).Info("Creating k8s client using InClusterlientConfig()")
		clientConfig, _ = rest.InClusterConfig()
	}
	return clientConfig
}

// Get the kubernetes dynamic client.
func GetDynamicClient() dynamic.Interface {
	mutex.Lock()
	defer mutex.Unlock()
	if dynamicClient != nil {
		return dynamicClient
	}
	newDynamicClient, err := dynamic.NewForConfig(GetConfig())
	if err != nil {
		glog.Fatal("Cannot Construct Dynamic Client ", err)
	}
	dynamicClient = newDynamicClient

	return dynamicClient
}

func GetKubeClient() *kubernetes.Clientset {
	if clientSet != nil {
		return clientSet
	}
	clientSet, err := kubernetes.NewForConfig(GetConfig())
	if err != nil {
		glog.Fatal("Cannot Construct ClientSet ", err)
	}
	return clientSet
}

// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
package config

import (
	"github.com/golang/glog"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/wg-policy-prototypes/policy-report/api/v1alpha1"
)

var CRDGroup string = "wgpolicyk8s.io"
var CRDVersion string = "v1alpha1"
var SchemeGroupVersion = schema.GroupVersion{Group: CRDGroup, Version: CRDVersion}
var restClient *rest.RESTClient
var clientSet *kubernetes.Clientset

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&v1alpha1.PolicyReport{},
	)
	meta_v1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

func RESTClient(cfg *rest.Config) *rest.RESTClient {
	if restClient != nil {
		return restClient
	}
	scheme := runtime.NewScheme()
	SchemeBuilder := runtime.NewSchemeBuilder(addKnownTypes)
	if err := SchemeBuilder.AddToScheme(scheme); err != nil {
		glog.Warningf("Cannot add scheme %v", err)
		return nil
	}
	config := *cfg
	config.GroupVersion = &SchemeGroupVersion
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON
	config.NegotiatedSerializer = serializer.NewCodecFactory(scheme)
	restClient, err := rest.RESTClientFor(&config)
	if err != nil {
		glog.Warningf("Error creating RestClient %v", err)
		return nil
	}
	return restClient
}

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

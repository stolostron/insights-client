// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (
	"testing"
)

func TestGetKubeClient(t *testing.T) {
	if GetKubeClient() == nil {
		t.Fatal("GetKubeClient failed - Cannot get KubeClient")
	}
}

func TestRESTClient(t *testing.T) {
	if RESTClient(GetConfig()) == nil {
		t.Fatal("RESTClient failed - Cannot get RESTClient")
	}
}

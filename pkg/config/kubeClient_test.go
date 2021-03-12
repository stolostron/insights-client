// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (
	"testing"
)

func TestRESTClient(t *testing.T) {
	if RESTClient(GetConfig()) == nil {
		t.Fatal("RESTClient failed - Cannot get RESTClient")
	}
}

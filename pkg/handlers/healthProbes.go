// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
package handlers

import (
	"fmt"
	"net/http"

	"github.com/golang/glog"
	"github.com/open-cluster-management/insights-client/pkg/monitor"
	"github.com/open-cluster-management/insights-client/pkg/retriever"
)

// LivenessProbe is used to check if this service is alive.
func LivenessProbe(w http.ResponseWriter, r *http.Request) {
	glog.V(2).Info("livenessProbe - Checking local cluster id.")
	monitor := monitor.NewClusterMonitor()

	//Get local-cluster id , if -1 is returned then service will
	// not be able to get to cloud.redhat.com
	if monitor.GetLocalCluster() == "-1" {
		// Respond with error.
		glog.Warning("Cannot get local-cluster id.")
		http.Error(w, "Cannot get local-cluster id.", 503)
		return
	}
	// Respond with success
	fmt.Fprint(w, "OK")
}

// ReadinessProbe checks if contents size.
func ReadinessProbe(w http.ResponseWriter, r *http.Request) {
	glog.V(2).Info("readinessProbe - Checking contents map size.")

	// Get Contents map length to find if we can get contents
	// from cloud.redhat.com
	if len(retriever.ContentsMap) < 1 {
		// Respond with error.
		glog.Warning("Contents map is empty.")
		http.Error(w, "Contents map is empty.", 503)
		return
	}
	// Respond with success
	fmt.Fprint(w, "OK")
}

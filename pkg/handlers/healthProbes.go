// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
package handlers

import (
	"fmt"
	"net/http"

	"github.com/golang/glog"
)

// LivenessProbe is used to check if this service is alive.
func LivenessProbe(w http.ResponseWriter, r *http.Request) {
	glog.V(2).Info("livenessProbe")
	fmt.Fprint(w, "OK")
}

func ReadinessProbe(w http.ResponseWriter, r *http.Request) {
	glog.V(2).Info("readinessProbe - Checking CCX connection.")

	// Respond with success
	fmt.Fprint(w, "OK")
}

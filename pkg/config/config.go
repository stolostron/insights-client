// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package config

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/golang/glog"
)

const (
	DEFAULT_SERVICE_PORT     = ":3030"
	DEFAULT_HTTP_TIMEOUT     = 180000                                  // 3 minutes HTTP Timeout
	DEFAULT_CCX_SERVER       = "http://localhost:8080/api/v1/clusters" // For local use only
	DEFAULT_POLL_INTERVAL    = 30                                      // 30mins default polling interval cloud.redhat.com
	DEFAULT_REQUEST_INTERVAL = 1                                       // 1 second Interval between 2 consecutive requests
	DEFAULT_POD_NAMESPACE    = "kube-system"                           // Namespace of insights-client pod
)

// Config - Define a config type to hold our config properties.
type Config struct {
	ServicePort     string `env:"SERVICE_PORT"`
	CCXServer       string `env:"CCX_SERVER"`
	HTTPTimeout     int    `env:"HTTP_TIMEOUT"`     // timeout when the http server should drop connections
	KubeConfig      string `env:"KUBECONFIG"`       // Local kubeconfig path
	CCXToken        string `env:"CCX_TOKEN"`        // Token to access CCX server , when pull-secret cannot be used
	PollInterval    int    `env:"POLL_INTERVAL"`    // Polling interval to reports from cloud.redhat.com
	RequestInterval int    `env:"REQUEST_INTERVAL"` // Interval between 2 consequent requests
	CACert          string `env:"CACert"`           // base64 encoded caCert used for dev & test
	PodNamespace    string `env:"POD_NAMESPACE"`    // Namespace of insights-client pod
}

// Cfg service configuration
var Cfg = Config{}

var message = "Using %s from environment: %s"

func SetupConfig() {
	// If environment variables are set, use those values constants
	// Simply put, the order of preference is env -> default constants (from left to right)
	setDefault(&Cfg.ServicePort, "SERVICE_PORT", DEFAULT_SERVICE_PORT)
	setDefault(&Cfg.CCXServer, "CCX_SERVER", DEFAULT_CCX_SERVER)
	setDefault(&Cfg.CCXToken, "CCX_TOKEN", "")
	setDefault(&Cfg.CACert, "CACert", "")
	setDefault(&Cfg.PodNamespace, "POD_NAMESPACE", DEFAULT_POD_NAMESPACE)
	setDefaultInt(&Cfg.HTTPTimeout, "HTTP_TIMEOUT", DEFAULT_HTTP_TIMEOUT)
	setDefaultInt(&Cfg.PollInterval, "POLL_INTERVAL", DEFAULT_POLL_INTERVAL)
	setDefaultInt(&Cfg.RequestInterval, "REQUEST_INTERVAL", DEFAULT_REQUEST_INTERVAL)
	defaultKubePath := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	if _, err := os.Stat(defaultKubePath); os.IsNotExist(err) {
		// set default to empty string if path does not resolve
		defaultKubePath = ""
	}
	setDefault(&Cfg.KubeConfig, "KUBECONFIG", defaultKubePath)
}

func setDefault(field *string, env, defaultVal string) {
	if val := os.Getenv(env); val != "" {
		glog.V(2).Infof(message, env, val)
		*field = val
	} else if *field == "" && defaultVal != "" {
		glog.V(2).Infof("%s not set, using default value: %s", env, defaultVal)
		*field = defaultVal
	}
}

func setDefaultInt(field *int, env string, defaultVal int) {
	if val := os.Getenv(env); val != "" {
		glog.Infof(message, env, val)
		var err error
		*field, err = strconv.Atoi(val)
		if err != nil {
			glog.Error("Error parsing env [", env, "].  Expected an integer.  Original error: ", err)
		}
	} else if *field == 0 && defaultVal != 0 {
		glog.V(2).Infof("No %s from file or environment, using default value: %d", env, defaultVal)
		*field = defaultVal
	}
}

func setDefaultBool(field *bool, env string, defaultVal bool) {
	if val := os.Getenv(env); val != "" {
		glog.Infof(message, env, val)
		var err error
		*field, err = strconv.ParseBool(val)
		if err != nil {
			glog.Error("Error parsing env [", env, "].  Expected a boolean.  Original error: ", err)
		}
	} else {
		glog.V(2).Infof("No %s from file or environment, using default value: %v", env, defaultVal)
		*field = defaultVal
	}
}

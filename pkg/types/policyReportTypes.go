// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
package types

type ResponseBody struct {
	Reports map[string]interface{} `json:"reports"`
	Errors  map[string]string      `json:"errors"`
}

type PostBody struct {
	Clusters []string `json:"clusters"`
}

type Reports struct {
	Reports []ReportData     `json:"reports"`
}

type ReportData struct {
	RuleID    string      `json:"rule_id"`
	Key       string      `json:"key"`
	Component string      `json:"component"`
	Details   interface{} `json:"details"`
}

type ProcessorData struct {
	ClusterInfo ManagedClusterInfo
	Reports     Reports
}

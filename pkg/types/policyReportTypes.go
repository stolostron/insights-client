// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
package types

type ResponseBody struct {
	Reports map[string]interface{} `json:"reports"`
}

type PostBody struct {
	Clusters []string `json:"clusters"`
}

type Reports struct {
	Reports []ReportData     `json:"reports"`
	Skips   []SkippedReports `json:"skips"`
}

type ReportData struct {
	RuleID    string      `json:"rule_id"`
	Key       string      `json:"key"`
	Component string      `json:"component"`
	Details   interface{} `json:"details"`
}

type SkippedReports struct {
	RuleID string `json:"rule_fqdn"`
}

type PolicyReportGetResponse struct {
	Meta struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Results []PolicyReportResultData `json:"results"`
}

type PolicyReportResultData struct {
    Status     string             `json:"status"`
    Properties map[string]string  `json:"properties"`
}

type ProcessorData struct {
	ClusterInfo ManagedClusterInfo
	Reports     Reports
}

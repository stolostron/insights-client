// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
package types

import "time"

// ResponseBody represents the main response structure from the insights API
type ResponseBody struct {
	Report ReportBody  `json:"report"`
	Status string      `json:"status"`
}

// ReportBody contains the report data and metadata
type ReportBody struct {
	Data []ReportData `json:"data"`
	Meta MetaData     `json:"meta"`
}

// MetaData contains metadata about the report
type MetaData struct {
	ClusterName     string    `json:"cluster_name"`
	Count           int       `json:"count"`
	GatheredAt      time.Time `json:"gathered_at"`
	LastCheckedAt   time.Time `json:"last_checked_at"`
	Managed         bool      `json:"managed"`
}

// ReportData represents a single report item
type ReportData struct {
	CreatedAt      time.Time              `json:"created_at"`
	Description    string                 `json:"description"`
	Details        string                 `json:"details"`
	DisableFeedback string                `json:"disable_feedback"`
	Disabled       bool                   `json:"disabled"`
	DisabledAt     string                 `json:"disabled_at"`
	ExtraData      ExtraData              `json:"extra_data"`
	Impacted       string                 `json:"impacted"`
	Internal       bool                   `json:"internal"`
	MoreInfo       string                 `json:"more_info"`
	Reason         string                 `json:"reason"`
	Resolution     string                 `json:"resolution"`
	RuleID         string                 `json:"rule_id"`
	Tags           []string               `json:"tags"`
	TotalRisk      int                    `json:"total_risk"`
	UserVote       int                    `json:"user_vote"`
}

// ExtraData contains additional data for the report
type ExtraData struct {
	ErrorKey string   `json:"error_key"`
	Nodes    []string `json:"nodes"`
	Type     string   `json:"type"`
}

// Legacy types for backward compatibility
type Report struct {
	Report []ReportData `json:"report"`
}

type ProcessorData struct {
	ClusterInfo ManagedClusterInfo
	Report      ReportBody
}
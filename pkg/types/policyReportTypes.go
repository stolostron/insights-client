// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
package types

type ResponseBody struct {
	Reports map[string]interface{} `json:"reports"`
}

type ReportData struct {
	Created         string      `json:"created_at"`
	Description     string      `json:"description"`
	Details         Details     `json:"details"`
	Reason          string      `json:"reason"`
	Resolution      string      `json:"resolution"`
	RiskOfChange    int64       `json:"risk_of_change"`
	TotalRisk       int         `json:"total_risk"`
	ID              string      `json:"rule_id"`
	Disabled        bool        `json:"disabled"`
	DisableFeedback string      `json:"disable_feedback"`
	DisabledAt      string      `json:"disabled_at"`
	Internal        bool        `json:"internal"`
	UserVote        int64       `json:"user_vote"`
	ExtraData       interface{} `json:"extra_data"`
	Tags            []string    `json:"tags"`
}

type PolicyInfo struct {
	ClusterId string
	Policy    Policy
}
type Policy struct {
	Report []ReportData `json:"reports"`
}

type PolicyReport struct {
	Meta struct {
		LastChecked string `json:"last_checked_at"`
	} `json:"metadata"`
	Data []ReportData `json:"data"`
}

type PostBody struct {
	Clusters []string `json:"clusters"`
}

type Details struct {
	Error_key string `json:"error_key"`
	Typestr   string `json:"type"`
}

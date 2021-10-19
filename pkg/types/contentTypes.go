// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
package types

type ContentsResponse struct {
	Content []Summary `json:"content"`
}

type Summary struct {
	Summary    string                 `json:"summary"`
	Reason     string                 `json:"reason"`
	Resolution string                 `json:"resolution"`
	Error_keys map[string]interface{} `json:"error_keys"`
}

type Error_vals struct {
	Generic   string      `json:"generic"`
	Metadata  interface{} `json:"metadata"`
	Reason    string      `json:"reason"`
	HasReason string      `json:"HasReason"`
}

type FormattedContentData struct {
	Summary     string   `json:"summary"`
	Reason      string   `json:"reason"`
	Resolution  string   `json:"resolution"`
	Generic     string   `json:"generic"`
	HasReason   bool     `json:"HasReason"`
	Condition   string   `json:"condition"`
	Description string   `json:"description"`
	Impact      int      `json:"impact"`
	Likelihood  int      `json:"likelihood"`
	TotalRisk   int      `json:"total_risk"`
	PublishDate string   `json:"publish_date"`
	Status      string   `json:"status"`
	Tags        []string `json:"tags"`
}

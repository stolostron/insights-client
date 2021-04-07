// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package processor

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/open-cluster-management/insights-client/pkg/config"
	"github.com/open-cluster-management/insights-client/pkg/retriever"
	"github.com/open-cluster-management/insights-client/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sTypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/wg-policy-prototypes/policy-report/api/v1alpha2"
)

//  patchUint32Value specifies a patch operation for a uint32.
type patchStringValue struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value string `json:"value"`
}

// Processor struct
type Processor struct {
}

// NewProcessor ...
func NewProcessor() *Processor {
	p := &Processor{}
	return p
}

// CreateUpdatePolicyReports ...
func (p *Processor) CreateUpdatePolicyReports(input chan types.ProcessorData, ret *retriever.Retriever, hubID string) {
	data := <-input
	// Loop through Report array and return a PolicyReport for each violation
	for _, report := range data.Reports.Reports {
		// Find the correct Insight content data from cache
		reportData := retriever.ContentsMap[report.Key]
		if reportData != nil {
			var contentData types.FormattedContentData
			reportDataBytes, _ := json.Marshal(reportData)
			unmarshalError := json.Unmarshal(reportDataBytes, &contentData)
			if unmarshalError == nil {
				createPolicyReport(contentData, report, data.ClusterInfo)
			} else {
				glog.Infof(
					"Error unmarshalling Report %v for cluster %s (%s)",
					unmarshalError,
					data.ClusterInfo.Namespace,
					data.ClusterInfo.ClusterID,
				)
			}
		} else {
			glog.Info("Could not find the content data for this Insight - Refreshing content list")

			ret.InitializeContents(hubID)
		}
	}

	// Update any existing PolicyReports that have been resolved
	updatePolicyReports(data.Reports.Skips, data.ClusterInfo.Namespace)
}

// createUpdatePolicyReport ...
func createPolicyReport(
	contentData types.FormattedContentData,
	report types.ReportData,
	cluster types.ManagedClusterInfo,
) {
	cfg := config.GetConfig()
	restClient := config.RESTClient(cfg)
	// PolicyReport name must consist of lower case alphanumeric characters, '-' or '.'
	ruleName := strings.ReplaceAll(report.Component, "_", "-")
	ruleName = strings.ToLower(ruleName)

	// Try a GET request to see if the PolicyReport already exists
	getResp := restClient.Get().
		Resource("policyreports").
		Namespace(cluster.Namespace).
		Name(ruleName).
		Do(context.TODO())

	respBytes, _ := getResp.Raw()
	var prResponse types.PolicyReportGetResponse
	unmarshalError := json.Unmarshal(respBytes, &prResponse)
	if unmarshalError != nil {
		glog.Infof(
			"Error unmarshalling PolicyReport CR: %v",
			unmarshalError,
		)
	}

	if unmarshalError == nil && prResponse.Meta.Name == "" {
		// If the PolicyReport doesn't exist Create it
		// *** Need to use report.details to fill in the template values
		// Example: policyReport.Summary may have template strings that need to be replaced by the report.details information
		policyreport := &v1alpha2.PolicyReport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ruleName,
				Namespace: cluster.Namespace,
			},
			Results: []*v1alpha2.PolicyReportResult{{
				Policy:      report.Key,
				Description: contentData.Description,
				Scored:      false,
				Category:    strings.Join(contentData.Tags, ","),
				Timestamp:   metav1.Timestamp{Seconds: time.Now().Unix(), Nanos: int32(time.Now().UnixNano())},
				// We will use Result to represent whether the violation has been resolved
				// On creation it is error, when the violation is cleared it is set to skip
				Result: "error",
				Properties: map[string]string{
					"created_at": contentData.Publish_date,
					// *** total_risk is not currently included in content data, but being added by CCX team.
					"total_risk": strconv.Itoa(contentData.Likelihood),
					"reason":     contentData.Reason,
					"resolution": contentData.Resolution,
				},
			}},
		}

		postResp := restClient.Post().
			Resource("policyreports").
			Namespace(cluster.Namespace).
			Body(policyreport).
			Do(context.TODO())

		if postResp.Error() != nil {
			glog.Infof(
				"Error creating PolicyReport %s for cluster %s (%s): %v",
				report.Component,
				cluster.Namespace,
				cluster.ClusterID,
				postResp.Error(),
			)
		} else {
			glog.Infof(
				"Successfully created PolicyReport %s for cluster %s (%s)",
				report.Component,
				cluster.Namespace,
				cluster.ClusterID,
			)
		}
	} else if unmarshalError == nil && prResponse.Meta.Name != "" && prResponse.Results[0].Status == "skip" {
		glog.Infof("PolicyReport %s has been reintroduced, updating status to error", prResponse.Meta.Name)
		payload := []patchStringValue{{
			Op:    "replace",
			Path:  "/results/0/result",
			Value: "error",
		}}
		payloadBytes, _ := json.Marshal(payload)

		resp := restClient.Patch(k8sTypes.JSONPatchType).
			Resource("policyreports").
			Namespace(cluster.Namespace).
			Name(ruleName).
			Body(payloadBytes).
			Do(context.TODO())

		if resp.Error() != nil {
			glog.Infof(
				"Error updating PolicyReport %s status to error for cluster %s (%s): %v",
				report.Component,
				cluster.Namespace,
				cluster.ClusterID,
				resp.Error(),
			)
		} else {
			glog.Infof(
				"Successfully updated PolicyReport %s status to error for cluster %s (%s)",
				report.Component,
				cluster.Namespace,
				cluster.ClusterID,
			)
		}
	}
}

// updatePolicyReports - Updates status to "skip" for all PolicyReports present in a namespace
// This means the PolicyReport has been resolved
func updatePolicyReports(skippedReports []types.SkippedReports, clusterNamespace string) {
	for _, rule := range skippedReports {
		glog.Info(rule)
		cfg := config.GetConfig()
		restClient := config.RESTClient(cfg)
		getResp := restClient.Get().
			Resource("policyreports").
			Namespace(clusterNamespace).
			Name(rule.RuleID).
			Do(context.TODO())

		respBytes, _ := getResp.Raw()
		var prResponse types.PolicyReportGetResponse
		unmarshalError := json.Unmarshal(respBytes, &prResponse)
		if unmarshalError != nil {
			glog.Infof(
				"Error unmarshalling PolicyReport: %v",
				unmarshalError,
			)
		}

		if unmarshalError == nil && prResponse.Meta.Name != "" && prResponse.Results[0].Status != "skip" {
			glog.Infof("PolicyReport %s has been resolved, updating status to skip", rule.RuleID)
			payload := []patchStringValue{{
				Op:    "replace",
				Path:  "/results/0/result",
				Value: "skip",
			}}
			payloadBytes, _ := json.Marshal(payload)

			resp := restClient.Patch(k8sTypes.JSONPatchType).
				Resource("policyreports").
				Namespace(clusterNamespace).
				Name(rule.RuleID).
				Body(payloadBytes).
				Do(context.TODO())

			if resp.Error() != nil {
				glog.Infof(
					"Error updating PolicyReport %s status for cluster %s: %v",
					rule.RuleID,
					clusterNamespace,
					resp.Error(),
				)
			} else {
				glog.Infof(
					"Successfully updated PolicyReport %s status for cluster %s",
					rule.RuleID,
					clusterNamespace,
				)
			}
		}
	}
}

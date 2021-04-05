// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package processor

import (
	"context"
	"encoding/json"
	"strings"
	"strconv"

	"github.com/golang/glog"
	"github.com/open-cluster-management/insights-client/pkg/config"
	"github.com/open-cluster-management/insights-client/pkg/types"
	"github.com/open-cluster-management/insights-client/pkg/retriever"
	k8sTypes "k8s.io/apimachinery/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/wg-policy-prototypes/policy-report/api/v1alpha1"
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
func (p *Processor) CreateUpdatePolicyReports(input chan types.ProcessorData) {
	data := <-input
	// Loop through Report array and return a PolicyReport for each violation
	for _, report := range data.Reports.Reports {
		// Find the correct Insight content data from cache
		reportData := retriever.ContentsMap[report.Key]
		if reportData != nil {
			var contentData types.FormattedContentData
			reportDataBytes, _ := json.Marshal(reportData)
			unmarshalError := json.Unmarshal(reportDataBytes, &contentData)
			if unmarshalError != nil {
				glog.Infof(
					"Error unmarshalling Report %v for cluster %s (%s)",
					unmarshalError,
					data.ClusterInfo.Namespace,
					data.ClusterInfo.ClusterID,
				)
			}
			createPolicyReport(contentData, report, data.ClusterInfo)
		}
	}

	// Update any existing PolicyReports that have been resolved
	updatePolicyReports(data.Reports.Skips, data.ClusterInfo.Namespace)
}

// createUpdatePolicyReport ...
func createPolicyReport(
	policyReport types.FormattedContentData,
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
			"Error unmarshalling PolicyReport: %v",
			unmarshalError,
		)
	}

	if (prResponse.Meta.Name == "") {
		// If the PolicyReport doesn't exist Create it
		// TODO Need to use report.details to fill in the template values
		// Example: policyReport.Summary may have template strings that need to be replaced by the report.details information
		policyreport := &v1alpha1.PolicyReport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ruleName,
				Namespace: cluster.Namespace,
			},
			Results: []*v1alpha1.PolicyReportResult{{
				Policy:   report.Key,
				Message:  policyReport.Description,
				// We will use Scored to represent whether the violation has been resolved
				// On creation it is false, when the violation is cleared it is set to true
				Scored:   false,
				Category: strings.Join(policyReport.Tags, ","),
				Status:   "error",
				Data: map[string]string{
					"created_at": policyReport.Publish_date,
					// TODO total_risk is no longer available, need to sync with CCX team to determine best route here
					"total_risk": strconv.Itoa(policyReport.Likelihood),
					"reason":     policyReport.Reason,
					"resolution": policyReport.Resolution,
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
	} else if (prResponse.Meta.Name != "" && prResponse.Results[0].Status == "skip") {
		glog.Infof("PolicyReport %s has been reintroduced, updating status to error", prResponse.Meta.Name)
		payload := []patchStringValue{{
			Op:    "replace",
			Path:  "/results/0/status",
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
				"Error updating PolicyReport %s status to error for cluster %s: %v",
				report.Component,
				cluster.Namespace,
				resp.Error(),
			)
		} else {
			glog.Infof(
				"Successfully updated PolicyReport %s status to error for cluster %s",
				report.Component,
				cluster.Namespace,
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

		if (prResponse.Meta.Name != "") {
			glog.Infof("PolicyReport %s has been resolved, updating status to skip", rule.RuleID)
			payload := []patchStringValue{{
				Op:    "replace",
				Path:  "/results/0/status",
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

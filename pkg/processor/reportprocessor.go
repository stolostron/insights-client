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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime"
	k8sTypes "k8s.io/apimachinery/pkg/types"
    "sigs.k8s.io/wg-policy-prototypes/policy-report/api/v1alpha2"
)

// Processor struct
type Processor struct {
}

var currentPolicyReport v1alpha2.PolicyReport
var policyReportGvr = schema.GroupVersionResource{
	Group:    "wgpolicyk8s.io",
	Version:  "v1alpha2",
	Resource: "policyreports",
}

// NewProcessor ...
func NewProcessor() *Processor {
    p := &Processor{}
    return p
}

func findRuleIndex(ruleID string, prResults []*v1alpha2.PolicyReportResult) (int) {
    for idx, value := range prResults {
        if ruleID == value.Properties["component"] {
            return idx
        }
    }
    return -1    //not found.
}

func getPolicyReportResults(
	reports []types.ReportData,
    clusterInfo types.ManagedClusterInfo,
) ([]*v1alpha2.PolicyReportResult) {
	var newPolicyReportViolations []*v1alpha2.PolicyReportResult
    for _, report := range reports {
        // Find the correct Insight content data from cache
		reportData := retriever.ContentsMap[report.Key]
        ruleIndex := findRuleIndex(report.Component, currentPolicyReport.Results)
        if reportData != nil && ruleIndex == -1 {
            var contentData types.FormattedContentData
            reportDataBytes, _ := json.Marshal(reportData)
            unmarshalError := json.Unmarshal(reportDataBytes, &contentData)
            if unmarshalError == nil {
                newPolicyReportViolations = append(newPolicyReportViolations, &v1alpha2.PolicyReportResult{
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
						"component":  report.Component,
						// Need to store extra data here for templating changes in UI
                    },
				})
            } else {
                glog.Infof(
                    "Error unmarshalling Report %v for cluster %s (%s)",
                    unmarshalError,
                    clusterInfo.Namespace,
                    clusterInfo.ClusterID,
				)
            }
        }
	}
	return newPolicyReportViolations
}

func patchRequest(
	clusterInfo types.ManagedClusterInfo,
) (error) {
	currentPolicyReport.SetManagedFields(nil)
	data, marshalErr := json.Marshal(currentPolicyReport)
	if marshalErr != nil {
		glog.Warningf("Error Marshalling PolicyReport patch object for cluster %s: %v", clusterInfo.Namespace, marshalErr)
		return marshalErr
	}

	dynamicClient := config.GetDynamicClient()
	forcePatch := true
	successPatchRes, err := dynamicClient.Resource(policyReportGvr).Namespace(clusterInfo.Namespace).Patch(
		context.TODO(),
		clusterInfo.Namespace,
		k8sTypes.ApplyPatchType,
		data,
		metav1.PatchOptions{
			FieldManager: "insights-client",
			Force: &forcePatch,
		},
	)

	if successPatchRes != nil && err == nil {
		unstructConvErr := runtime.DefaultUnstructuredConverter.FromUnstructured(
			successPatchRes.UnstructuredContent(),
			&currentPolicyReport,
		)
		if unstructConvErr != nil {
			glog.Warningf("Error unstructuring PolicyReport for cluster: %s", clusterInfo.Namespace)
			return unstructConvErr
		}
	}
	
	return err
}

// CreateUpdatePolicyReports - Creates a PolicyReport for cluster if one does not already exist and updates the status of violations
func (p *Processor) CreateUpdatePolicyReports(input chan types.ProcessorData) {
    for {
        data := <-input

        dynamicClient := config.GetDynamicClient()
		policyReportRes, _ := dynamicClient.Resource(policyReportGvr).Namespace(data.ClusterInfo.Namespace).Get(
			context.TODO(),
			data.ClusterInfo.Namespace,
			metav1.GetOptions{},
		)

		if policyReportRes != nil {
			unstructConvErr := runtime.DefaultUnstructuredConverter.FromUnstructured(
				policyReportRes.UnstructuredContent(),
				&currentPolicyReport,
			)
			if unstructConvErr != nil {
				glog.Warningf("Error unstructuring PolicyReport for cluster: %s", data.ClusterInfo.Namespace)
				return
			}
		}
		newPolicyReportViolations := getPolicyReportResults(
			data.Reports.Reports,
			data.ClusterInfo,
		)
		if currentPolicyReport.GetName() == "" {
			// If the PolicyReport does not exist on the cluster create it
			createPolicyReport(newPolicyReportViolations, data.ClusterInfo)
		} else if currentPolicyReport.GetName() != "" {
			// PolicyReport exists on cluster - Adding rule violations
			if len(newPolicyReportViolations) > 0 {
				// If the PolicyReport exists need to update the results if there are new violations
				addPolicyReportViolations(newPolicyReportViolations, data.ClusterInfo)
			}
			// Only update statuses if the PolicyReport has > 0 violations
			if len(currentPolicyReport.Results) > 0 {
				// Update status of existing PolicyReport violations
				updatePolicyReportResultStatus(data.Reports, data.ClusterInfo)
			}
		}
    }
}

func createPolicyReport(
    newPolicyReportViolations []*v1alpha2.PolicyReportResult,
    clusterInfo types.ManagedClusterInfo,
) {
	// PolicyReport doesnt exist for cluster - creating
	policyreport := &v1alpha2.PolicyReport{
		TypeMeta: metav1.TypeMeta{
			Kind: "PolicyReport",
			APIVersion: "wgpolicyk8s.io/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterInfo.Namespace,
			Namespace: clusterInfo.Namespace,
		},
		Results: newPolicyReportViolations,
	}
	prUnstructured, unstructuredErr := runtime.DefaultUnstructuredConverter.ToUnstructured(policyreport)
	if unstructuredErr != nil {
		glog.Warningf("Error converting to unstructured.Unstructured: %s", unstructuredErr)
	}
	obj := &unstructured.Unstructured{Object: prUnstructured}

	dynamicClient := config.GetDynamicClient()
	_, err := dynamicClient.Resource(policyReportGvr).Namespace(clusterInfo.Namespace).Create(
		context.TODO(),
		obj,
		metav1.CreateOptions{},
	)

	if err != nil {
		glog.Infof(
			"Error creating PolicyReport for cluster %s (%s): %v",
			clusterInfo.Namespace,
			clusterInfo.ClusterID,
			err,
		)
	} else {
		glog.Infof(
			"Successfully created PolicyReport for cluster %s (%s)",
			clusterInfo.Namespace,
			clusterInfo.ClusterID,
		)
	}
}

func addPolicyReportViolations(
	newPolicyReportViolations []*v1alpha2.PolicyReportResult,
    clusterInfo types.ManagedClusterInfo,
) {
	// merge existing PolicyReport results with new results
	currentPolicyReport.Results = append(currentPolicyReport.Results, newPolicyReportViolations...)
	err := patchRequest(clusterInfo)

	if err != nil {
		glog.Infof(
			"Error adding new PolicyReport violations for cluster %s (%s): %v",
			clusterInfo.Namespace,
			clusterInfo.ClusterID,
			err,
		)
	} else {
		glog.Infof(
			"Successfully added new PolicyReport violations for cluster %s (%s)",
			clusterInfo.Namespace,
			clusterInfo.ClusterID,
		)
	}
}

// updatePolicyReportResultStatus Updates status ('error' | 'skip') of violations
func updatePolicyReportResultStatus(
	Reports     types.Reports,
    clusterInfo types.ManagedClusterInfo,
) {
	isUpdatedStatuses := false
	for idx, resultRule := range currentPolicyReport.Results {
		// Update status of all resolved violations from error to skip
		for _, rule := range Reports.Skips {
			if resultRule.Result != "skip" && rule.RuleID == resultRule.Properties["component"] {
				glog.Infof("PolicyReport violation %s has been resolved, updating status to skip", rule.RuleID)
				currentPolicyReport.Results[idx].Result = "skip"
				isUpdatedStatuses = true
			}
		}
		// Update status of violations that were resolved but are now active again from skip to error
		for _, rule := range Reports.Reports {
			if resultRule.Result != "error" && rule.Component == resultRule.Properties["component"] {
				glog.Infof("PolicyReport violation %s has reappeared, updating status to error", rule.RuleID)
				currentPolicyReport.Results[idx].Result = "error"
				isUpdatedStatuses = true
			}
		}
	}
	// Apply patch only if there are violations whose status has changed
	if isUpdatedStatuses {
		err := patchRequest(clusterInfo)

		if err != nil {
			glog.Infof(
				"Error updating PolicyReport statuses for cluster %s (%s): %v",
				clusterInfo.Namespace,
				clusterInfo.ClusterID,
				err,
			)
		} else {
			glog.Infof(
				"Successfully updated PolicyReport statuses for cluster %s (%s)",
				clusterInfo.Namespace,
				clusterInfo.ClusterID,
			)
		}
	}
}

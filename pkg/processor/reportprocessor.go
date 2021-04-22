// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package processor

import (
	// "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

//  patchStringValue specifies a patch operation for a uint32.
type patchStringValue struct {
    Op    string `json:"op"`
    Path  string `json:"path"`
    Value string `json:"value"`
}

//  patchStringValue specifies a patch operation for a uint32.
type patchPRResultsValue struct {
    Op    string                         `json:"op"`
    Path  string                         `json:"path"`
    Value []*v1alpha2.PolicyReportResult `json:"value"`
}

// Processor struct
type Processor struct {
}

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
    prResponse v1alpha2.PolicyReport,
    clusterInfo types.ManagedClusterInfo,
) ([]*v1alpha2.PolicyReportResult) {
	var newPolicyReportViolations []*v1alpha2.PolicyReportResult
    for _, report := range reports {
        // Find the correct Insight content data from cache
		reportData := retriever.ContentsMap[report.Key]
        ruleIndex := findRuleIndex(report.Component, prResponse.Results)
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

// CreateUpdatePolicyReports ...
func (p *Processor) CreateUpdatePolicyReports(input chan types.ProcessorData) {
    for {
        data := <-input

		var prResponse v1alpha2.PolicyReport
        dynamicClient := config.GetDynamicClient()
		policyReportRes, _ := dynamicClient.Resource(policyReportGvr).Namespace(data.ClusterInfo.Namespace).Get(
			context.TODO(),
			data.ClusterInfo.Namespace,
			metav1.GetOptions{},
		)

		if policyReportRes != nil {
			unstructConvErr := runtime.DefaultUnstructuredConverter.FromUnstructured(policyReportRes.UnstructuredContent(), &prResponse)
			if unstructConvErr != nil {
				glog.Warningf("Error unstructuring PolicyReport for cluster: %s", data.ClusterInfo.Namespace)
				return
			}
		}
		newPolicyReportViolations := getPolicyReportResults(
			data.Reports.Reports,
			prResponse,
			data.ClusterInfo,
		)
		if prResponse.GetName() == "" {
			// If the PolicyReport does not exist on the cluster create it
			createPolicyReport(newPolicyReportViolations, data.ClusterInfo)
		} else if prResponse.GetName() != "" {
			// If the PolicyReport exists need to update the results if there are new violations
			addPolicyReportViolations(newPolicyReportViolations, prResponse, data.ClusterInfo)
			// Update any existing PolicyReport violations that have been resolved
			updatePolicyReportResultStatus(data.Reports.Skips, prResponse, data.ClusterInfo)
		}
    }
}

func createPolicyReport(
    newPolicyReportViolations []*v1alpha2.PolicyReportResult,
    clusterInfo types.ManagedClusterInfo,
) {
	if newPolicyReportViolations != nil {
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
		_, err := dynamicClient.Resource(policyReportGvr).Namespace(clusterInfo.Namespace).Create(context.TODO(), obj, metav1.CreateOptions{})

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
}

func addPolicyReportViolations(
	newPolicyReportViolations []*v1alpha2.PolicyReportResult,
	prResponse v1alpha2.PolicyReport,
    clusterInfo types.ManagedClusterInfo,
) {
	// PolicyReport exists on cluster - Adding rule violations
	if len(newPolicyReportViolations) > 0 {
		// merge existing PolicyReport results with new results
		prResponse.Results = append(prResponse.Results, newPolicyReportViolations...)
		prResponse.SetManagedFields(nil)
		data, marshalErr := json.Marshal(prResponse)
		if marshalErr != nil {
			glog.Warningf("Error Marshalling PolicyReport patch object for cluster %s: %v", clusterInfo.Namespace, marshalErr)
			return
		}

		dynamicClient := config.GetDynamicClient()
		forcePatch := true
		_, err := dynamicClient.Resource(policyReportGvr).Namespace(clusterInfo.Namespace).Patch(
			context.TODO(),
			clusterInfo.Namespace,
			k8sTypes.ApplyPatchType,
			data,
			metav1.PatchOptions{
				FieldManager: "insights-client",
				Force: &forcePatch,
			},
		)

		if err != nil {
			glog.Infof(
				"Error adding PolicyReport violations for cluster %s (%s): %v",
				clusterInfo.Namespace,
				clusterInfo.ClusterID,
				err,
			)
		} else {
			glog.Infof(
				"Successfully added PolicyReport violations for cluster %s (%s)",
				clusterInfo.Namespace,
				clusterInfo.ClusterID,
			)
		}
	}
}

// updatePolicyReportResultStatus - Updates status to "skip" for all violations that have been resolved
func updatePolicyReportResultStatus(
    skippedReports []types.SkippedReports,
    prResponse v1alpha2.PolicyReport,
    clusterInfo types.ManagedClusterInfo,
) {
	// Only update statuses if the PolicyReport has > 0 violations
	if len(prResponse.Results) > 0 {
		isUpdatedStatuses := false
		// Update status of all resolved vilations from error to skip
		for _, rule := range skippedReports {
			for idx, resultRule := range prResponse.Results {
				if resultRule.Result != "skip" && rule.RuleID == resultRule.Properties["component"] {
					glog.Infof("PolicyReport violation %s has been resolved, updating status to skip", rule.RuleID)
					prResponse.Results[idx].Result = "skip"
					isUpdatedStatuses = true
				}
			}
		}
		// Apply patch only if there are violations that have been resolved
		if isUpdatedStatuses {
			prResponse.SetManagedFields(nil)
			data, marshalErr := json.Marshal(prResponse)
			if marshalErr != nil {
				glog.Warningf("Error Marshalling PolicyReport patch object for cluster %s: %v", clusterInfo.Namespace, marshalErr)
			}

			dynamicClient := config.GetDynamicClient()
			forcePatch := true
			_, err := dynamicClient.Resource(policyReportGvr).Namespace(clusterInfo.Namespace).Patch(
				context.TODO(),
				clusterInfo.Namespace,
				k8sTypes.ApplyPatchType,
				data,
				metav1.PatchOptions{
					FieldManager: "insights-client",
					Force: &forcePatch,
				},
			)

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
}

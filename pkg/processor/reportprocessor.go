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
	"github.com/stolostron/insights-client/pkg/retriever"
	"github.com/stolostron/insights-client/pkg/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sTypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/wg-policy-prototypes/policy-report/pkg/api/wgpolicyk8s.io/v1alpha2"
)

var prSuffix = "-policyreport"

// Processor struct
type Processor struct {
}

var policyReportGvr = schema.GroupVersionResource{
	Group:    "wgpolicyk8s.io",
	Version:  "v1alpha2",
	Resource: "policyreports",
}

var policyGvr = schema.GroupVersionResource{
	Group:    "policy.open-cluster-management.io",
	Version:  "v1",
	Resource: "policies",
}

// NewProcessor ...
func NewProcessor() *Processor {
	p := &Processor{}
	return p
}

// FilterOpenshiftCategory Filter openshift category from list
func FilterOpenshiftCategory(categories []string) string {
	var filteredCategories []string
	for i := range categories {
		if categories[i] != "openshift" {
			filteredCategories = append(filteredCategories, categories[i])
		}
	}
	return strings.Join(filteredCategories, ",")
}

func getPolicyReportResults(
	reports []types.ReportData,
	clusterInfo types.ManagedClusterInfo,
) []*v1alpha2.PolicyReportResult {
	var clusterViolations []*v1alpha2.PolicyReportResult
	for _, report := range reports {
		// Find the correct Insight content data from cache
		reportContentData := retriever.ContentsMap[report.Key]
		// Convert details data to string
		jsonStr, _ := json.Marshal(report.Details)
		extraData := string(jsonStr)
		if reportContentData != nil && !strings.Contains(report.Component, "tutorial_rule") {
			var contentData types.FormattedContentData
			reportContentDataBytes, _ := json.Marshal(reportContentData)
			unmarshalError := json.Unmarshal(reportContentDataBytes, &contentData)
			if unmarshalError == nil {
				clusterViolations = append(clusterViolations, &v1alpha2.PolicyReportResult{
					Policy:      report.Key,
					Description: contentData.Description,
					Scored:      false,
					Category:    FilterOpenshiftCategory(contentData.Tags),
					Source:      "insights",
					Timestamp:   metav1.Timestamp{Seconds: time.Now().Unix(), Nanos: int32(time.Now().UnixNano())},
					Result:      "fail",
					Properties: map[string]string{
						"created_at": contentData.PublishDate,
						"total_risk": strconv.Itoa(contentData.TotalRisk),
						"component":  report.Component,
						"extra_data": extraData,
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
	return clusterViolations
}

// CreateUpdatePolicyReports - Creates a PolicyReport for cluster if one does not already exist and updates the status of violations
func (p *Processor) createUpdatePolicyReports(input chan types.ProcessorData, dynamicClient dynamic.Interface) {
	data := <-input

	if data.ClusterInfo.ClusterID == "" || data.ClusterInfo.Namespace == "" {
		glog.Info("Missing managed cluster ID and/or Namespace nothing to process")
		return
	}

	currentPolicyReport := v1alpha2.PolicyReport{}
	policyReportRes, _ := dynamicClient.Resource(policyReportGvr).Namespace(data.ClusterInfo.Namespace).Get(
		context.TODO(),
		data.ClusterInfo.Namespace+prSuffix,
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

	clusterViolations := getPolicyReportResults(
		data.Reports.Reports,
		data.ClusterInfo,
	)

	govViolations := getGovernanceResults(dynamicClient, data.ClusterInfo)
	if len(govViolations) > 0 {
		clusterViolations = append(clusterViolations, govViolations...)
	}

	if currentPolicyReport.GetName() == "" && len(clusterViolations) > 0 {
		// If PolicyReport does not exist for cluster -> create it ONLY if there are violations
		createPolicyReport(clusterViolations, data.ClusterInfo, dynamicClient)
	} else if currentPolicyReport.GetName() != "" && len(clusterViolations) > 0 {
		// If PolicyReport exists -> add new violations and remove violations no longer present
		updatePolicyReportViolations(currentPolicyReport, clusterViolations, data.ClusterInfo, dynamicClient)
	} else if currentPolicyReport.GetName() != "" && len(clusterViolations) == 0 {
		// If PolicyReport no longer has violations && No policyresults from grc-> delete PolicyReport for cluster
		deletePolicyReport(data.ClusterInfo, dynamicClient)

	} else if currentPolicyReport.GetName() == "" && len(clusterViolations) == 0 {
		glog.Infof(
			"Cluster %s (%s) is healthy. Skipping PolicyReport creation for this cluster as there are no violations to process.",
			data.ClusterInfo.Namespace,
			data.ClusterInfo.ClusterID,
		)
	}
}

func convertSevFromGovernance(policySev string) string {
	policySev = strings.ToLower(policySev)
	sevMapping := map[string]interface{}{
		"critical": "4",
		"high":     "3",
		"medium":   "2",
		"low":      "1",
	}
	if severity, ok := sevMapping[policySev]; ok {
		return severity.(string)
	}
	return "0"
}

// getGovernanceResults creates a result object for each policy violation in the cluster
func getGovernanceResults(dynamicClient dynamic.Interface, clusterInfo types.ManagedClusterInfo) []*v1alpha2.PolicyReportResult {
	glog.V(1).Infof(
		"Getting policy violations for cluster %s (%s)",
		clusterInfo.Namespace,
		clusterInfo.ClusterID,
	)

	res := dynamicClient.Resource(policyGvr).Namespace(clusterInfo.Namespace)
	policyList, err := res.List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		glog.Warningf(
			"Error getting policy data for cluster %s (%s)",
			clusterInfo.Namespace,
			clusterInfo.ClusterID,
		)
		return []*v1alpha2.PolicyReportResult{}
	}

	var clusterViolations []*v1alpha2.PolicyReportResult

	// Iterate over policies in the cluster
	for _, plc := range policyList.Items {
		// Parse relevant policy fields
		plcName := plc.GetName()
		category := plc.GetAnnotations()["policy.open-cluster-management.io/categories"]
		creationTimestamp, _, err := unstructured.NestedString(plc.Object, "metadata", "creationTimestamp")
		if err != nil {
			glog.Warningf("error parsing creation timestamp as a string for policy %s: %s", plcName, err)
			continue
		}
		details, _, err := unstructured.NestedSlice(plc.Object, "status", "details")
		if err != nil {
			glog.Warningf("error parsing status details as a map for policy %s: %s", plcName, err)
			continue
		}
		compliance, _, err := unstructured.NestedString(plc.Object, "status", "compliant")
		if err != nil {
			glog.Warningf("error parsing status compliance as a string for policy %s: %s", plcName, err)
			continue
		}

		// Only create a violation if the policy is non-compliant
		if compliance != "NonCompliant" {
			continue
		}

		// Generate a cluster policy report result for each non-compliant policy template within the root policy
		for idx, detail := range details {
			var detailMap map[string]interface{}
			var ok bool
			if detailMap, ok = detail.(map[string]interface{}); !ok {
				glog.Warningf("failed to parse history detail as a map for policy %s, template %d", plcName, idx)
				continue
			}

			// Only create a violation if the policy template is non-compliant
			if detailMap["compliant"] != "NonCompliant" {
				continue
			}

			// Parse relevant status fields for the policy template
			plcTemplateName, _, err := unstructured.NestedString(detailMap, "templateMeta", "name")
			if err != nil {
				glog.Warningf(
					"error parsing policy template name as a string for policy %s, template %d: %s", plcName, idx, err)
				continue
			}
			historyItems, _, err := unstructured.NestedSlice(detailMap, "history")
			glog.V(1).Infof("error parsing history as a slice for policy %s, template %d: %s", plcName, idx, err)
			if len(historyItems) == 0 {
				glog.Warningf("history is empty for policy %s, template %d", plcName, idx)
				continue
			}
			message, _, err := unstructured.NestedString(historyItems[0].(map[string]interface{}), "message")
			if err != nil {
				glog.Warningf(
					"error parsing compliance message as a string for policy %s, template %d: %s", plcName, idx, err)
				continue
			}

			// Append violation to policy report results
			clusterViolations = append(clusterViolations, &v1alpha2.PolicyReportResult{
				Policy:      plcName,
				Description: message,
				Scored:      false,
				Category:    category,
				Source:      "grc",
				Timestamp:   metav1.Timestamp{Seconds: time.Now().Unix(), Nanos: int32(time.Now().UnixNano())},
				Result:      "fail",
				Properties: map[string]string{
					"created_at": creationTimestamp,
					"total_risk": convertSevFromGovernance(getSevFromTemplate(plc, plcTemplateName)),
				},
			})
		}
	}
	return clusterViolations
}

// getSevFromTemplate pulls the severity for the specified policy template from the spec
func getSevFromTemplate(plc unstructured.Unstructured, name string) string {
	plcName := plc.GetName()
	// Parse policy templates
	plcTemplates, _, err := unstructured.NestedSlice(plc.Object, "spec", "policy-templates")
	if err != nil {
		glog.Warningf("error parsing policy-templates as a map for policy %s: %s", plcName, err)
		return ""
	}

	for idx, template := range plcTemplates {
		// Find policy template with matching policy name
		objDef, _, err := unstructured.NestedMap(template.(map[string]interface{}), "objectDefinition")
		if err != nil {
			glog.Warningf("error parsing objectDefinition as a map for policy %s, template %d: %s", plcName, idx, err)
			continue
		}
		objDefName, _, err := unstructured.NestedString(objDef, "metadata", "name")
		if err != nil {
			glog.Warningf("error parsing objectDefinition name as a string for policy %s, template %d: %s", plcName, idx, err)
			continue
		}

		// Skip if the name doesn't match
		if objDefName != name {
			continue
		}

		// Check API group
		apiVersion, _, err := unstructured.NestedString(objDef, "apiVersion")
		if err != nil {
			glog.Warningf(
				"error parsing objectDefinition apiVersion as a string for policy %s, template %d: %s", plcName, idx, err)
			break
		}

		// Handle OCM policies
		if strings.Split(apiVersion, "/")[0] == policyGvr.Group {
			severity, _, err := unstructured.NestedString(objDef, "spec", "severity")
			if err != nil {
				glog.Warningf(
					"error parsing severity for policy %s, template %d: %s", plcName, idx, err)
				break
			}

			return severity

			// If this isn't an OCM policy, check for a severity annotation
		} else {
			severityAnnotation, severityFound, err := unstructured.NestedString(
				objDef, "metadata", "annotations", "policy.open-cluster-management.io/severity",
			)
			if !severityFound || err != nil {
				glog.V(1).Infof(
					"error parsing objectDefinition severity annotation as a string for policy %s, template %d: %s", plcName, idx, err)
				break
			}

			return severityAnnotation
		}
	}

	// Return an empty string if the severity wasn't set or the policy wasn't found
	return ""
}

func createPolicyReport(
	clusterViolations []*v1alpha2.PolicyReportResult,
	clusterInfo types.ManagedClusterInfo, dynamicClient dynamic.Interface) {
	glog.V(1).Infof(
		"Starting createPolicyReport for cluster %s (%s)",
		clusterInfo.Namespace,
		clusterInfo.ClusterID,
	)
	// PolicyReport doesnt exist for cluster - creating
	policyreport := &v1alpha2.PolicyReport{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PolicyReport",
			APIVersion: "wgpolicyk8s.io/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterInfo.Namespace + prSuffix,
			Namespace: clusterInfo.Namespace,
		},
		Results: clusterViolations,
		Scope: &corev1.ObjectReference{
			Kind:      "cluster",
			Name:      clusterInfo.Namespace,
			Namespace: clusterInfo.Namespace,
		},
		Summary: v1alpha2.PolicyReportSummary{
			Pass:  0,
			Fail:  len(clusterViolations),
			Warn:  0,
			Error: 0,
			Skip:  0,
		},
	}
	prUnstructured, unstructuredErr := runtime.DefaultUnstructuredConverter.ToUnstructured(policyreport)
	if unstructuredErr != nil {
		glog.Warningf("Error converting to unstructured.Unstructured: %s", unstructuredErr)
	}
	obj := &unstructured.Unstructured{Object: prUnstructured}

	_, err := dynamicClient.Resource(policyReportGvr).Namespace(clusterInfo.Namespace).Create(
		context.TODO(),
		obj,
		metav1.CreateOptions{},
	)

	if err != nil {
		glog.Warningf(
			"Could not create PolicyReport for cluster %s (%s): %v",
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

func updatePolicyReportViolations(
	currentPolicyReport v1alpha2.PolicyReport,
	clusterViolations []*v1alpha2.PolicyReportResult,
	clusterInfo types.ManagedClusterInfo, dynamicClient dynamic.Interface) {
	glog.V(2).Infof(
		"Starting updatePolicyReportViolations for cluster %s (%s)",
		clusterInfo.Namespace,
		clusterInfo.ClusterID,
	)
	// merge existing PolicyReport results with new results
	currentPolicyReport.Results = clusterViolations
	currentPolicyReport.SetManagedFields(nil)
	currentPolicyReport.Summary.Fail = len(clusterViolations)
	data, marshalErr := json.Marshal(currentPolicyReport)
	if marshalErr != nil {
		glog.Warningf("Error Marshalling PolicyReport patch object for cluster %s: %v", clusterInfo.Namespace, marshalErr)
	}

	forcePatch := true
	successPatchRes, err := dynamicClient.Resource(policyReportGvr).Namespace(clusterInfo.Namespace).Patch(
		context.TODO(),
		clusterInfo.Namespace+prSuffix,
		k8sTypes.ApplyPatchType,
		data,
		metav1.PatchOptions{
			FieldManager: "insights-client",
			Force:        &forcePatch,
		},
	)

	if successPatchRes != nil && err == nil {
		unstructConvErr := runtime.DefaultUnstructuredConverter.FromUnstructured(
			successPatchRes.UnstructuredContent(),
			&currentPolicyReport,
		)
		if unstructConvErr != nil {
			glog.Warningf("Error unstructuring PolicyReport for cluster: %s", clusterInfo.Namespace)
		}
	}

	if err != nil {
		glog.Infof(
			"Error updating Insights for cluster %s (%s): %v",
			clusterInfo.Namespace,
			clusterInfo.ClusterID,
			err,
		)
	} else {
		glog.Infof(
			"Successfully updated Insights for cluster %s (%s)",
			clusterInfo.Namespace,
			clusterInfo.ClusterID,
		)
	}
}

func deletePolicyReport(clusterInfo types.ManagedClusterInfo, dynamicClient dynamic.Interface) {
	glog.V(2).Infof(
		"Starting deletePolicyReport for cluster %s (%s)",
		clusterInfo.Namespace,
		clusterInfo.ClusterID,
	)
	deleteErr := dynamicClient.Resource(policyReportGvr).Namespace(clusterInfo.Namespace).Delete(
		context.TODO(),
		clusterInfo.Namespace+prSuffix,
		metav1.DeleteOptions{},
	)

	if deleteErr != nil {
		glog.Warningf("Error deleting PolicyReport for cluster %s (%s): %v",
			clusterInfo.Namespace,
			clusterInfo.ClusterID,
			deleteErr,
		)
	} else {
		glog.Infof("Successfully deleted PolicyReport for cluster %s (%s)",
			clusterInfo.Namespace,
			clusterInfo.ClusterID,
		)
	}
}

// ProcessPolicyReports ...
func (p *Processor) ProcessPolicyReports(input chan types.ProcessorData, dynamicClient dynamic.Interface) {
	for {
		p.createUpdatePolicyReports(input, dynamicClient)
	}
}

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
	"github.com/open-cluster-management/insights-client/pkg/retriever"
	"github.com/open-cluster-management/insights-client/pkg/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sTypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/wg-policy-prototypes/policy-report/api/v1alpha2"
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
		if reportContentData != nil && !strings.Contains(report.Component, "ccx_rules_ocm.tutorial_rule") {
			var contentData types.FormattedContentData
			reportContentDataBytes, _ := json.Marshal(reportContentData)
			unmarshalError := json.Unmarshal(reportContentDataBytes, &contentData)
			if unmarshalError == nil {
				clusterViolations = append(clusterViolations, &v1alpha2.PolicyReportResult{
					Policy:      report.Key,
					Description: contentData.Description,
					Scored:      false,
					Category:    FilterOpenshiftCategory(contentData.Tags),
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
		data.ClusterInfo.Namespace + prSuffix,
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

	if currentPolicyReport.GetName() == "" && len(clusterViolations) > 0 {
		// If PolicyReport does not exist for cluster -> create it ONLY if there are violations
		createPolicyReport(clusterViolations, data.ClusterInfo, dynamicClient)
	} else if currentPolicyReport.GetName() != "" && len(clusterViolations) > 0 {
		// If PolicyReport exists -> add new violations and remove violations no longer present
		updatePolicyReportViolations(currentPolicyReport, clusterViolations, data.ClusterInfo, dynamicClient)
	} else if currentPolicyReport.GetName() != "" && len(clusterViolations) == 0 {
		// If PolicyReport no longer has violations -> delete PolicyReport for cluster
		deletePolicyReport(data.ClusterInfo, dynamicClient)
	} else if currentPolicyReport.GetName() == "" && len(clusterViolations) == 0 {
		glog.Infof(
			"Cluster %s (%s) is healthy. Skipping PolicyReport creation for this cluster as there are no violations to process.",
			data.ClusterInfo.Namespace,
			data.ClusterInfo.ClusterID,
		)
	}
}

func createPolicyReport(
	clusterViolations []*v1alpha2.PolicyReportResult,
	clusterInfo types.ManagedClusterInfo, dynamicClient dynamic.Interface) {
	glog.V(2).Infof(
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
			Kind: "cluster",
			Name: clusterInfo.Namespace,
			Namespace: clusterInfo.Namespace,
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
		glog.Infof(
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
	data, marshalErr := json.Marshal(currentPolicyReport)
	if marshalErr != nil {
		glog.Warningf("Error Marshalling PolicyReport patch object for cluster %s: %v", clusterInfo.Namespace, marshalErr)
	}

	forcePatch := true
	successPatchRes, err := dynamicClient.Resource(policyReportGvr).Namespace(clusterInfo.Namespace).Patch(
		context.TODO(),
		clusterInfo.Namespace + prSuffix,
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
		clusterInfo.Namespace + prSuffix,
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

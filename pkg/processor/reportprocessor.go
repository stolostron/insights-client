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
    "k8s.io/client-go/rest"
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

func indexOf(ruleID string, prResults []types.PolicyReportResultData) (int) {
    for idx, value := range prResults {
        if ruleID == value.Properties["component"] {
            return idx
        }
    }
    return -1    //not found.
 }

// NewProcessor ...
func NewProcessor() *Processor {
    p := &Processor{}
    return p
}

// CreateUpdatePolicyReports ...
func (p *Processor) CreateUpdatePolicyReports(input chan types.ProcessorData, ret *retriever.Retriever, hubID string) {
    for {
        data := <-input

        cfg := config.GetConfig()
        restClient := config.RESTClient(cfg)
        getResp := restClient.Get().
            Resource("policyreports").
            Namespace(data.ClusterInfo.Namespace).
            Name(data.ClusterInfo.Namespace).
            Do(context.TODO())
    
        respBytes, _ := getResp.Raw()
        var prResponse types.PolicyReportGetResponse
        unmarshalError := json.Unmarshal(respBytes, &prResponse)
        if unmarshalError != nil {
            glog.Infof(
                "Error unmarshalling PolicyReport for cluster %s: %v",
                data.ClusterInfo.Namespace,
                unmarshalError,
            )
        } else {
            // Create a PolicyReport for the cluster
            createPolicyReport(restClient, data.Reports.Reports, prResponse, data.ClusterInfo, ret, hubID)

            if len(prResponse.Results) > 0 {
                // Update any existing PolicyReports that have been resolved
                updateViolationSkips(restClient, data.Reports.Skips, prResponse, data.ClusterInfo.Namespace)
            }
        }
    }
}

// createPolicyReport ...
func createPolicyReport(
    restClient *rest.RESTClient,
    reports []types.ReportData,
    prResponse types.PolicyReportGetResponse,
    clusterInfo types.ManagedClusterInfo,
    ret *retriever.Retriever,
    hubID string,
) {
    var policyReportResults []*v1alpha2.PolicyReportResult
    for _, report := range reports {
        // Find the correct Insight content data from cache
        reportData := retriever.ContentsMap[report.Key]
        ruleIndex := indexOf(report.Component, prResponse.Results)
        if (reportData == nil) {
            glog.Infof(
                "Could not find the content data for rule %s - Refreshing content list",
                report.Component,
            )
            ret.InitializeContents(hubID)
        } else if reportData != nil && ruleIndex == -1 {
            var contentData types.FormattedContentData
            reportDataBytes, _ := json.Marshal(reportData)
            unmarshalError := json.Unmarshal(reportDataBytes, &contentData)
            if unmarshalError == nil {
                policyReportResults = append(policyReportResults, &v1alpha2.PolicyReportResult{
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
                        "reason":     contentData.Reason, // Need to figure out where to store this value outside of the PR
                        "resolution": contentData.Resolution, // Need to figure out where to store this value outside of the PR
                        "rule_id":    report.RuleID,
                        "component":  report.Component,
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

    if prResponse.Meta.Name == "" {
        // PolicyReport doesnt exist for cluster - creating
        policyreport := &v1alpha2.PolicyReport{
            ObjectMeta: metav1.ObjectMeta{
                Name:      clusterInfo.Namespace,
                Namespace: clusterInfo.Namespace,
            },
            Results: policyReportResults,
        }

        postResp := restClient.Post().
            Resource("policyreports").
            Namespace(clusterInfo.Namespace).
            Body(policyreport).
            Do(context.TODO())

        if postResp.Error() != nil {
            glog.Infof(
                "Error creating PolicyReport for cluster %s (%s): %v",
                clusterInfo.Namespace,
                clusterInfo.ClusterID,
                postResp.Error(),
            )
        } else {
            glog.Infof(
                "Successfully created PolicyReport for cluster %s (%s)",
                clusterInfo.Namespace,
                clusterInfo.ClusterID,
            )
        }
    } else if prResponse.Meta.Name != "" && len(policyReportResults) > 0 {
        // PolicyReport exists on cluster - Updating the rule violations
        payload := []patchPRResultsValue{{
            Op:    "replace",
            Path:  "/results",
            Value: policyReportResults,
        }}
        payloadBytes, _ := json.Marshal(payload)

        resp := restClient.Patch(k8sTypes.JSONPatchType).
            Resource("policyreports").
            Namespace(clusterInfo.Namespace).
            Name(clusterInfo.Namespace).
            Body(payloadBytes).
            Do(context.TODO())

        if resp.Error() != nil {
            glog.Infof(
                "Error updating PolicyReport data for cluster %s (%s): %v",
                clusterInfo.Namespace,
                clusterInfo.ClusterID,
                resp.Error(),
            )
        } else {
            glog.Infof(
                "Successfully updated PolicyReport data for cluster %s (%s)",
                clusterInfo.Namespace,
                clusterInfo.ClusterID,
            )
        }
    }
}

// updateViolationSkips - Updates status to "skip" for all violations that have been resolved
func updateViolationSkips(
    restClient *rest.RESTClient,
    skippedReports []types.SkippedReports,
    prResponse types.PolicyReportGetResponse,
    clusterNamespace string,
) {
    var payload []patchStringValue
    for _, rule := range skippedReports {
        for idx, resultRule := range prResponse.Results {
            if prResponse.Meta.Name != "" && resultRule.Status != "skip" && rule.RuleID == resultRule.Properties["Component"] {
                glog.Infof("PolicyReport violation %s has been resolved, updating status to skip", rule.RuleID)
                payload = append(payload, patchStringValue{
                    Op:    "replace",
                    Path:  "/results/" + strconv.Itoa(idx) + "/result",
                    Value: "skip",
                })
            }
        }
    }

    if len(payload) > 0 {
        payloadBytes, _ := json.Marshal(payload)
        resp := restClient.Patch(k8sTypes.JSONPatchType).
            Resource("policyreports").
            Namespace(clusterNamespace).
            Name(clusterNamespace).
            Body(payloadBytes).
            Do(context.TODO())

        if resp.Error() != nil {
            glog.Infof(
                "Error updating PolicyReport statuses for cluster %s: %v",
                clusterNamespace,
                resp.Error(),
            )
        } else {
            glog.Infof(
                "Successfully updated PolicyReport statuses for cluster %s",
                clusterNamespace,
            )
        }
    }
}


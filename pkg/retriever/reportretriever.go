// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package retriever

import (
	"bytes"
	"context"
	"encoding/json"
	e "errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"strconv"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/open-cluster-management/insights-client/pkg/config"
	"github.com/open-cluster-management/insights-client/pkg/types"
	"k8s.io/apimachinery/pkg/api/errors"
	k8sTypes "k8s.io/apimachinery/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/wg-policy-prototypes/policy-report/api/v1alpha1"
)

// Retriever struct
type Retriever struct {
	CCXUrl                  string
	ContentURL              string
	Client                  *http.Client
	Token                   string // token to connect to CRC
	TokenValidationInterval time.Duration
}

type serializedAuthMap struct {
	Auths map[string]serializedAuth `json:"auths"`
}
type serializedAuth struct {
	Auth string `json:"auth"`
}

//  patchUint32Value specifies a patch operation for a uint32.
type patchStringValue struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value string `json:"value"`
}

var contentsMap map[string]map[string]interface{}
var lock = sync.RWMutex{}

// NewRetriever ...
func NewRetriever(ccxurl string, ContentURL string, client *http.Client,
	tokenValidationInterval time.Duration, token string) *Retriever {
	if client == nil {
		client = &http.Client{}
	}
	r := &Retriever{
		TokenValidationInterval: tokenValidationInterval,
		Client:                  client,
		CCXUrl:                  ccxurl,
		ContentURL:              ContentURL,
	}
	if token == "" {
		r.setUpRetriever()
	} else {
		r.Token = token
	}
	return r
}

// Get CRC token , wait until we can get token
func (r *Retriever) setUpRetriever() {
	err := r.StartTokenRefresh()
	for err != nil {
		glog.Warningf("Unable to get CRC Token: %v", err)
		time.Sleep(5 * time.Second)
	}
}

// StartTokenRefresh sets the CRC token for use in Insights queries
func (r *Retriever) StartTokenRefresh() error {
	glog.Infof("Refreshing CRC credentials  ")
	secret, err := config.GetKubeClient().CoreV1().Secrets("openshift-config").
		Get(context.TODO(), "pull-secret", metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			glog.V(2).Infof("pull-secret does not exist")
			err = nil
		} else if errors.IsForbidden(err) {
			glog.V(2).Infof("Operator does not have permission to check pull-secret: %v", err)
			err = nil
		} else {
			err = fmt.Errorf("could not check pull-secret: %v", err)
		}
		return err
	}
	if secret != nil {
		if data := secret.Data[".dockerconfigjson"]; len(data) > 0 {
			var pullSecret serializedAuthMap
			if err := json.Unmarshal(data, &pullSecret); err != nil {
				glog.Errorf("Unable to unmarshal cluster pull-secret: %v", err)
			}
			if auth, ok := pullSecret.Auths["cloud.openshift.com"]; ok {
				token := strings.TrimSpace(auth.Auth)
				if strings.Contains(token, "\n") || strings.Contains(token, "\r") {
					return fmt.Errorf("cluster authorization token is not valid: contains newlines")
				}
				if len(token) > 0 {
					glog.V(2).Info("Found cloud.openshift.com token ")
					r.Token = token
				}
			}
		}
	}
	return nil
}

// RetrieveCCXReport ...
func (r *Retriever) RetrieveCCXReport(input chan types.ManagedClusterInfo) {
	for {
		cluster := <-input
		// If the cluster id is empty do nothing
		if cluster.Namespace == "" || cluster.ClusterID == "" {
			return
		}

		glog.Infof("RetrieveCCXReport for cluster %s", cluster.Namespace)
		req, err := r.GetInsightsRequest(context.TODO(), r.CCXUrl, cluster)
		if err != nil {
			glog.Warningf("Error creating HttpRequest for cluster %s (%s), %v", cluster.Namespace, cluster.ClusterID, err)
			continue
		}
		response, err := r.CallInsights(req, cluster)
		if err != nil {
			glog.Warningf("Error sending HttpRequest for cluster %s (%s), %v", cluster.Namespace, cluster.ClusterID, err)
			continue
		}

		err = r.GetPolicyInfo(response, cluster)
		if err != nil {
			glog.Warningf("Error creating PolicyInfo for cluster %s (%s), %v", cluster.Namespace, cluster.ClusterID, err)
			continue
		}
	}
}

// GetInsightsRequest ...
func (r *Retriever) GetInsightsRequest(
	ctx context.Context,
	endpoint string,
	cluster types.ManagedClusterInfo,
) (*http.Request, error) {
	glog.Infof(
		"Creating Request for cluster %s (%s) using Insights URL %s",
		cluster.Namespace,
		cluster.ClusterID,
		r.CCXUrl,
	)
	reqCluster := types.PostBody{
		Clusters: []string{
			cluster.ClusterID,
		},
	}
	reqBody, _ := json.Marshal(reqCluster)
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		glog.Warningf("Error creating HttpRequest for cluster %s (%s), %v", cluster.Namespace, cluster.ClusterID, err)
		return nil, err
	}
	// userAgent for value will be updated to insights-client once the
	// the task https://github.com/RedHatInsights/insights-results-smart-proxy/issues/450
	// is completed
	userAgent := "insights-operator/v1.0.0+b653953-b653953ed174001d5aca50b3515f1fa6f6b28728 cluster/" + cluster.ClusterID
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Authorization", "Bearer "+r.Token)
	return req, nil
}

// CallInsights ...
func (r *Retriever) CallInsights(req *http.Request, cluster types.ManagedClusterInfo) (types.ResponseBody, error) {
	glog.Infof("Calling Insights for cluster %s (%s)", cluster.Namespace, cluster.ClusterID)
	var responseBody types.ResponseBody
	res, err := r.Client.Do(req)
	if err != nil {
		glog.Warningf("Error sending HttpRequest for cluster %s (%s), %v", cluster.Namespace, cluster.ClusterID, err)
		return types.ResponseBody{}, err
	}
	if res.StatusCode != 200 {
		glog.Warningf(
			"Response Code error for cluster %s (%s), response code %d",
			cluster.Namespace,
			cluster.ClusterID,
			res.StatusCode,
		)
		return types.ResponseBody{}, e.New("No Success HTTP Response code ")
	}
	defer res.Body.Close()
	data, _ := ioutil.ReadAll(res.Body)
	// unmarshal response data into the ResponseBody struct
	unmarshalError := json.Unmarshal(data, &responseBody)
	if unmarshalError != nil {
		glog.Errorf("Error unmarshalling ResponseBody %v", unmarshalError)
		return types.ResponseBody{}, unmarshalError
	}
	return responseBody, err
}

// GetPolicyInfo ...
func (r *Retriever) GetPolicyInfo(
	responseBody types.ResponseBody,
	cluster types.ManagedClusterInfo,
) (error) {
	glog.Infof("Creating Policy Info for cluster %s (%s)", cluster.Namespace, cluster.ClusterID)
	reports := types.Reports{}

	// loop through the clusters in the response and pull out the report violations
	for reportClusterID := range responseBody.Reports {
		if reportClusterID == cluster.ClusterID {
			// convert report data into []byte
			reportBytes, _ := json.Marshal(responseBody.Reports[reportClusterID])
			// unmarshal response data into the Report struct
			unmarshalError := json.Unmarshal(reportBytes, &reports)
			if unmarshalError != nil {
				glog.Infof(
					"Error unmarshalling Policy %v for cluster %s (%s)",
					unmarshalError,
					cluster.Namespace,
					cluster.ClusterID,
				)
				return unmarshalError
			}
			// Loop through Report array and create a PolicyReport for each violation
			for _, report := range reports.Reports {
				// Find the correct Insight content data from cache
				reportData := contentsMap[report.Key]
				if reportData != nil {
					var contentData types.FormattedContentData // TODO neeed to update this type for new content
					reportDataBytes, _ := json.Marshal(reportData)
					unmarshalError := json.Unmarshal(reportDataBytes, &contentData)
					if unmarshalError != nil {
						glog.Infof(
							"Error unmarshalling Report %v for cluster %s (%s)",
							unmarshalError,
							cluster.Namespace,
							cluster.ClusterID,
						)
						return unmarshalError
					}
					createPolicyReport(contentData, report, cluster)
				}
			}

			// Update any existing PolicyReports that have been resolved
			updatePolicyReports(reports.Skips, cluster.Namespace)
		}
	}
	return nil
}

// createUpdatePolicyReport ...
func createPolicyReport(
	policyReport types.FormattedContentData,
	report types.ReportData,
	cluster types.ManagedClusterInfo,
) {
	cfg := config.GetConfig()
	restClient := config.RESTClient(cfg)
	// PolicyReport name must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character
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
	json.Unmarshal(respBytes, &prResponse)

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
					"total_risk": strconv.Itoa(policyReport.Likelihood), // TODO total_risk is no longer available, need to sync with CCX team to determine best route here
					"reason":     policyReport.Reason,
					"resolution": policyReport.Resolution,
				},
			}},
		}

		postResp := restClient.Post().
			Namespace(cluster.Namespace).
			Resource("policyreports").
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
		// TODO: Rule violation has returned need to update the status to error again.
		glog.Info("PolicyReport %s has been reintroduced, updating status to error")
		payload := []patchStringValue{{
			Op:    "replace",
			Path:  "/results/0/status",
			Value: "error",
		}}
		payloadBytes, _ := json.Marshal(payload)

		resp := restClient.Patch(k8sTypes.JSONPatchType).
			Resource("policyreports").
			Namespace(cluster.Namespace).
			Name(report.Component).
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

// updatePolicyReports - Updates status to "skip" for all PolicyReports present in a namespace -> this means the PolicyReport has been resolved
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
		json.Unmarshal(respBytes, &prResponse)

		if (prResponse.Meta.Name != "") {
			// Patch PolicyReport: status -> skip as the violation has been resolved
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

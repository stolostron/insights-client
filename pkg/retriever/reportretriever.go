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
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/open-cluster-management/insights-client/pkg/config"
	"github.com/open-cluster-management/insights-client/pkg/types"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Retriever struct
type Retriever struct {
	CCXUrl                  string
	ContentUrl              string
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

var contentsMap map[string]map[string]interface{}
var lock = sync.RWMutex{}

// NewRetriever ...
func NewRetriever(ccxurl string, contentUrl string, client *http.Client,
	tokenValidationInterval time.Duration, token string) *Retriever {
	if client == nil {
		client = &http.Client{}
	}
	r := &Retriever{
		TokenValidationInterval: tokenValidationInterval,
		Client:                  client,
		CCXUrl:                  ccxurl,
		ContentUrl:              contentUrl,
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
func (r *Retriever) RetrieveCCXReport(input chan types.ManagedClusterInfo, output chan types.PolicyInfo) {
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

		policyInfo, err := r.GetPolicyInfo(response, cluster)
		if err != nil {
			glog.Warningf("Error creating PolicyInfo for cluster %s (%s), %v", cluster.Namespace, cluster.ClusterID, err)
			continue
		}

		output <- policyInfo
	}
}

// GetInsightsRequest ...
func (r *Retriever) GetInsightsRequest(ctx context.Context, endpoint string, cluster types.ManagedClusterInfo) (*http.Request, error) {
	glog.Infof("Creating Request for cluster %s (%s) using Insights URL %s", cluster.Namespace, cluster.ClusterID, r.CCXUrl)
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
		glog.Warningf("Response Code error for cluster %s (%s), response code %d", cluster.Namespace, cluster.ClusterID, res.StatusCode)
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
func (r *Retriever) GetPolicyInfo(responseBody types.ResponseBody, cluster types.ManagedClusterInfo) (types.PolicyInfo, error) {
	glog.Infof("Creating Policy Info for cluster %s (%s)", cluster.Namespace, cluster.ClusterID)
	var policy types.Policy
	policyInfo := types.PolicyInfo{}

	// loop through the clusters in the response and create the PolicyReport node for each violation
	for clusterReport := range responseBody.Reports {
		if clusterReport == cluster.ClusterID {
			// convert report data into []byte
			reportBytes, _ := json.Marshal(responseBody.Reports[clusterReport])
			// unmarshal response data into the Policy struct
			unmarshalError := json.Unmarshal(reportBytes, &policy)

			if unmarshalError != nil {
				glog.Infof("Error unmarshalling Policy %v for cluster %s (%s)", unmarshalError, cluster.Namespace, cluster.ClusterID)
				return policyInfo, unmarshalError
			}
			policyInfo = types.PolicyInfo{Policy: policy, ClusterId: clusterReport}
		}
	}
	return policyInfo, nil
}

// Creates GET request for contents
func (r *Retriever) GetContentRequest(ctx context.Context, clusterId string) (*http.Request, error) {
	glog.Infof("Creating Content Request for cluster %s using  URL %s :", clusterId, r.ContentUrl)
	req, err := http.NewRequest("GET", r.ContentUrl, nil)
	if err != nil {
		glog.Warningf("Error creating HttpRequest with endpoint %s, %v", r.ContentUrl, err)
		return nil, err
	}
	// userAgent for value will be updated to insights-client once the
	// the task https://github.com/RedHatInsights/insights-results-smart-proxy/issues/450
	// is completed
	userAgent := "insights-operator/v1.0.0+b653953-b653953ed174001d5aca50b3515f1fa6f6b28728 cluster/" + clusterId
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Authorization", "Bearer "+r.Token)
	return req, nil
}

// Makes HTTP call to get contents
func (r *Retriever) CallContents(req *http.Request) (types.ContentsResponse, error) {
	glog.Infof("Making GET call for  Contents .")
	var responseBody types.ContentsResponse
	res, err := r.Client.Do(req)
	if err != nil {
		glog.Warningf("Error sending HttpRequest for contents %v", err)
		return types.ContentsResponse{}, err
	}
	if res.StatusCode != 200 {
		glog.Warningf("Unsucessful response during contents GET - response code %d", res.StatusCode)
		return types.ContentsResponse{}, e.New("No Success HTTP Response code ")
	}
	defer res.Body.Close()
	data, _ := ioutil.ReadAll(res.Body)
	// unmarshal response data into the ResponseBody struct
	unmarshalError := json.Unmarshal(data, &responseBody)
	if unmarshalError != nil {
		glog.Errorf("Error unmarshalling ResponseBody %v", unmarshalError)
		return types.ContentsResponse{}, unmarshalError
	}
	return responseBody, err
}

// Function to make a GET HTTP call to get all the contents for reports
func (r *Retriever) retrieveCCXContent(clusterId string) int {
	req, err := r.GetContentRequest(context.TODO(), clusterId)
	if err != nil {
		glog.Warningf("Error creating HttpRequest with endpoint %s, %v", r.ContentUrl, err)
		return -1
	}
	contents, err := r.CallContents(req)
	if err != nil {
		glog.Warningf("Error calling for contents %s, %v", clusterId, err)
		return -1
	}
	r.createContents(contents)
	return len(contentsMap)
}

func (r *Retriever) InitializeContents(hubId string) int {
	return r.retrieveCCXContent(hubId)
}

// Populate json response from /contents call onto a Map to quick lookup
func (r *Retriever) createContents(responseBody types.ContentsResponse) {
	glog.Infof("Creating Contents from json ")
	contentsMap = make(map[string]map[string]interface{})

	for content := range responseBody.Content {
		for errorName, errorVal := range responseBody.Content[content].Error_keys {
			errorMap := make(map[string]interface{})
			errorMap["summary"] = responseBody.Content[content].Summary
			errorMap["reason"] = responseBody.Content[content].Reason
			errorMap["resolution"] = responseBody.Content[content].Resolution
			errorVals := errorVal.(map[string]interface{})
			for key, val := range errorVals {
				if key == "metadata" {
					errorMap = r.getErrorKey(val, errorMap)
				} else {
					errorMap[key] = val
				}
			}
			lock.Lock()
			contentsMap[errorName] = errorMap
			lock.Unlock()
		}
	}
}

// Helper function to populate metadata interface{}
func (r *Retriever) getErrorKey(metadata interface{}, errorMap map[string]interface{}) map[string]interface{} {
	metatype := metadata.(map[string]interface{})
	for mkey, mval := range metatype {
		errorMap[mkey] = mval
	}
	return errorMap
}

//  Given Error_Key and field name returns value
func (r *Retriever) GetContents(errorKey string, key string) interface{} {
	lock.RLock()
	defer lock.RUnlock()
	return contentsMap[errorKey][key]
}

//  Given Error_Key gives the fields is has
func (r *Retriever) GetFields(errorKey string) []string {
	lock.RLock()
	defer lock.RUnlock()
	var fields []string
	for mkey := range contentsMap[errorKey] {
		fields = append(fields, mkey)
	}
	return fields
}

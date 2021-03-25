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

func NewRetriever(ccxurl string, contentUrl string, client *http.Client, tokenValidationInterval time.Duration, token string) *Retriever {
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
		if err := r.StartTokenRefresh(); err != nil {
			glog.Warningf("Unable to get CRC Token: %v", err)
		}
	} else {
		r.Token = token
	}

	return r
}

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

func (r *Retriever) RetrieveCCXReport(input chan string, output chan types.PolicyInfo) {
	for {

		clusterId := <-input
		// If the cluster id is empty do nothing
		if clusterId == "" {
			return
		} else {
			glog.Infof("RetrieveCCXReport for cluster %s", clusterId)
		}
		req, err := r.GetInsightsRequest(context.TODO(), r.CCXUrl, clusterId)
		if err != nil {
			glog.Warningf("Error creating HttpRequest for cluster %s, %v", clusterId, err)
			continue
		}
		response, err := r.CallInsights(req, clusterId)
		if err != nil {
			glog.Warningf("Error sending HttpRequest for cluster %s, %v", clusterId, err)
			continue
		}

		policyInfo, err := r.GetPolicyInfo(response, clusterId)
		if err != nil {
			glog.Warningf("Error creating PolicyInfo for cluster %s, %v", clusterId, err)
			continue
		}

		output <- policyInfo
	}
}

func (r *Retriever) GetInsightsRequest(ctx context.Context, endpoint string, clusterId string) (*http.Request, error) {
	glog.Infof("Creating Request for cluster %s using Insights URL %s :", clusterId, r.CCXUrl)
	mockClusters := types.PostBody{
		Clusters: []string{
			clusterId,
		},
	}
	reqBody, _ := json.Marshal(mockClusters)
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		glog.Warningf("Error creating HttpRequest for cluster %s, %v", clusterId, err)
		return nil, err
	}
	userAgent := "insights-operator/v1.0.0+b653953-b653953ed174001d5aca50b3515f1fa6f6b28728 cluster/" + clusterId
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Authorization", "Bearer "+r.Token)
	return req, nil
}

func (r *Retriever) CallInsights(req *http.Request, clusterId string) (types.ResponseBody, error) {
	glog.Infof("Calling Insights for cluster %s ", clusterId)
	var responseBody types.ResponseBody
	res, err := r.Client.Do(req)
	if err != nil {
		glog.Warningf("Error sending HttpRequest for cluster %s, %v", clusterId, err)
		return types.ResponseBody{}, err
	}
	if res.StatusCode != 200 {
		glog.Warningf("Response Code error for cluster %s, response code %d", clusterId, res.StatusCode)
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

func (r *Retriever) GetPolicyInfo(responseBody types.ResponseBody, clusterId string) (types.PolicyInfo, error) {
	glog.Infof("Creating Policy Info for cluster %s ", clusterId)
	var policy types.Policy
	policyInfo := types.PolicyInfo{}

	// loop through the clusters in the response and create the PolicyReport node for each violation
	for cluster := range responseBody.Reports {
		if cluster == clusterId {
			// convert report data into []byte
			reportBytes, _ := json.Marshal(responseBody.Reports[cluster])
			// unmarshal response data into the Policy struct
			unmarshalError := json.Unmarshal(reportBytes, &policy)

			if unmarshalError != nil {
				glog.Infof("Error unmarshalling Policy %v for cluster %s ", unmarshalError, clusterId)
				return policyInfo, unmarshalError
			}
			policyInfo = types.PolicyInfo{Policy: policy, ClusterId: cluster}
		}
	}
	return policyInfo, nil
}

func (r *Retriever) GetContentRequest(ctx context.Context, clusterId string) (*http.Request, error) {
	glog.Infof("Creating Content Request for cluster %s using  URL %s :", clusterId, r.ContentUrl)
	req, err := http.NewRequest("GET", r.ContentUrl, nil)
	if err != nil {
		glog.Warningf("Error creating HttpRequest with endpoint %s, %v", r.ContentUrl, err)
		return nil, err
	}
	userAgent := "insights-operator/v1.0.0+b653953-b653953ed174001d5aca50b3515f1fa6f6b28728 cluster/" + clusterId
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Authorization", "Bearer "+r.Token)
	return req, nil
}

func (r *Retriever) CallContents(req *http.Request) (types.ContentsResponse, error) {
	glog.Infof("Making GET call for  Contents ")
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

func (r *Retriever) RetrieveCCXContent(clusterId string) {
	req, err := r.GetContentRequest(context.TODO(), clusterId)
	if err != nil {
		glog.Warningf("Error creating HttpRequest with endpoint %s, %v", r.ContentUrl, err)
	}
	contents, err := r.CallContents(req)
	if err != nil {
		glog.Warningf("Error sending HttpRequest for contents %s, %v", clusterId, err)
	}

	err = r.CreateContents(contents)
	if err != nil {
		glog.Warningf("Error creating contents %v", err)
	}
	r.print()
}

func (r *Retriever) CreateContents(responseBody types.ContentsResponse) error {
	glog.Infof("Creating Contents from json ")
	contentsMap = make(map[string]map[string]interface{})

	//policyInfo := types.PolicyInfo{}

	// loop through the contents in the response and create a cache to lookup
	for content := range responseBody.Content {
		for error_name, error_val := range responseBody.Content[content].Error_keys {
			errorMap := make(map[string]interface{})
			errorMap["summary"] = responseBody.Content[content].Summary
			errorMap["reason"] = responseBody.Content[content].Reason
			errorMap["resolution"] = responseBody.Content[content].Resolution
			error_vals := error_val.(map[string]interface{})
			for key, val := range error_vals {
				glog.Infof("X %s", error_name)

				if key == "metadata" {
					errorMap = r.getErrorKey(val, errorMap)
				} else {
					errorMap[key] = val
					glog.Infof("%s -> %v", key, val)
				}

			}
			lock.Lock()
			contentsMap[error_name] = errorMap
			lock.Unlock()
		}
	}
	return nil
}

func (r *Retriever) getErrorKey(matadata interface{}, errorMap map[string]interface{}) map[string]interface{} {
	matatype := matadata.(map[string]interface{})
	for mkey, mval := range matatype {
		errorMap[mkey] = mval
		glog.Infof("%s => %v", mkey, mval)
	}
	return errorMap
}

func (r *Retriever) print() {
	for mkey, mval := range contentsMap {

		glog.Infof("**** %s ", mkey)
		for nkey := range mval {
			glog.Infof("%%%% %s ", nkey)
			glog.Infof("%%%% %s ", mval["summary"])
		}
	}
}
func (r *Retriever) GetContents(errorKey string, key string) interface{} {
	lock.RLock()
	defer lock.RUnlock()
	return contentsMap[errorKey][key]
}

func (r *Retriever) GetFields(errorKey string) []string {
	lock.RLock()
	defer lock.RUnlock()
	var fields []string
	for mkey := range contentsMap[errorKey] {
		fields = append(fields, mkey)
	}
	return fields
}

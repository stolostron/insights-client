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
	"time"

	"github.com/golang/glog"
	"github.com/open-cluster-management/insights-client/pkg/config"
	"github.com/open-cluster-management/insights-client/pkg/types"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Retriever struct {
	CCXUrl                  string
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

func NewRetriever(ccxurl string, client *http.Client, tokenValidationInterval time.Duration, token string) *Retriever {
	if client == nil {
		client = &http.Client{}
	}
	r := &Retriever{
		TokenValidationInterval: tokenValidationInterval,
		Client:                  client,
		CCXUrl:                  ccxurl,
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

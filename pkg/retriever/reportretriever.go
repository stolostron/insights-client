// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
package retriever

import (
	"bytes"
	"context"
	"encoding/json"
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
	client                  *http.Client
	token                   string // token to connect to CRC
	tokenValidationInterval time.Duration
}

type serializedAuthMap struct {
	Auths map[string]serializedAuth `json:"auths"`
}
type serializedAuth struct {
	Auth string `json:"auth"`
}

func NewRetriever() *Retriever {
	r := &Retriever{
		tokenValidationInterval: 5 * time.Second,
	}
	r.client = &http.Client{}
	if err := r.setUpRetriever(); err != nil {
		glog.Warningf("Unable to update mnaged clusters: %v", err)
	}
	return r
}

func (r *Retriever) setUpRetriever() error {
	r.CCXUrl = config.Cfg.CCXServer
	if !config.Cfg.UseMock {
		go func() {
			ticker := time.NewTicker(r.tokenValidationInterval)
			for ; true; <-ticker.C {
				r.StartTokenRefresh()
			}
		}()
	}
	return nil
}

func (c *Retriever) StartTokenRefresh() error {
	glog.Infof("Refreshing CRC credentials  ")
	secret, err := config.GetKubeClient().CoreV1().Secrets("openshift-config").Get(context.TODO(), "pull-secret", metav1.GetOptions{})
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
					c.token = token
				}
			}
		}
	}
	return nil
}

type PostBody struct {
	Clusters []string `json:"clusters"`
}

func (r *Retriever) RetrieveCCXReport(input chan string, output chan types.PolicyInfo) {
	for {
		var responseBody types.ResponseBody
		var policy types.Policy
		clusterId := <-input
		// If the cluster id is empty do nothing
		if clusterId == "" {
			return
		}
		ua := "insights-operator/v1.0.0+alpha cluster/" + clusterId
		glog.Infof("Start Retrieving CCXReport for cluster %s", clusterId)
		glog.V(2).Info("Insights using URL :", string(r.CCXUrl))
		mockClusters := PostBody{
			Clusters: []string{
				clusterId,
			},
		}
		reqBody, _ := json.Marshal(mockClusters)

		req, err := http.NewRequest("POST", r.CCXUrl, bytes.NewBuffer(reqBody))
		if err != nil {
			glog.Infof("Error creating HttpRequest %v", err)
			continue
		}
		req.Header.Add("Content-Type", "application/json")
		req.Header.Add("User-Agent", ua)
		req.Header.Add("Authorization", "Bearer "+r.token)

		res, err := r.client.Do(req)
		if err != nil {
			glog.Infof("Error sending HttpRequest %v", err)
			continue
		}
		defer res.Body.Close()
		data, _ := ioutil.ReadAll(res.Body)
		// unmarshal response data into the ResponseBody struct
		unmarshalError := json.Unmarshal(data, &responseBody)
		if unmarshalError != nil {
			glog.Error(unmarshalError)
			glog.Error(bytes.NewBuffer(reqBody))
		}

		// loop through the clusters in the response and create the PolicyReport node for each violation
		for cluster := range responseBody.Reports {
			// convert report data into []byte
			reportBytes, _ := json.Marshal(responseBody.Reports[cluster])
			// unmarshal response data into the Policy struct
			unmarshalReportError := json.Unmarshal(reportBytes, &policy)
			if err != nil {
				glog.Infof("Error %s", unmarshalReportError)
			}
			glog.V(2).Infof("Received PolicyInfo for cluster %s", cluster)
			policyInfo := types.PolicyInfo{Policy: policy, ClusterId: cluster}
			output <- policyInfo
		}
	}
}

// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package retriever

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	b64 "encoding/base64"
	"encoding/json"
	e "errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/open-cluster-management/insights-client/pkg/config"
	"github.com/open-cluster-management/insights-client/pkg/monitor"
	"github.com/open-cluster-management/insights-client/pkg/types"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Retriever struct
type Retriever struct {
	CCXUrl     string
	ContentURL string
	Client     *http.Client
	Token      string // token to connect to CRC
}

type serializedAuthMap struct {
	Auths map[string]serializedAuth `json:"auths"`
}
type serializedAuth struct {
	Auth string `json:"auth"`
}

// NewRetriever ...
func NewRetriever(ccxurl string, ContentURL string, client *http.Client,
	token string) *Retriever {
	if client == nil {
		if config.Cfg.CACert != "" {
			// If caCert is defiend in Insights-client deployment - we need to use it in http client
			// This will be used only for dev & testing purposes to use qaprodauth.cloud.redhat.com
			decodedCert, err := b64.URLEncoding.DecodeString(config.Cfg.CACert)
			if err != nil {
				// Exit because this is an unrecoverable configuration problem.
				glog.Fatal("Error decoding CA certificate. Certificate must be a base64 encoded CA certificate. Error: ", err)
			}
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(decodedCert)

			tlsCfg := &tls.Config{
				RootCAs: caCertPool,
			}

			tr := &http.Transport{
				TLSClientConfig: tlsCfg,
			}

			client = &http.Client{Transport: tr}
		} else {
			client = &http.Client{}
		}
	}
	r := &Retriever{
		Client:     client,
		CCXUrl:     ccxurl,
		ContentURL: ContentURL,
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
					r.Token = "Bearer " + token
				}
			}
		}
	}
	return nil
}

// RetrieveCCXReport ...
func (r *Retriever) RetrieveCCXReport(
	hubID string,
	input chan types.ManagedClusterInfo,
	output chan types.ProcessorData,
) {
	for {
		cluster := <-input
		// If the cluster id is empty do nothing
		if cluster.Namespace == "" || cluster.ClusterID == "" {
			return
		}

		glog.Infof("RetrieveCCXReport for cluster %s", cluster.Namespace)
		req, err := r.CreateInsightsRequest(context.TODO(), r.CCXUrl, cluster, hubID)
		if err != nil {
			glog.Warningf("Error creating HttpRequest for cluster %s (%s), %v", cluster.Namespace, cluster.ClusterID, err)
			continue
		}
		response, err := r.CallInsights(req, cluster)
		if err != nil {
			glog.Warningf("Error sending HttpRequest for cluster %s (%s), %v", cluster.Namespace, cluster.ClusterID, err)
			continue
		}

		policyReports, err := r.GetPolicyInfo(response, cluster)
		if err != nil {
			glog.Warningf("Error creating PolicyInfo for cluster %s (%s), %v", cluster.Namespace, cluster.ClusterID, err)
			continue
		}
		output <- policyReports
	}
}

// CreateInsightsRequest ...
func (r *Retriever) CreateInsightsRequest(
	ctx context.Context,
	endpoint string,
	cluster types.ManagedClusterInfo,
	hubID string,
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
	userAgent := "insights-operator/v1.0.0+b653953-b653953ed174001d5aca50b3515f1fa6f6b28728 cluster/" + hubID
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Authorization", r.Token)
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
) (types.ProcessorData, error) {
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
				return types.ProcessorData{}, unmarshalError
			}

			return types.ProcessorData{
				ClusterInfo: cluster,
				Reports:     reports,
			}, nil
		}
	}
	return types.ProcessorData{
		ClusterInfo: cluster,
		Reports:     types.Reports{},
	}, nil
}

// FetchClusters forwards the managed clusters to RetrieveCCXReports function
func (r *Retriever) FetchClusters(monitor *monitor.Monitor, input chan types.ManagedClusterInfo, refreshToken bool) {
	ticker := time.NewTicker(monitor.ClusterPollInterval)
	defer ticker.Stop()
	for ; true; <-ticker.C {
		if refreshToken {
			err := r.StartTokenRefresh()
			if err != nil {
				glog.Warningf("Unable to get CRC Token, Using previous Token: %v", err)
			}
		}
		if len(monitor.ManagedClusterInfo) > 0 {
			lock.RLock()
			for _, cluster := range monitor.ManagedClusterInfo {
				glog.Infof("Starting to get  cluster report for  %s", cluster)
				input <- cluster
				time.Sleep(time.Duration(config.Cfg.RequestInterval) * time.Second)
			}
			lock.RUnlock()
		}
	}
}

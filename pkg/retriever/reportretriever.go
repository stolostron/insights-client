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
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/stolostron/insights-client/pkg/config"
	"github.com/stolostron/insights-client/pkg/monitor"
	"github.com/stolostron/insights-client/pkg/types"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	knet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/client-go/dynamic"
)

// Retriever struct
type Retriever struct {
	CCXUrl          string
	ContentURL      string
	Client          *http.Client
	Token           string // token to connect to CRC
	DisconnectedEnv bool
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
		clientTransport := &http.Transport{
			Proxy: knet.NewProxierWithNoProxyCIDR(http.ProxyFromEnvironment),
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
			DisableKeepAlives:   true,
		}
		if config.Cfg.CACert != "" {
			// If caCert is defiend in Insights-client deployment - we need to use it in http client
			decodedCert, err := b64.URLEncoding.DecodeString(config.Cfg.CACert)
			if err != nil {
				// Exit because this is an unrecoverable configuration problem.
				glog.Fatal("Error decoding CA certificate. Certificate must be a base64 encoded CA certificate. Error: ", err)
			}
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(decodedCert)

			tlsCfg := &tls.Config{
				MinVersion: tls.VersionTLS12,
				RootCAs:    caCertPool,
			}
			clientTransport.TLSClientConfig = tlsCfg
		}
		client = &http.Client{Transport: clientTransport}
	}
	r := &Retriever{
		Client:     client,
		CCXUrl:     ccxurl,
		ContentURL: ContentURL,
	}
	if token == "" {
		r.DisconnectedEnv = r.setUpRetriever()
	} else {
		r.Token = token
		r.DisconnectedEnv = false
	}
	return r
}

// Get CRC token , wait until we can get token
func (r *Retriever) setUpRetriever() bool {
	err := r.StartTokenRefresh()
	refreshCounter := 0
	for err != nil && refreshCounter < 12 {
		glog.Warningf("Unable to get CRC Token: %v", err)
		time.Sleep(5 * time.Second)
		refreshCounter += 1
	}
	if refreshCounter == 12 {
		glog.Warning("Could not get token from CCX server after 1 minute, treating env as disconnected")
		return true
	}
	return false
}

// StartTokenRefresh sets the CRC token for use in Insights queries
func (r *Retriever) StartTokenRefresh() error {
	glog.Infof("Refreshing CRC credentials  ")
	secret, err := config.GetKubeClient().CoreV1().Secrets("openshift-config").
		Get(context.TODO(), "pull-secret", metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			glog.V(2).Infof("pull-secret does not exist")
			err = fmt.Errorf("pull-secret does not exist in openshift-config namespace: %v", err)
		} else if errors.IsForbidden(err) {
			glog.V(2).Infof("Operator does not have permission to check pull-secret: %v", err)
			err = fmt.Errorf("operator does not have permission to check pull-secret: %v", err)
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
				err = fmt.Errorf("unable to unmarshal cluster pull-secret: %v", err)
				return err
			}
			if auth, ok := pullSecret.Auths["cloud.openshift.com"]; ok {
				token := strings.TrimSpace(auth.Auth)
				if strings.Contains(token, "\n") || strings.Contains(token, "\r") {
					return fmt.Errorf("cluster authorization token is not valid: contains newlines")
				}
				if len(token) > 0 {
					glog.V(2).Info("Found cloud.openshift.com token ")
					r.Token = "Bearer " + token
					return nil
				}
			} else {
				return fmt.Errorf("cloud.openshift.com token is not found")
			}
		} else {
			return fmt.Errorf(".dockerconfigjson token is not found")
		}
	} else {
		return fmt.Errorf("could not get pull-secret")
	}
	return fmt.Errorf("unknown error during TokenRefresh")
}

func clusterNeedsCCX(cluster types.ManagedClusterInfo, clusterCCXMap map[string]bool) bool {
	lock.Lock()
	defer lock.Unlock()
	needsCCX := clusterCCXMap[cluster.ClusterID]
	return needsCCX
}

// RetrieveReport ...
func (r *Retriever) RetrieveReport(
	hubID string,
	input chan types.ManagedClusterInfo,
	output chan types.ProcessorData,
	clusterCCXMap map[string]bool,
	isDisconnected bool,
) {
	for {
		cluster := <-input
		// If the cluster id is empty do nothing
		if cluster.Namespace == "" || cluster.ClusterID == "" {
			continue
		}

		if !clusterNeedsCCX(cluster, clusterCCXMap) || isDisconnected {
			glog.Infof("Retrieve Report for cluster %s", cluster.Namespace)
			output <- types.ProcessorData{
				ClusterInfo: cluster,
				Reports:     types.Reports{},
			}
			continue
		}

		glog.Infof("Retrieve CCX Report for cluster %s", cluster.Namespace)
		req, err := r.CreateInsightsRequest(context.TODO(), r.CCXUrl, cluster, hubID)
		if err != nil {
			handleCCXRequestErr(err, "Error creating HttpRequest for cluster %s (%s), %v", output, cluster)
			continue
		}
		response, err := r.CallInsights(req, cluster)
		if err != nil {
			handleCCXRequestErr(err, "Error getting good Response for cluster %s (%s), %v", output, cluster)
			continue
		}

		policyReports, err := r.GetPolicyInfo(response, cluster)
		if err != nil {
			handleCCXRequestErr(err, "Error creating PolicyInfo for cluster %s (%s), %v", output, cluster)
			continue
		}
		output <- policyReports
	}
}

func handleCCXRequestErr(
	err error,
	message string,
	output chan types.ProcessorData,
	cluster types.ManagedClusterInfo,
) {
	glog.Warningf(message, cluster.Namespace, cluster.ClusterID, err)
	output <- types.ProcessorData{
		ClusterInfo: cluster,
		Reports:     types.Reports{},
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
	userAgent := "acm-operator/v2.3.0 cluster/" + hubID
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Authorization", r.Token)
	return req, nil
}

// CallInsights ...
func (r *Retriever) CallInsights(req *http.Request, cluster types.ManagedClusterInfo) (types.ResponseBody, error) {
	glog.V(2).Infof("Starting CallInsights for cluster %s (%s)", cluster.Namespace, cluster.ClusterID)
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
		if res.StatusCode == 400 {
			glog.Infof("Check OCM Console - cluster should be registered in CCX server %v", cluster.ClusterID)
		}
		if res.StatusCode == 401 {
			glog.Infof("Check OCM Console - Hub cluster and managed cluster should be reqistered with IDs from Same Org %v", cluster.ClusterID)
		}
		glog.V(2).Infof("Response status for report %v", res.Status)
		glog.V(3).Infof("Response body for report  %v", req.Body)
		glog.V(3).Infof("Response header for report %v", req.Header)
		return types.ResponseBody{}, e.New("no Success HTTP Response code ")
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(res.Body)
	data, _ := io.ReadAll(res.Body)
	// unmarshal response data into the ResponseBody struct
	unmarshalError := json.Unmarshal(data, &responseBody)
	if unmarshalError != nil {
		glog.Errorf("Error unmarshalling ResponseBody %v", unmarshalError)
		return types.ResponseBody{}, unmarshalError
	}
	glog.V(2).Info("Successfully called insights. Returning the response body.")
	return responseBody, err
}

// GetPolicyInfo ...
func (r *Retriever) GetPolicyInfo(
	responseBody types.ResponseBody,
	cluster types.ManagedClusterInfo,
) (types.ProcessorData, error) {
	glog.V(2).Infof("Starting GetPolicyInfo for cluster %s (%s)", cluster.Namespace, cluster.ClusterID)
	reports := types.Reports{}
	for _, clusterErrored := range responseBody.Errors {
		glog.Warningf("No Reports returned from CCX Insights for cluster: %s", clusterErrored)
		glog.V(2).Infof("Errors returned from CCX Insights for cluster : %v", responseBody)
	}

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

			glog.V(2).Infof(
				"Successfully requested report for cluster %s (%s). Proceeding to processor.",
				cluster.Namespace,
				cluster.ClusterID,
			)
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
func (r *Retriever) FetchClusters(
	monitor *monitor.Monitor,
	input chan types.ManagedClusterInfo,
	refreshToken bool,
	hubID string,
	dynamicClient dynamic.Interface,
) {
	ticker := time.NewTicker(monitor.ClusterPollInterval)
	defer ticker.Stop()
	for ; true; <-ticker.C {
		if refreshToken {
			err := r.StartTokenRefresh()
			if err != nil {
				glog.Warningf("Unable to get CRC Token, Using previous Token: %v", err)
			}
			if len(ContentsMap) < 1 {
				r.InitializeContents(hubID, dynamicClient)
			} else if len(ContentsMap) > 0 && r.GetContentConfigMap(dynamicClient) == nil {
				r.CreateInsightContentConfigmap(dynamicClient)
			}
		}
		if len(monitor.GetManagedClusterInfo()) > 0 {
			for _, cluster := range monitor.GetManagedClusterInfo() {
				glog.Infof("Starting to get  cluster report for  %s", cluster)
				input <- cluster
				time.Sleep(time.Duration(config.Cfg.RequestInterval) * time.Second)
			}
		}
	}
}

// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package retriever

import (
	"context"
	"encoding/json"
	e "errors"
	"io"
	"net/http"
	"sync"

	"github.com/golang/glog"
	"github.com/stolostron/insights-client/pkg/config"
	"github.com/stolostron/insights-client/pkg/types"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// ContentsMap contains all policy data
var ContentsMap map[string]map[string]interface{}
var lock = sync.RWMutex{}
var podNS = config.Cfg.PodNamespace
var configmapGvr = schema.GroupVersionResource{
	Version:  "v1",
	Resource: "configmaps",
}

// InitializeContents ...
func (r *Retriever) InitializeContents(hubID string, dynamicClient dynamic.Interface) int {
	contentLength := r.retrieveCCXContent(hubID)
	// create a configmap containing insight content data
	if contentLength > 0 {
		r.CreateInsightContentConfigmap(dynamicClient)
	}
	return contentLength
}

// Function to make a GET HTTP call to get all the contents for reports
func (r *Retriever) retrieveCCXContent(hubID string) int {
	req, err := r.GetContentRequest(context.TODO(), hubID)
	if err != nil {
		glog.Warningf("Error creating HttpRequest with endpoint %s, %v", r.ContentURL, err)
		return -1
	}
	contents, err := r.CallContents(req)
	if err != nil {
		glog.Warningf("Error calling for contents %s, %v", hubID, err)
		return -1
	}
	r.CreateContents(contents)
	return len(ContentsMap)
}

// GetContentRequest - Creates GET request for contents
func (r *Retriever) GetContentRequest(ctx context.Context, hubID string) (*http.Request, error) {
	glog.Infof("Creating Content Request for cluster %s using  URL %s :", hubID, r.ContentURL)
	req, err := http.NewRequest("GET", r.ContentURL, nil)
	if err != nil {
		glog.Warningf("Error creating HttpRequest with endpoint %s, %v", r.ContentURL, err)
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

// CallContents - Makes HTTP call to get contents
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
		glog.V(2).Infof("Contents statuscode %v", res.Status)
		glog.V(3).Infof("Contents responseBody %v", res.Body)
		glog.V(3).Infof("Contents request %v", res.Request)
		return types.ContentsResponse{}, e.New("no Success HTTP Response code ")
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(res.Body)
	data, _ := io.ReadAll(res.Body)
	// unmarshal response data into the ResponseBody struct
	unmarshalError := json.Unmarshal(data, &responseBody)
	if unmarshalError != nil {
		glog.Errorf("Error unmarshalling ResponseBody %v", unmarshalError)
		return types.ContentsResponse{}, unmarshalError
	}
	return responseBody, err
}

// CreateContents - Populate json response from /contents call onto a Map to quick lookup
func (r *Retriever) CreateContents(responseBody types.ContentsResponse) {
	glog.Infof("Creating Contents from json ")
	ContentsMap = make(map[string]map[string]interface{})

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
			ContentsMap[errorName] = errorMap
			lock.Unlock()
		}
	}
}

// GetContentConfigMap ...
func (r *Retriever) GetContentConfigMap(dynamicClient dynamic.Interface) *unstructured.Unstructured {
	configmapRes, err := dynamicClient.Resource(configmapGvr).Namespace(podNS).Get(
		context.TODO(),
		"insight-content-data",
		metav1.GetOptions{},
	)

	if err != nil {
		glog.V(2).Infof(
			"Error getting ConfigMap: insight-content-data. Error: %v",
			err,
		)
	}

	return configmapRes
}

// CreateInsightContentConfigmap Creates a configmap to store content data in open-cluster-management namespace
func (r *Retriever) CreateInsightContentConfigmap(dynamicClient dynamic.Interface) {
	var configMapData = make(map[string]string)
	for policy := range ContentsMap {
		jsonStr, _ := json.Marshal(ContentsMap[policy])
		configMapData[policy] = string(jsonStr)
	}
	configmap := &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "insight-content-data",
			Namespace: podNS,
		},
		Data: configMapData,
	}
	cmUnstructured, unstructuredErr := runtime.DefaultUnstructuredConverter.ToUnstructured(configmap)
	if unstructuredErr != nil {
		glog.Warningf("Error converting to unstructured.Unstructured: %s", unstructuredErr)
	}
	obj := &unstructured.Unstructured{Object: cmUnstructured}
	configmapRes := r.GetContentConfigMap(dynamicClient)

	var err error
	if configmapRes != nil {
		_, err = dynamicClient.Resource(configmapGvr).Namespace(podNS).Update(
			context.TODO(),
			obj,
			metav1.UpdateOptions{},
		)
	} else {
		_, err = dynamicClient.Resource(configmapGvr).Namespace(podNS).Create(
			context.TODO(),
			obj,
			metav1.CreateOptions{},
		)
	}

	if err != nil {
		glog.Infof(
			"Error while creating or updating ConfigMap: insight-content-data. Error: %v",
			err,
		)
	} else {
		glog.Infof(
			"Successfully stored Insight content data in ConfigMap: insight-content-data, on namespace: %s",
			podNS,
		)
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

// GetContents - Given Error_Key and field name returns value
func (r *Retriever) GetContents(errorKey string, key string) interface{} {
	lock.RLock()
	defer lock.RUnlock()
	return ContentsMap[errorKey][key]
}

// GetFields - Given Error_Key gives the fields is has
func (r *Retriever) GetFields(errorKey string) []string {
	lock.RLock()
	defer lock.RUnlock()
	var fields []string
	for mkey := range ContentsMap[errorKey] {
		fields = append(fields, mkey)
	}
	return fields
}

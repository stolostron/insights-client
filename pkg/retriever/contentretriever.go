// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package retriever

import (
	"context"
	"encoding/json"
	e "errors"
	"io/ioutil"
	"net/http"
	"sync"

	"github.com/golang/glog"
	"github.com/open-cluster-management/insights-client/pkg/types"
)

var ContentsMap map[string]map[string]interface{}
var lock = sync.RWMutex{}

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
	userAgent := "insights-operator/v1.0.0+b653953-b653953ed174001d5aca50b3515f1fa6f6b28728 cluster/" + hubID
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
	r.createContents(contents)
	return len(ContentsMap)
}

// InitializeContents ...
func (r *Retriever) InitializeContents(hubID string) int {
	return r.retrieveCCXContent(hubID)
}

// Populate json response from /contents call onto a Map to quick lookup
func (r *Retriever) createContents(responseBody types.ContentsResponse) {
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

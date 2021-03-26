package retriever

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mocks "github.com/open-cluster-management/insights-client/pkg/utils"

	"github.com/open-cluster-management/insights-client/pkg/types"
)

func TestCallInsights(t *testing.T) {
	var postBody types.PostBody

	postFunc := func(w http.ResponseWriter, r *http.Request) {
		body, _ := ioutil.ReadAll(r.Body)
		err := json.Unmarshal(body, &postBody)
		if err == nil {
			w.Header().Set("Content-Type", "application/json")

			response := mocks.GetMockData(string(postBody.Clusters[0]))
			fmt.Fprintln(w, string(response))

		}

	}
	ts := httptest.NewServer(http.HandlerFunc(postFunc))
	ts.EnableHTTP2 = true
	defer ts.Close()

	ret := NewRetriever(ts.URL, "testContentUrl", nil, 2*time.Second, "testToken")
	req, _ := ret.GetInsightsRequest(context.TODO(), ts.URL, "34c3ecc5-624a-49a5-bab8-4fdc5e51a266")
	if req.Header.Get("User-Agent") != "insights-operator/v1.0.0+b653953-b653953ed174001d5aca50b3515f1fa6f6b28728 cluster/34c3ecc5-624a-49a5-bab8-4fdc5e51a266" {
		t.Errorf("Header User-Agent not formed correct    : %s", req.Header.Get("User-Agent"))
	}
	if !strings.HasPrefix(req.Header.Get("Authorization"), "Bearer testToken") {
		t.Errorf("Header Authorization not formed correct    : %s", req.Header.Get("Authorization"))
	}

	response, _ := ret.CallInsights(req, "34c3ecc5-624a-49a5-bab8-4fdc5e51a266")
	if len(response.Reports) != 1 {
		t.Errorf("Unexpected Report length %d", len(response.Reports))
	}

}

func TestCallContents(t *testing.T) {
	postFunc := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := mocks.GetMockContent()
		fmt.Fprintln(w, string(response))
	}
	ts := httptest.NewServer(http.HandlerFunc(postFunc))
	ts.EnableHTTP2 = true
	defer ts.Close()

	ret := NewRetriever("testCCXUrl", ts.URL, nil, 2*time.Second, "testToken")
	req, _ := ret.GetContentRequest(context.TODO(), "34c3ecc5-624a-49a5-bab8-4fdc5e51a266")
	if req.Header.Get("User-Agent") != "insights-operator/v1.0.0+b653953-b653953ed174001d5aca50b3515f1fa6f6b28728 cluster/34c3ecc5-624a-49a5-bab8-4fdc5e51a266" {
		t.Errorf("Header User-Agent not formed correct    : %s", req.Header.Get("User-Agent"))
	}
	if !strings.HasPrefix(req.Header.Get("Authorization"), "Bearer testToken") {
		t.Errorf("Header Authorization not formed correct    : %s", req.Header.Get("Authorization"))
	}

	response, _ := ret.CallContents(req)
	if len(response.Content) != 42 {
		t.Errorf("Unexpected Report length %d", len(response.Content))
	}
	ret.InitializeContents("34c3ecc5-624a-49a5-bab8-4fdc5e51a266")

	if len(ret.GetFields("TUTORIAL_ERROR")) != 12 {
		t.Error("RetrieveCCXContent  did not create all fields, expected 11 actual  ", len(ret.GetFields("TUTORIAL_ERROR")))
	}

	summary := ret.GetContents("TUTORIAL_ERROR", "summary")
	summary_str := summary.(string)
	summary_str = strings.ReplaceAll(summary_str, "\n", "")
	summary_str = strings.ReplaceAll(summary_str, "\t", "")
	var expected = "Red Hat Insights for OpenShift is a proactive management solution. It provides ongoing infrastructure" +
		" analyses of your Red Hat OpenShift Container Platform 4.2 and later installations. Red Hat Insights helps you identify," +
		" prioritize, and resolve risks to security, performance, availability, and stability before they become urgent issues." +
		"Red Hat Insights for OpenShift uses the Remote Health Monitoring feature of OpenShift 4. The health checks are created " +
		"by Red Hat subject matter experts and assessed according to severity and impact.This is an example  recommendation that" +
		" you can safely ignore. To disable it, click  the triple-dot menu button next to the header, and select Disable."
	if summary_str != expected {
		t.Error("Content summary did not match - actual   summary: ", summary_str)
		t.Error("Content summary did not match - expected summary: ", expected)
	}

}

package retriever

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mocks "github.com/stolostron/insights-client/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfakeclient "k8s.io/client-go/dynamic/fake"
)

var fakeDynamicClient *dynamicfakeclient.FakeDynamicClient
var namespace *corev1.Namespace

func TestCallContents(t *testing.T) {
	namespace = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "open-cluster-management",
		},
	}
	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Namespace{})
	scheme.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.ConfigMap{})
	fakeDynamicClient = dynamicfakeclient.NewSimpleDynamicClient(scheme, namespace)

	postFunc := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := mocks.GetMockContent()
		_, _ = fmt.Fprintln(w, string(response))
	}
	ts := httptest.NewServer(http.HandlerFunc(postFunc))
	ts.EnableHTTP2 = true
	defer ts.Close()

	ret := NewRetriever("testCCXUrl", ts.URL, nil, "testToken")
	req, _ := ret.GetContentRequest(context.TODO(), "34c3ecc5-624a-49a5-bab8-4fdc5e51a266")
	if req.Header.Get("User-Agent") != "acm-operator/v2.3.0 cluster/34c3ecc5-624a-49a5-bab8-4fdc5e51a266" {
		t.Errorf("Header User-Agent not formed correct    : %s", req.Header.Get("User-Agent"))
	}
	if !strings.HasPrefix(req.Header.Get("Authorization"), "testToken") {
		t.Errorf("Header Authorization not formed correct    : %s", req.Header.Get("Authorization"))
	}

	response, _ := ret.CallContents(req)
	if len(response.Content) != 42 {
		t.Errorf("Unexpected Report length %d", len(response.Content))
	}
	ret.InitializeContents("34c3ecc5-624a-49a5-bab8-4fdc5e51a266", fakeDynamicClient)

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

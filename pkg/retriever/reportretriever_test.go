package retriever

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stolostron/insights-client/pkg/config"
	"github.com/stolostron/insights-client/pkg/monitor"
	"github.com/stolostron/insights-client/pkg/types"
	mocks "github.com/stolostron/insights-client/pkg/utils"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfakeclient "k8s.io/client-go/dynamic/fake"
	"sigs.k8s.io/wg-policy-prototypes/policy-report/pkg/api/wgpolicyk8s.io/v1beta1"
)

func TestCallInsights(t *testing.T) {
	var postBody types.PostBody

	postFunc := func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		err := json.Unmarshal(body, &postBody)
		if err == nil {
			w.Header().Set("Content-Type", "application/json")

			response := mocks.GetMockData(string(postBody.Clusters[0]))
			_, _ = fmt.Fprintln(w, string(response))

		}

	}
	ts := httptest.NewServer(http.HandlerFunc(postFunc))
	ts.EnableHTTP2 = true
	defer ts.Close()

	ret := NewRetriever(ts.URL, "testContentUrl", nil, "testToken")
	req, _ := ret.CreateInsightsRequest(
		context.TODO(),
		ts.URL,
		types.ManagedClusterInfo{Namespace: "testCluster", ClusterID: "34c3ecc5-624a-49a5-bab8-4fdc5e51a266"},
		"34c3ecc5-624a-49a5-bab8-4fdc5e51a266",
	)
	if req.Header.Get("User-Agent") != "acm-operator/v2.3.0 cluster/34c3ecc5-624a-49a5-bab8-4fdc5e51a266" {
		t.Errorf("Header User-Agent not formed correct    : %s", req.Header.Get("User-Agent"))
	}
	if !strings.HasPrefix(req.Header.Get("Authorization"), "testToken") {
		t.Errorf("Header Authorization not formed correct    : %s", req.Header.Get("Authorization"))
	}

	response, _ := ret.CallInsights(req, types.ManagedClusterInfo{Namespace: "testCluster", ClusterID: "34c3ecc5-624a-49a5-bab8-4fdc5e51a266"})
	if len(response.Reports) != 1 {
		t.Errorf("Unexpected Report length %d", len(response.Reports))
	}

}

func Test_FetchClusters(t *testing.T) {
	// Establish the config
	config.SetupConfig()

	namespace = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "local-cluster",
		},
	}
	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Namespace{})
	scheme.AddKnownTypes(v1beta1.SchemeGroupVersion, &v1beta1.PolicyReport{})
	fakeDynamicClient = dynamicfakeclient.NewSimpleDynamicClient(scheme, namespace)

	monitor := monitor.NewClusterMonitor()
	monitor.ManagedClusterInfo = []types.ManagedClusterInfo{{Namespace: "local-cluster", ClusterID: "323a00cd-428a-49fb-80ab-201d2a5d3050"}}

	fetchClusterIDs := make(chan types.ManagedClusterInfo)

	ret := NewRetriever("testServer", "testContentUrl", nil, "testToken")

	go ret.FetchClusters(monitor, fetchClusterIDs, false, "323a00cd-428a-49fb-80ab-201d2a5d3050", fakeDynamicClient)
	testData := <-fetchClusterIDs

	assert.Equal(
		t,
		types.ManagedClusterInfo{Namespace: "local-cluster", ClusterID: "323a00cd-428a-49fb-80ab-201d2a5d3050"},
		testData,
		"Test Fetch ManagedCluster list",
	)
}

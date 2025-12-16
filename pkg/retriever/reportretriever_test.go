package retriever

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stolostron/insights-client/pkg/config"
	"github.com/stolostron/insights-client/pkg/monitor"
	"github.com/stolostron/insights-client/pkg/types"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfakeclient "k8s.io/client-go/dynamic/fake"
	"sigs.k8s.io/wg-policy-prototypes/policy-report/pkg/api/wgpolicyk8s.io/v1beta1"
)

func TestCallInsights(t *testing.T) {
	getFunc := func(w http.ResponseWriter, r *http.Request) {
		// Verify the request method is GET
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}

		// Verify the URL path contains the cluster ID
		expectedPath := "/cluster/34c3ecc5-624a-49a5-bab8-4fdc5e51a266/reports"
		if r.URL.Path != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		// Create a mock response that matches the expected structure
		mockResponse := `{
			"status": "ok",
			"report": {
				"data": [
					{
						"rule_id": "test_rule_1",
						"key": "test_key_1",
						"component": "test_component_1",
						"details": "test details 1"
					}
				],
				"meta": {
					"cluster_name": "test-cluster",
					"count": 1,
					"gathered_at": "2021-10-20T15:00:00Z",
					"last_checked_at": "2021-10-20T15:00:00Z",
					"managed": false
				}
			}
		}`
		_, _ = fmt.Fprintln(w, mockResponse)
	}
	ts := httptest.NewServer(http.HandlerFunc(getFunc))
	ts.EnableHTTP2 = true
	defer ts.Close()

	ret := NewRetriever(ts.URL, nil, "testToken")
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
	if len(response.Report.Data) != 1 {
		t.Errorf("Unexpected Report length %d", len(response.Report.Data))
	}

}

func Test_FetchClusters(t *testing.T) {
	// Establish the config
	config.SetupConfig()

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "local-cluster",
		},
	}
	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Namespace{})
	scheme.AddKnownTypes(v1beta1.SchemeGroupVersion, &v1beta1.PolicyReport{})
	fakeDynamicClient := dynamicfakeclient.NewSimpleDynamicClient(scheme, namespace)

	monitor := monitor.NewClusterMonitor()
	monitor.ManagedClusterInfo = []types.ManagedClusterInfo{{Namespace: "local-cluster", ClusterID: "323a00cd-428a-49fb-80ab-201d2a5d3050"}}

	fetchClusterIDs := make(chan types.ManagedClusterInfo)

	ret := NewRetriever("testServer", nil, "testToken")

	go ret.FetchClusters(monitor, fetchClusterIDs, false, "323a00cd-428a-49fb-80ab-201d2a5d3050", fakeDynamicClient)
	testData := <-fetchClusterIDs

	assert.Equal(
		t,
		types.ManagedClusterInfo{Namespace: "local-cluster", ClusterID: "323a00cd-428a-49fb-80ab-201d2a5d3050"},
		testData,
		"Test Fetch ManagedCluster list",
	)
}

func TestRetrieveReport(t *testing.T) {
	t.Run("Successful report retrieval", func(t *testing.T) {
		// Create a mock server
		getFunc := func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				t.Errorf("Expected GET request, got %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			// Create a properly formatted response that matches the expected structure
			mockResponse := `{
				"status": "ok",
				"report": {
					"data": [
						{
							"rule_id": "test_rule_1",
							"key": "test_key_1",
							"component": "test_component_1",
							"details": "test details 1"
						},
						{
							"rule_id": "test_rule_2", 
							"key": "test_key_2",
							"component": "test_component_2",
							"details": "test details 2"
						}
					],
					"meta": {
						"cluster_name": "test-cluster",
						"count": 2,
						"gathered_at": "2021-10-20T15:00:00Z",
						"last_checked_at": "2021-10-20T15:00:00Z",
						"managed": false
					}
				}
			}`
			_, _ = fmt.Fprintln(w, mockResponse)
		}
		ts := httptest.NewServer(http.HandlerFunc(getFunc))
		defer ts.Close()

		input := make(chan types.ManagedClusterInfo, 1)
		output := make(chan types.ProcessorData, 1)

		cluster := types.ManagedClusterInfo{
			Namespace: "test-cluster",
			ClusterID: "34c3ecc5-624a-49a5-bab8-4fdc5e51a266",
		}
		input <- cluster
		close(input)

		// Cluster is in CCX map
		clusterCCXMap := map[string]bool{
			"34c3ecc5-624a-49a5-bab8-4fdc5e51a266": true,
		}

		ret := NewRetriever(ts.URL, nil, "testToken")
		go ret.RetrieveReport("testHubID", input, output, clusterCCXMap, false)

		result := <-output
		if result.ClusterInfo.Namespace != cluster.Namespace {
			t.Errorf("Expected cluster namespace %s, got %s", cluster.Namespace, result.ClusterInfo.Namespace)
		}
		if result.ClusterInfo.ClusterID != cluster.ClusterID {
			t.Errorf("Expected cluster ID %s, got %s", cluster.ClusterID, result.ClusterInfo.ClusterID)
		}
		// Should have reports from the mock data
		if len(result.Report.Data) != 2 {
			t.Errorf("Expected 2 reports to be retrieved, but got %d reports", len(result.Report.Data))
		}
	})
}

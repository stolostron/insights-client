// Copyright Contributors to the Open Cluster Management project

package processor

import (
	"context"
	json "encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/kennygrant/sanitize"
	"github.com/stolostron/insights-client/pkg/retriever"
	"github.com/stolostron/insights-client/pkg/types"
	mocks "github.com/stolostron/insights-client/pkg/utils"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfakeclient "k8s.io/client-go/dynamic/fake"
	"sigs.k8s.io/wg-policy-prototypes/policy-report/pkg/api/wgpolicyk8s.io/v1beta1"
)

func UnmarshalFile(filepath string, resourceType interface{}, t *testing.T) {
	// open given filepath string
	rawBytes, err := os.ReadFile("../../test-data/" + sanitize.Name(filepath))
	if err != nil {
		t.Fatal("Unable to read test data", err)
	}

	// unmarshal file into given resource type
	err = json.Unmarshal(rawBytes, resourceType)
	if err != nil {
		t.Fatalf("Unable to unmarshal json to type %T %s", resourceType, err)
	}
}

var (
	fetchPolicyReports chan types.ProcessorData
	mngd               types.ManagedClusterInfo
	fakeDynamicClient  *dynamicfakeclient.FakeDynamicClient
	ret                *retriever.Retriever
	respBody           types.ResponseBody
	processor          *Processor
)

func setUp(t *testing.T) {
	fetchPolicyReports = make(chan types.ProcessorData, 1)

	var postBody types.PostBody
	postFunc := func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		err := json.Unmarshal(body, &postBody)
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			_ = mocks.GetMockData(string(postBody.Clusters[0]))
		}
	}
	ts := httptest.NewServer(http.HandlerFunc(postFunc))

	ret = retriever.NewRetriever("testReportUrl", ts.URL, nil, "testToken")

	mngd = types.ManagedClusterInfo{Namespace: "testCluster", ClusterID: "972ea7cf-7428-438f-ade8-12ac4794ede0"}

	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Namespace{})
	scheme.AddKnownTypes(v1beta1.SchemeGroupVersion, &v1beta1.PolicyReport{})

	gvrToListKind := map[schema.GroupVersionResource]string{policyGvr: "PolicyList"}

	objects := []runtime.Object{
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: mngd.Namespace,
			},
		},
		// Noncompliant policy that will be in the policy report.
		&unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": policyGvr.GroupVersion().String(),
				"kind":       "Policy",
				"metadata": map[string]interface{}{
					"name":      "default.policy1",
					"namespace": mngd.Namespace,
					"annotations": map[string]interface{}{
						"policy.open-cluster-management.io/categories": "CM Configuration Management",
					},
					"creationTimestamp": "2023-12-04T18:47:37Z",
				},
				"spec": map[string]interface{}{
					"policy-templates": []interface{}{
						map[string]interface{}{
							"objectDefinition": map[string]interface{}{
								"apiVersion": "policy.open-cluster-management.io/v1",
								"kind":       "ConfigurationPolicy",
								"metadata": map[string]interface{}{
									"name": "policy-namespace-default",
								},
								"spec": map[string]interface{}{
									"object-templates": []interface{}{
										map[string]interface{}{
											"complianceType": "musthave",
											"objectDefinition": map[string]interface{}{
												"apiVersion": "v1",
												"kind":       "Namespace",
												"metadata": map[string]interface{}{
													"name": "default",
												},
											},
										},
									},
									"remediationAction": "inform",
									"severity":          "low",
								},
							},
						},
						map[string]interface{}{
							"objectDefinition": map[string]interface{}{
								"apiVersion": "policy.open-cluster-management.io/v1",
								"kind":       "ConfigurationPolicy",
								"metadata": map[string]interface{}{
									"name": "policy-namespace",
								},
								"spec": map[string]interface{}{
									"object-templates": []interface{}{
										map[string]interface{}{
											"complianceType": "musthave",
											"objectDefinition": map[string]interface{}{
												"apiVersion": "v1",
												"kind":       "Namespace",
												"metadata": map[string]interface{}{
													"name": "test-ns",
												},
											},
										},
									},
									"remediationAction": "inform",
									"severity":          "critical",
								},
							},
						},
						map[string]interface{}{
							"objectDefinition": map[string]interface{}{
								"apiVersion": "constraints.gatekeeper.sh/v1beta1",
								"kind":       "K8sRequiredLabels",
								"metadata": map[string]interface{}{
									"name": "ns-must-have-labels",
									"annotations": map[string]interface{}{
										"policy.open-cluster-management.io/severity": "high",
									},
								},
								"spec": map[string]interface{}{
									"match": map[string]interface{}{
										"kinds": []interface{}{
											map[string]interface{}{
												"apiGroups": []interface{}{""},
												"kinds":     []interface{}{"Namespace"},
											},
										},
									},
								},
							},
						},
					},
				},
				"status": map[string]interface{}{
					"compliant": "NonCompliant",
					"details": []interface{}{
						// A compliant policy template that should be skipped in the policy report.
						map[string]interface{}{
							"compliant": "Compliant",
							"history": []interface{}{
								map[string]interface{}{
									"eventName":     "default.policy1.179db5612e003d42",
									"lastTimestamp": "2023-12-04T18:47:41Z",
									"message":       "Compliant; notification - namespaces [default] found as specified",
								},
							},
							"templateMeta": map[string]interface{}{
								"name": "policy-namespace-default",
							},
						},
						map[string]interface{}{
							"compliant": "NonCompliant",
							"history": []interface{}{
								map[string]interface{}{
									"eventName":     "default.policy1.179db5612e003d61",
									"lastTimestamp": "2023-12-04T18:47:43Z",
									"message":       "NonCompliant; violation - namespaces [test-ns] not found",
								},
							},
							"templateMeta": map[string]interface{}{
								"name": "policy-namespace",
							},
						},
						map[string]interface{}{
							"compliant": "NonCompliant",
							"history": []interface{}{
								map[string]interface{}{
									"eventName":     "default.policy1.179db5612e003d63",
									"lastTimestamp": "2023-12-04T18:47:47Z",
									"message":       "NonCompliant; violation - some Gatekeeper audit failure message",
								},
							},
							"templateMeta": map[string]interface{}{
								"name": "ns-must-have-labels",
							},
						},
					},
				},
			},
		},
		// Compliant policy that should be ignored
		&unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": policyGvr.GroupVersion().String(),
				"kind":       "Policy",
				"metadata": map[string]interface{}{
					"name":      "default.policy2",
					"namespace": mngd.Namespace,
					"annotations": map[string]interface{}{
						"policy.open-cluster-management.io/categories": "CM Configuration Management",
					},
					"creationTimestamp": "2023-12-04T18:43:31Z",
				},
				"status": map[string]interface{}{
					"compliant": "Compliant",
					"details": []interface{}{
						map[string]interface{}{
							"compliant": "Compliant",
							"history": []interface{}{
								map[string]interface{}{
									"eventName":     "default.policy1.179db5612e003d21",
									"lastTimestamp": "2023-12-04T18:43:41Z",
									"message":       "Compliant; notification - namespaces [default] found as specified",
								},
							},
							"templateMeta": map[string]interface{}{
								"name": "policy-namespace2",
							},
						},
					},
				},
			},
		},
	}

	fakeDynamicClient = dynamicfakeclient.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, objects...)

	processor = NewProcessor()
	fmt.Println("Setup complete")
}

func addReportToChannel(t *testing.T, filename string) {
	UnmarshalFile(filename, &respBody, t)
	policyReports, err := ret.GetPolicyInfo(respBody, mngd)
	if err != nil {
		t.Log("Error getting policyInfo: ", err)
	}
	fetchPolicyReports <- policyReports
}

func Test_createPolicyReport(t *testing.T) {
	setUp(t)
	addReportToChannel(t, "createreporttest.json")

	processor.createUpdatePolicyReports(fetchPolicyReports, fakeDynamicClient)
	createdPolicyReport := &v1beta1.PolicyReport{}

	// Check if the policyReport is created
	unstructuredPolR, err := fakeDynamicClient.Resource(policyReportGvr).Namespace(mngd.Namespace).Get(context.TODO(), mngd.Namespace+"-policyreport", metav1.GetOptions{})
	if err != nil {
		t.Log(err)
	}
	assert.Nil(t, err, "Expected policy report to be created. Got error: %v", err)

	unstructConvErr := runtime.DefaultUnstructuredConverter.FromUnstructured(
		unstructuredPolR.UnstructuredContent(),
		&createdPolicyReport,
	)
	if unstructConvErr != nil {
		t.Log(unstructConvErr)
	}

	assert.Nil(t, unstructConvErr, "Expected policy report to be properly formatted. Got error: %v", unstructConvErr)
	assert.Equal(t, 4, len(createdPolicyReport.Results), "Expected 3 issues to be found. Got %v", len(createdPolicyReport.Results))

	policyResult1 := createdPolicyReport.Results[2]
	expectedPolicyResult1 := v1beta1.PolicyReportResult{
		Source:      "grc",
		Policy:      "default.policy1",
		Category:    "CM Configuration Management",
		Timestamp:   metav1.Timestamp{Seconds: 1701715663},
		Result:      "fail",
		Description: "NonCompliant; violation - namespaces [test-ns] not found",
		Properties: map[string]string{
			"created_at": "2023-12-04T18:47:37Z",
			"total_risk": "4",
		},
	}

	assert.Equal(t, expectedPolicyResult1, policyResult1)

	policyResult2 := createdPolicyReport.Results[3]
	expectedPolicyResult2 := v1beta1.PolicyReportResult{
		Source:      "grc",
		Policy:      "default.policy1",
		Category:    "CM Configuration Management",
		Timestamp:   metav1.Timestamp{Seconds: 1701715667},
		Result:      "fail",
		Description: "NonCompliant; violation - some Gatekeeper audit failure message",
		Properties: map[string]string{
			"created_at": "2023-12-04T18:47:37Z",
			"total_risk": "3",
		},
	}

	assert.Equal(t, expectedPolicyResult2, policyResult2)
}

func Test_filterOpenshiftCategory(t *testing.T) {
	categories := []string{"test1", "openshift", "test2"}
	filtered := FilterOpenshiftCategory(categories)

	assert.Equal(t, "test1,test2", filtered, "Expected category list to exclude openshift")
}

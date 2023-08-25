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
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfakeclient "k8s.io/client-go/dynamic/fake"
	"sigs.k8s.io/wg-policy-prototypes/policy-report/pkg/api/wgpolicyk8s.io/v1alpha2"
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

var fetchPolicyReports chan types.ProcessorData
var mngd types.ManagedClusterInfo
var fakeDynamicClient *dynamicfakeclient.FakeDynamicClient
var namespace *corev1.Namespace
var ret *retriever.Retriever
var respBody types.ResponseBody
var processor *Processor

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

	ret = retriever.NewRetriever("testCCXUrl", ts.URL, nil, "testToken")

	mngd = types.ManagedClusterInfo{Namespace: "testCluster", ClusterID: "972ea7cf-7428-438f-ade8-12ac4794ede0"}

	fmt.Println("Load contentsMap")
	var content types.ContentsResponse
	UnmarshalFile("content.json", &content, t)
	ret.CreateContents(content)

	namespace = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: mngd.Namespace,
		},
	}

	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Namespace{})
	scheme.AddKnownTypes(v1alpha2.SchemeGroupVersion, &v1alpha2.PolicyReport{})

	gvrToListKind := map[schema.GroupVersionResource]string{
		{Group: "policy.open-cluster-management.io", Version: "v1", Resource: "policies"}: "PolicyList",
	}
	fakeDynamicClient = dynamicfakeclient.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, namespace)

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
	createdPolicyReport := &v1alpha2.PolicyReport{}

	//Check if the policyReport is created
	unstructuredPolR, err := fakeDynamicClient.Resource(policyReportGvr).Namespace(mngd.Namespace).Get(context.TODO(), mngd.Namespace+"-policyreport", v1.GetOptions{})
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
	assert.Equal(t, len(createdPolicyReport.Results), 2, "Expected 2 issues to be found. Got %v", len(createdPolicyReport.Results))
}

func Test_filterOpenshiftCategory(t *testing.T) {
	categories := []string{"test1", "openshift", "test2"}
	filtered := FilterOpenshiftCategory(categories)

	assert.Equal(t, "test1,test2", filtered, "Expected category list to exclude openshift")
}

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

func TestChangeSupportConfig(t *testing.T) {
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

	ret := NewRetriever(ts.URL, nil, 2*time.Second, "testToken")
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

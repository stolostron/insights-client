// Copyright Contributors to the Open Cluster Management project

package monitor

import (
	"context"
	"errors"
	"encoding/json"
	"io/ioutil"
	"testing"

	sanitize "github.com/kennygrant/sanitize"
	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	"github.com/stretchr/testify/assert"
	"github.com/open-cluster-management/insights-client/pkg/types"
)

func unmarshalFile(filepath string, resourceType interface{}, t *testing.T) {
	// open given filepath string
	rawBytes, err := ioutil.ReadFile("../../test-data/" + sanitize.Name(filepath))
	if err != nil {
		t.Fatal("Unable to read test data", err)
	}

	// unmarshal file into given resource type
	err = json.Unmarshal(rawBytes, resourceType)
	if err != nil {
		t.Fatalf("Unable to unmarshal json to type %T %s", resourceType, err)
	}
}

func Test_addCluster(t *testing.T) {
	monitor := NewClusterMonitor()
	managedCluster := clusterv1.ManagedCluster{}
	unmarshalFile("managed-cluster.json", &managedCluster, t)
	monitor.addCluster(&managedCluster)

	assert.Equal(t, types.ManagedClusterInfo{Namespace: "local-cluster", ClusterID: "323a00cd-428a-49fb-80ab-201d2a5d3050"}, monitor.ManagedClusterInfo[0], "Test Add ManagedCluster: local-cluster")

}

func Test_updateCluster(t *testing.T) {
	monitor := NewClusterMonitor()
	monitor.ManagedClusterInfo = []types.ManagedClusterInfo{{Namespace: "local-cluster", ClusterID: "123a00cd-428a-49fb-80ab-201d2a5d3050"}}
	managedCluster := clusterv1.ManagedCluster{}
	unmarshalFile("managed-cluster.json", &managedCluster, t)

	monitor.updateCluster(&managedCluster)

	assert.Equal(t, types.ManagedClusterInfo{Namespace: "local-cluster", ClusterID: "323a00cd-428a-49fb-80ab-201d2a5d3050"}, monitor.ManagedClusterInfo[0], "Test Add ManagedCluster: local-cluster")

}

func Test_deleteCluster(t *testing.T) {
	monitor := NewClusterMonitor()
	monitor.ManagedClusterInfo = []types.ManagedClusterInfo{{Namespace: "local-cluster", ClusterID: "323a00cd-428a-49fb-80ab-201d2a5d3050"}}

	managedCluster := clusterv1.ManagedCluster{}
	unmarshalFile("managed-cluster.json", &managedCluster, t)

	monitor.deleteCluster(&managedCluster)

	assert.Equal(t, []types.ManagedClusterInfo{}, monitor.ManagedClusterInfo, "Test Delete ManagedCluster: local-cluster")

}

func Test_FetchClusters(t *testing.T) {
	monitor := NewClusterMonitor()
	monitor.ManagedClusterInfo = []types.ManagedClusterInfo{{Namespace: "local-cluster", ClusterID: "323a00cd-428a-49fb-80ab-201d2a5d3050"}}

	fetchClusterIDs := make(chan types.ManagedClusterInfo)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go monitor.FetchClusters(ctx, fetchClusterIDs)
	testData := <-fetchClusterIDs

	assert.Equal(
		t,
		types.ManagedClusterInfo{Namespace: "local-cluster", ClusterID: "323a00cd-428a-49fb-80ab-201d2a5d3050"},
		testData,
		"Test Fetch ManagedCluster list",
	)
}

func Test_isClustermissing(t *testing.T) {
	resultFalse := isClusterMissing(nil)
	assert.Equal(t, false, resultFalse, "Test isClusterMissing - false")

	err := errors.New("could not find the requested resource")
	resultTrue := isClusterMissing(err)
	assert.Equal(t, true, resultTrue, "Test isClusterMissing - true")
}

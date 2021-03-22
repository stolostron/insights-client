// Copyright Contributors to the Open Cluster Management project

package monitor

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/open-cluster-management/insights-client/pkg/config"
	"github.com/open-cluster-management/insights-client/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
)

type Monitor struct {
    ManagedClusterInfo  []types.ManagedClusterInfo
    clusterPollInterval time.Duration // How often we want to update managed cluster list
}

func NewClusterMonitor() *Monitor {
    m := &Monitor{
        ManagedClusterInfo:   []types.ManagedClusterInfo{},
        clusterPollInterval:  1 * time.Minute,
    }
    return m
}

// WatchClusters - Watches ManagedCluster objects and updates clusterID list for Insights call.
func (m *Monitor) WatchClusters(input chan types.ManagedClusterInfo) {
	glog.Info("Begin ClusterWatch routine")

	dynamicClient := config.GetDynamicClient()
	dynamicFactory := dynamicinformer.NewDynamicSharedInformerFactory(dynamicClient, 60*time.Second)

	// Create GVR for ManagedCluster
	managedClusterGvr, _ := schema.ParseResourceArg("managedclusters.v1.cluster.open-cluster-management.io")

	//Create Informers for ManagedCluster
	managedClusterInformer := dynamicFactory.ForResource(*managedClusterGvr).Informer()

	// Create handlers for events
	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			m.processCluster(obj, input)
		},
		UpdateFunc: func(prev interface{}, next interface{}) {
			m.processCluster(next, input)
		},
	}

	// Add Handlers to both Informers
	managedClusterInformer.AddEventHandler(handlers)

	// Periodically check if the ManagedCluster resource exists
	go m.stopAndStartInformer("cluster.open-cluster-management.io/v1", managedClusterInformer)
}

// Stop and Start informer according to Rediscover Rate
func (m *Monitor) stopAndStartInformer(groupVersion string, informer cache.SharedIndexInformer) {
	var stopper chan struct{}
	informerRunning := false

	for {
		_, err := config.GetKubeClient().ServerResourcesForGroupVersion(groupVersion)
		// we fail to fetch for some reason other than not found
		if err != nil && !isClusterMissing(err) {
			glog.Errorf("Cannot fetch resource list for %s, error message: %s ", groupVersion, err)
		} else {
			if informerRunning && isClusterMissing(err) {
				glog.Infof("Stopping cluster informer routine because %s resource not found.", groupVersion)
				stopper <- struct{}{}
				informerRunning = false
			} else if !informerRunning && !isClusterMissing(err) {
				glog.Infof("Starting cluster informer routine for cluster watch for %s resource", groupVersion)
				stopper = make(chan struct{})
				informerRunning = true
				go informer.Run(stopper)
			}
		}
		time.Sleep(time.Duration(m.clusterPollInterval))
	}
}

var mux sync.Mutex

func isClusterMissing(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "could not find the requested resource")
}

func (m *Monitor) processCluster(obj interface{}, input chan types.ManagedClusterInfo) {
	// Lock so only one goroutine at a time can access add a cluster.
	// Helps to eliminate duplicate entries.
	mux.Lock()
	defer mux.Unlock()
	j, err := json.Marshal(obj.(*unstructured.Unstructured))
	if err != nil {
		glog.Warning("Error unmarshalling object from Informer in processCluster.")
	}

	// Unmarshall ManagedCluster
	managedCluster := clusterv1.ManagedCluster{}
	err = json.Unmarshal(j, &managedCluster)
	if err != nil {
		glog.Warning("Failed to Unmarshal MangedCluster", err)
	}
	m.transformManagedCluster(&managedCluster, input)
}

// Transform ManagedCluster grabs the cluster name & clusterID for Insights call
func (m *Monitor) transformManagedCluster(managedCluster *clusterv1.ManagedCluster, input chan types.ManagedClusterInfo) {
	var version int64
	var clusterVendor string
	var clusterID string
	for _, claimInfo := range managedCluster.Status.ClusterClaims {
		if claimInfo.Name == "product.open-cluster-management.io" {
			clusterVendor = claimInfo.Value
		}
		if claimInfo.Name == "version.openshift.io" {
			parsed, _ := strconv.ParseInt(claimInfo.Value[0:1], 10, 64)
			version = parsed
		}
		if claimInfo.Name == "id.openshift.io" {
			clusterID = claimInfo.Value
		}
	}

	// We only get Insights for OpenShift clusters versioned 4.x or greater.
	if clusterVendor == "OpenShift" && version >= 4 {
		input <- types.ManagedClusterInfo{ClusterID: clusterID, Namespace: managedCluster.GetName()}
		m.ManagedClusterInfo = append(m.ManagedClusterInfo, types.ManagedClusterInfo{ClusterID: clusterID, Namespace: managedCluster.GetName()})
	}
}

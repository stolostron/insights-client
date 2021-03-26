// Copyright Contributors to the Open Cluster Management project

package monitor

import (
	"context"
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
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
)

// Find returns a bool if the item exists in the given slice
func Find(slice []types.ManagedClusterInfo, val types.ManagedClusterInfo) (int, bool) {
	for i, item := range slice {
		if item.Namespace == val.Namespace {
			return i, true
		}
	}
	return -1, false
}

// GetClusterClaimInfo return the ManagedCluster vendor, version and ID
func GetClusterClaimInfo(managedCluster *clusterv1.ManagedCluster) (string, int64, string) {
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
	return clusterVendor, version, clusterID
}

// Monitor struct
type Monitor struct {
	ManagedClusterInfo  []types.ManagedClusterInfo
	clusterPollInterval time.Duration // How often we want to update managed cluster list
}

// NewClusterMonitor ...
func NewClusterMonitor() *Monitor {
	m := &Monitor{
		ManagedClusterInfo:  []types.ManagedClusterInfo{},
		clusterPollInterval: 1 * time.Minute,
	}
	return m
}

// WatchClusters - Watches ManagedCluster objects and updates clusterID list for Insights call.
func (m *Monitor) WatchClusters() {
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
			m.processCluster(obj, "add")
		},
		UpdateFunc: func(prev interface{}, next interface{}) {
			m.processCluster(next, "update")
		},
		DeleteFunc: func(obj interface{}) {
			m.processCluster(obj, "delete")
		},
	}

	// Add Handler to Informer
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

func (m *Monitor) processCluster(obj interface{}, handlerType string) {
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

	switch handlerType {
	case "add":
		m.addCluster(&managedCluster)
	case "update":
		m.updateCluster(&managedCluster)
	case "delete":
		m.deleteCluster(&managedCluster)
	default:
		glog.Warning("Process cluster received unknown type")
	}
}

func (m *Monitor) addCluster(managedCluster *clusterv1.ManagedCluster) {
	glog.V(2).Info("Processing Cluster Addition.")

	clusterVendor, version, clusterID := GetClusterClaimInfo(managedCluster)
	// We only get Insights for OpenShift clusters versioned 4.x or greater.
	if clusterVendor == "OpenShift" && version >= 4 {
		glog.Infof("Adding %s to Insights cluster list", managedCluster.GetName())
		m.ManagedClusterInfo = append(m.ManagedClusterInfo, types.ManagedClusterInfo{
			ClusterID: clusterID,
			Namespace: managedCluster.GetName(),
		})
	}
}

// Removes a ManagedCluster resource from ManagedClusterInfo list
func (m *Monitor) updateCluster(managedCluster *clusterv1.ManagedCluster) {
	glog.V(2).Info("Processing Cluster Update.")

	clusterToUpdate := managedCluster.GetName()
	clusterVendor, version, clusterID := GetClusterClaimInfo(managedCluster)
	clusterIdx, found := Find(m.ManagedClusterInfo, types.ManagedClusterInfo{
		Namespace: clusterToUpdate,
		ClusterID: clusterID,
	})
	if found && clusterID != m.ManagedClusterInfo[clusterIdx].ClusterID {
		// If the cluster ID has changed update it - otherwise do nothing.
		glog.Infof("Updating %s from Insights cluster list", clusterToUpdate)
		m.ManagedClusterInfo[clusterIdx] = types.ManagedClusterInfo{
			ClusterID: clusterID,
			Namespace: managedCluster.GetName(),
		}
		return
	}

	// Case to add a ManagedCluster to cluster list after it has been upgraded to version >= 4.X
	if !found && clusterVendor == "OpenShift" && version >= 4 {
		glog.Infof("Adding %s to Insights cluster list - Cluster was upgraded", managedCluster.GetName())
		m.ManagedClusterInfo = append(m.ManagedClusterInfo, types.ManagedClusterInfo{
			ClusterID: clusterID,
			Namespace: managedCluster.GetName(),
		})
	}
}

// Removes a ManagedCluster resource from ManagedClusterInfo list
func (m *Monitor) deleteCluster(managedCluster *clusterv1.ManagedCluster) {
	glog.V(2).Info("Processing Cluster Delete.")

	clusterToDelete := managedCluster.GetName()
	for clusterIdx, cluster := range m.ManagedClusterInfo {
		if clusterToDelete == cluster.Namespace {
			glog.Infof("Removing %s from Insights cluster list", clusterToDelete)
			m.ManagedClusterInfo = append(m.ManagedClusterInfo[:clusterIdx], m.ManagedClusterInfo[clusterIdx+1:]...)
		}
	}
}

// FetchClusters forwards the managed clusters to RetrieveCCXReports function
func (m *Monitor) FetchClusters(ctx context.Context, input chan types.ManagedClusterInfo) {
	wait.Until(func() {
		for _, cluster := range m.ManagedClusterInfo {
			glog.Infof("Starting to get  cluster report for  %s", cluster)
			input <- cluster
		}
	}, m.clusterPollInterval, ctx.Done())
}

// GetLocalCluster Get Local cluster ID
func (m *Monitor) GetLocalCluster() string {
	glog.V(2).Info("Get Local Cluster ID.")
	for _, cluster := range m.ManagedClusterInfo {
		if "local-cluster" == cluster.Namespace {
			return cluster.ClusterID
		}
	}
	return "-1"
}

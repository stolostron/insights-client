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
		glog.V(2).Infof("Adding %s to Insights cluster list", managedCluster.GetName())
		m.ManagedClusterInfo = append(m.ManagedClusterInfo, types.ManagedClusterInfo{ClusterID: clusterID, Namespace: managedCluster.GetName()})
	}
}

// Removes a ManagedCluster resource from ManagedClusterInfo list
func (m *Monitor) updateCluster(managedCluster *clusterv1.ManagedCluster) {
	glog.V(2).Info("Processing Cluster Update.")

	var clusterID string 
	clusterToUpdate := managedCluster.GetName()
	for _, claimInfo := range managedCluster.Status.ClusterClaims {
		if claimInfo.Name == "id.openshift.io" {
			clusterID = claimInfo.Value
		}
	}
	for clusterIdx, cluster := range m.ManagedClusterInfo {
		if clusterToUpdate == cluster.Namespace && clusterID != cluster.ClusterID {
			// If the cluster ID has changed update it - otherwise do nothing.
			glog.V(2).Infof("Updating %s from Insights cluster list", clusterToUpdate)
			m.ManagedClusterInfo[clusterIdx] = types.ManagedClusterInfo{ClusterID: clusterID, Namespace: managedCluster.GetName()}
		}
	}
}

// Removes a ManagedCluster resource from ManagedClusterInfo list
func (m *Monitor) deleteCluster(managedCluster *clusterv1.ManagedCluster) {
	glog.V(2).("Processing Cluster Delete.")

	clusterToDelete := managedCluster.GetName()
	for clusterIdx, cluster := range m.ManagedClusterInfo {
		if clusterToDelete == cluster.Namespace {
			glog.V(2).Infof("Removing %s from Insights cluster list", clusterToDelete)
			m.ManagedClusterInfo = append(m.ManagedClusterInfo[:clusterIdx], m.ManagedClusterInfo[clusterIdx+1:]...)
		}
	}
}
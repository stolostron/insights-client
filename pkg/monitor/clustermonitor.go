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
	"github.com/stolostron/insights-client/pkg/config"
	"github.com/stolostron/insights-client/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

var lock = sync.RWMutex{}
var localClusterName = "local-cluster"

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
		if clusterID == "" && claimInfo.Name == "id.k8s.io" {
			clusterID = claimInfo.Value
		}
	}
	return clusterVendor, version, clusterID
}

// Monitor struct
type Monitor struct {
	ManagedClusterInfo  []types.ManagedClusterInfo
	ClusterNeedsCCX     map[string]bool
	ClusterPollInterval time.Duration // How often we want to update managed cluster list
}

var m *Monitor

// NewClusterMonitor ...
func NewClusterMonitor() *Monitor {
	if m != nil {
		return m
	}
	m = &Monitor{
		ManagedClusterInfo:  []types.ManagedClusterInfo{},
		ClusterNeedsCCX:     map[string]bool{},
		ClusterPollInterval: time.Duration(config.Cfg.PollInterval) * time.Minute,
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
		time.Sleep(time.Duration(m.ClusterPollInterval))
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
	glog.V(2).Infof("Currently mangaging %d clusters.", len(m.ManagedClusterInfo))
	// We add the local cluster during Initialization.Using the method GetLocalCluster
	if managedCluster.GetName() == localClusterName {
		return
	}
	clusterVendor, version, clusterID := GetClusterClaimInfo(managedCluster)
	if clusterID == "" {
		//cluster not imported properly, do not process
		glog.V(2).Info("Empty Cluster Id - Skipping Cluster Addition.")
		return
	}
	glog.Infof("Adding %s to all cluster list", managedCluster.GetName())
	lock.Lock()
	defer lock.Unlock()
	m.ManagedClusterInfo = append(m.ManagedClusterInfo, types.ManagedClusterInfo{
		ClusterID: clusterID,
		Namespace: managedCluster.GetName(),
	})
	glog.V(2).Infof("Currently mangaging %d clusters.", len(m.ManagedClusterInfo))
	// We only get Insights for OpenShift clusters versioned 4.x or greater.
	if clusterVendor == "OpenShift" && version >= 4 {
		glog.Infof("Adding %s to Insights cluster list", managedCluster.GetName())
		m.ClusterNeedsCCX[clusterID] = true
	} else {
		m.ClusterNeedsCCX[clusterID] = false
	}
}

// Removes a ManagedCluster resource from ManagedClusterInfo list
func (m *Monitor) updateCluster(managedCluster *clusterv1.ManagedCluster) {
	glog.V(2).Info("Processing Cluster Update.")
	glog.V(2).Infof("Currently mangaging %d clusters.", len(m.ManagedClusterInfo))
	lock.Lock()
	defer lock.Unlock()
	clusterToUpdate := managedCluster.GetName()
	if clusterToUpdate == "local-cluster" {
		// We get local-clsuter ID from clusterversion resource.
		// Dont update the clusterID here as it can be undefined.
		return
	}

	clusterVendor, version, clusterID := GetClusterClaimInfo(managedCluster)
	clusterIdx, found := Find(m.ManagedClusterInfo, types.ManagedClusterInfo{
		Namespace: clusterToUpdate,
		ClusterID: clusterID,
	})
	if found && clusterID != m.ManagedClusterInfo[clusterIdx].ClusterID {
		// If the cluster ID has changed update it - otherwise do nothing.
		glog.Infof("Updating %s from Insights cluster list", clusterToUpdate)
		if oldCluster, ok := m.ClusterNeedsCCX[m.ManagedClusterInfo[clusterIdx].ClusterID]; ok {
			glog.Infof("old cluster %s ", oldCluster)
			m.ClusterNeedsCCX[clusterID] = oldCluster
			delete(m.ClusterNeedsCCX, m.ManagedClusterInfo[clusterIdx].ClusterID)
			m.ManagedClusterInfo[clusterIdx] = types.ManagedClusterInfo{
				ClusterID: clusterID,
				Namespace: clusterToUpdate,
			}
		}
		return
	}

	// Case to add a ManagedCluster to cluster list after it has been upgraded to version >= 4.X
	// Or Cluster was missed during Add event
	if !found && clusterID != "" {
		glog.Infof("Adding %s to to all cluster list,missed from Add ", managedCluster.GetName())
		m.ManagedClusterInfo = append(m.ManagedClusterInfo, types.ManagedClusterInfo{
			ClusterID: clusterID,
			Namespace: clusterToUpdate,
		})
		if clusterVendor == "OpenShift" && version >= 4 {
			glog.Infof("Adding %s to Insights cluster list", managedCluster.GetName())
			m.ClusterNeedsCCX[clusterID] = true
		} else {
			m.ClusterNeedsCCX[clusterID] = false
		}
	}
}

// Removes a ManagedCluster resource from ManagedClusterInfo list
func (m *Monitor) deleteCluster(managedCluster *clusterv1.ManagedCluster) {
	glog.V(2).Info("Processing Cluster Delete.")
	glog.V(2).Infof("Currently mangaging %d clusters.", len(m.ManagedClusterInfo))
	lock.Lock()
	defer lock.Unlock()
	clusterToDelete := managedCluster.GetName()
	for clusterIdx, cluster := range m.ManagedClusterInfo {
		if clusterToDelete == cluster.Namespace {
			glog.Infof("Removing %s from Insights cluster list", clusterToDelete)
			delete(m.ClusterNeedsCCX, m.ManagedClusterInfo[clusterIdx].ClusterID)
			m.ManagedClusterInfo = append(m.ManagedClusterInfo[:clusterIdx], m.ManagedClusterInfo[clusterIdx+1:]...)
		}
	}
	glog.V(2).Infof("Currently mangaging %d clusters.", len(m.ManagedClusterInfo))
}

// AddLocalCluster - adds local cluster to Clusters list
func (m *Monitor) AddLocalCluster(versionObj *unstructured.Unstructured) bool {
	var clusterVersionGvr = schema.GroupVersionResource{
		Group:    "config.openshift.io",
		Version:  "v1",
		Resource: "clusterversions",
	}
	var dynamicClient dynamic.Interface
	var err error
	glog.V(2).Info("Adding Local Cluster ID.")
	if versionObj == nil {
		dynamicClient = config.GetDynamicClient()
		versionObj, err = dynamicClient.Resource(clusterVersionGvr).Get(context.TODO(), "version", metav1.GetOptions{})
	}
	if err != nil {
		glog.V(2).Infof("Failed to get clusterversions : %v", err)
		return false
	}
	clusterID, _, err := unstructured.NestedString(versionObj.Object, "spec", "clusterID")
	if err != nil {
		glog.V(2).Infof("Failed to get OCP clusterID from version: %v", err)
		return false
	}
	// If the cluster ID is not empty add to list and return true
	if clusterID != "" {
		lock.Lock()
		defer lock.Unlock()
		m.ManagedClusterInfo = append(m.ManagedClusterInfo, types.ManagedClusterInfo{
			ClusterID: clusterID,
			Namespace: localClusterName,
		})
		m.ClusterNeedsCCX[clusterID] = true
		return true
	}

	return false
}

// GetLocalCluster - GET ID from Clusters list
func (m *Monitor) GetLocalCluster() string {
	glog.V(2).Info("Getting local-cluster id .")
	for _, cluster := range m.ManagedClusterInfo {
		if localClusterName == cluster.Namespace {
			return cluster.ClusterID
		}
	}
	return ""
}

//Getter for ManagedClusterInfo
func (m *Monitor) GetManagedClusterInfo() []types.ManagedClusterInfo {
	lock.Lock()
	defer lock.Unlock()
	glog.V(2).Infof("Total managed clusters in processing list  %d .", len(m.ManagedClusterInfo))
	return m.ManagedClusterInfo
}

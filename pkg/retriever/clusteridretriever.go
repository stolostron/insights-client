// Copyright Contributors to the Open Cluster Management project

package retriever

import (
	"context"
	"strconv"
    "time"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/util/wait"
    "k8s.io/apimachinery/pkg/runtime/schema"
    "k8s.io/apimachinery/pkg/runtime"

    "github.com/golang/glog"
    "github.com/open-cluster-management/insights-client/pkg/config"
    "github.com/open-cluster-management/insights-client/pkg/types"
)

type Monitor struct {
    clusterIDs           []string
    clusterPollInterval time.Duration // How often we want to update managed cluster list
}

func NewClusterMonitor() *Monitor {
    c := &Monitor{
        clusterIDs:           []string{},
        clusterPollInterval:  1 * time.Minute,
    }
    return c
}

func (c *Monitor) UpdateClusterIDs(ctx context.Context, input chan string) {
    wait.Until(func() {
        glog.V(2).Info("Refreshing cluster list")
        if err := c.getManagedClusters(ctx); err != nil {
            glog.Warningf("Unable to retrieve managed clusters for update: %v", err)
        }
        for _, cluster := range c.clusterIDs {
         glog.Infof("Starting to get  cluster report for  %s", cluster)
         input <- cluster
        }
    }, c.clusterPollInterval, ctx.Done())
}

func (c *Monitor) getManagedClusters(ctx context.Context) error {
    client := config.GetDynamicClient()
    opts := metav1.ListOptions{}
    clusterList, err := client.Resource(
        schema.GroupVersionResource{Group: "cluster.open-cluster-management.io", Version: "v1", Resource: "managedclusters"},
    ).List(ctx, opts)
    if err != nil {
        glog.Warningf("Unable to retrieve managed clusters for update: %v", err)
    } else if clusterList != nil {
        for i := range clusterList.Items {
            var clusterType types.ManagedCluster
            err := runtime.DefaultUnstructuredConverter.FromUnstructured(clusterList.Items[i].UnstructuredContent(), &clusterType)
            if err != nil {
                glog.Fatalf("Unable to unmarshal mockclusters json %v", err)
            }
            var version int64
            var clusterVendor string
            var clusterID string
            for _, claimInfo := range clusterType.Status.ClusterClaims {
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
                c.clusterIDs = append(c.clusterIDs, clusterID)
			}
        }
    }

    return nil
}

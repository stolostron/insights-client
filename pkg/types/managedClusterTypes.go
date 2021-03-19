// Copyright Contributors to the Open Cluster Management project
package types

type ManagedClusterInfo struct {
    ClusterID string
    Namespace string
}

type ClusterClaims struct {
    Name string `json:"name"`
    Value string `json:"value"`
}

type ManagedClusterStatus struct {
    ClusterClaims []ClusterClaims `json:"clusterClaims"`
}

type ManagedCluster struct {
    Meta struct {
        Name string `json:"name"`
    } `json:"metadata"`
    Status ManagedClusterStatus `json:"status"`
}

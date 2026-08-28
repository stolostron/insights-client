package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/stolostron/insights-client/pkg/config"
	"github.com/stolostron/insights-client/pkg/monitor"
	"github.com/stolostron/insights-client/pkg/processor"
	"github.com/stolostron/insights-client/pkg/retriever"
	"github.com/stolostron/insights-client/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func main() {
	flag.Parse()
	err := flag.Lookup("logtostderr").Value.Set("true")
	if err != nil {
		fmt.Println("Error setting default flag:", err)
		os.Exit(1)
	}
	defer glog.Flush()

	glog.Info("Starting insights-client")
	if commit, ok := os.LookupEnv("VCS_REF"); ok {
		glog.Info("Built from git commit: ", commit)
	}

	config.SetupConfig()

	dynamicClient := config.GetDynamicClient()

	// Read the cluster's APIServer TLS security profile and poll for changes.
	// On change, the poller exits the process so the Deployment controller restarts
	// the pod with the updated TLS config.
	tlsPollCtx := context.Background()
	tlsCfg, initialProfile, profileOK := config.GetTLSConfig(tlsPollCtx, dynamicClient)
	go config.PollAPIServerTLSProfile(tlsPollCtx, dynamicClient, initialProfile, profileOK)
	fetchClusterIDs := make(chan types.ManagedClusterInfo)
	fetchPolicyReports := make(chan types.ProcessorData)

	monitor := monitor.NewClusterMonitor()
	go monitor.WatchClusters()

	// Set up Retriever and cache the Insights data.
	// The Retriever connects to an external endpoint (CCX/Insights), so it uses
	// a permissive TLS config instead of the cluster profile — the external server's
	// TLS requirements are outside the cluster admin's control.
	ret := retriever.NewRetriever(config.Cfg.CCXServer, nil, config.Cfg.CCXToken, nil)
	//Wait for hub cluster id to make GET API call
	hubID := "-1"
	for hubID == "-1" {
		var versionResource *unstructured.Unstructured
		//If Local cluster is added and is not empty, get hub ID
		if monitor.AddLocalCluster(versionResource) && monitor.GetLocalCluster() != "" {
			hubID = monitor.GetLocalCluster()
		}
		glog.Info("Waiting for local-cluster Id.")
		time.Sleep(2 * time.Second)
	}

	// Fetch the reports for each cluster & create the PolicyReport resources for each violation.
	go ret.RetrieveReport(hubID, fetchClusterIDs, fetchPolicyReports, monitor.ClusterNeedsCCX, ret.DisconnectedEnv)

	processor := processor.NewProcessor()
	go processor.ProcessPolicyReports(fetchPolicyReports, dynamicClient)

	refreshToken := config.Cfg.CCXToken != "" || ret.DisconnectedEnv
	//start triggering reports for clusters
	go ret.FetchClusters(monitor, fetchClusterIDs, refreshToken, hubID, dynamicClient)

	router := mux.NewRouter()

	srv := &http.Server{
		Addr:              config.Cfg.ServicePort,
		Handler:           router,
		TLSConfig:         tlsCfg,
		ReadHeaderTimeout: time.Duration(config.Cfg.HTTPTimeout) * time.Millisecond,
		ReadTimeout:       time.Duration(config.Cfg.HTTPTimeout) * time.Millisecond,
		WriteTimeout:      time.Duration(config.Cfg.HTTPTimeout) * time.Millisecond,
		TLSNextProto:      make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}

	glog.Info("insights-client listening on", config.Cfg.ServicePort)
	log.Fatal(srv.ListenAndServeTLS("./sslcert/tls.crt", "./sslcert/tls.key"),
		" Use ./setup.sh to generate certificates for local development.")
}

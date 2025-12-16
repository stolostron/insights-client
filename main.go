package main

import (
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
	fetchClusterIDs := make(chan types.ManagedClusterInfo)
	fetchPolicyReports := make(chan types.ProcessorData)

	monitor := monitor.NewClusterMonitor()
	go monitor.WatchClusters()

	// Set up Retriever and cache the Insights data
	ret := retriever.NewRetriever(config.Cfg.CCXServer, nil, config.Cfg.CCXToken)
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

	// Configure TLS
	cfg := &tls.Config{
		MinVersion:               tls.VersionTLS12,
		CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		},
	}
	srv := &http.Server{
		Addr:              config.Cfg.ServicePort,
		Handler:           router,
		TLSConfig:         cfg,
		ReadHeaderTimeout: time.Duration(config.Cfg.HTTPTimeout) * time.Millisecond,
		ReadTimeout:       time.Duration(config.Cfg.HTTPTimeout) * time.Millisecond,
		WriteTimeout:      time.Duration(config.Cfg.HTTPTimeout) * time.Millisecond,
		TLSNextProto:      make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}

	glog.Info("insights-client listening on", config.Cfg.ServicePort)
	log.Fatal(srv.ListenAndServeTLS("./sslcert/tls.crt", "./sslcert/tls.key"),
		" Use ./setup.sh to generate certificates for local development.")
}

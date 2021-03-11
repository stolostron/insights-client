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
	"github.com/open-cluster-management/insights-client/pkg/config"
	"github.com/open-cluster-management/insights-client/pkg/handlers"
	"github.com/open-cluster-management/insights-client/pkg/retriever"
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

	retriever.NewRetriever()
	//go retriever.RetrieveCCXReport(fetch, reports)

	router := mux.NewRouter()

	router.HandleFunc("/liveness", handlers.LivenessProbe).Methods("GET")
	router.HandleFunc("/readiness", handlers.ReadinessProbe).Methods("GET")

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

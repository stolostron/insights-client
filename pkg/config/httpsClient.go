/*
IBM Confidential
OCO Source Materials
(C) Copyright IBM Corporation 2019 All Rights Reserved
The source code for this program is not published or otherwise divested of its trade secrets,
irrespective of what has been deposited with the U.S. Copyright Office.
*/
// Copyright (c) 2020 Red Hat, Inc.

package config

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net/http"

	"github.com/golang/glog"
)

// getHTTPSClient description....
func getHTTPSClient() (client http.Client) {
	// Hub deployment: Generate TLS config using the mounted certificates.
	caCert, err := ioutil.ReadFile("./sslcert/tls.crt")
	if err != nil {
		// Exit because this is an unrecoverable configuration problem.
		glog.Fatal("Error loading TLS certificate from mounted secret. Certificate must be mounted at ./sslcert/tls.crt  Original error: ", err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	cert, err := tls.LoadX509KeyPair("./sslcert/tls.crt", "./sslcert/tls.key")
	if err != nil {
		// Exit because this is an unrecoverable configuration problem.
		glog.Fatal("Error loading TLS certificate from mounted secret. Certificate must be mounted at ./sslcert/tls.crt and ./sslcert/tls.key  Original error: ", err)
	}

	// Configure TLS
	tlsCfg := &tls.Config{
		MinVersion:               tls.VersionTLS12,
		CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		},
		RootCAs:            caCertPool,
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true, // TODO flag for dev vs. prod
	}

	tr := &http.Transport{
		TLSClientConfig: tlsCfg,
	}

	client = http.Client{Transport: tr}

	return client
}

// KubeRequest - client to perform all kube requests
func KubeRequest(method string, url string, body []byte) (response []byte) {
	// TODO need to see if this client works in prod
	httpClient := getHTTPSClient()
	req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
	if err != nil {
		glog.Error(err)
	}
	// TODO /var/run/secrets/kubernetes.io/serviceaccount/token??
	req.Header.Set("Authorization", "Bearer ")
	// postReq.Header.Set("Content-Type", "application/json")
	reqResp, reqErr := httpClient.Do(req)
	if reqErr != nil {
		glog.Error(reqErr)
	}
	defer reqResp.Body.Close()

	reponseBody, responseErr := ioutil.ReadAll(reqResp.Body)
	if responseErr != nil {
		glog.Error(responseErr)
	}
	return reponseBody
}

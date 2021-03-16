// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
package mocks

import (
	"io/ioutil"
	"os"

	"github.com/golang/glog"
)

func GetMockData(clusterId string) []byte {

	jsonFile, err := os.Open("../utils/" + clusterId + ".json")
	if err != nil {
		pwd, _ := os.Getwd()
		glog.Errorf("Error opening %s.json file in the dir %s ", clusterId, pwd)
	}
	defer jsonFile.Close()
	mock_data, _ := ioutil.ReadAll(jsonFile)
	return mock_data
}

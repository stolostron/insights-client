// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
package mocks

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/golang/glog"
)

func GetMockData(clusterId string) []byte {

	dirName := "../utils/"
	fileName := clusterId + ".json"
	jsonFile, err := os.Open(filepath.Join(dirName, fileName))
	if err != nil {
		pwd, _ := os.Getwd()
		glog.Errorf("Error opening %s.json file in the dir %s ", clusterId, pwd)
	}
	defer jsonFile.Close()
	mock_data, _ := ioutil.ReadAll(jsonFile)
	return mock_data
}

// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
package mocks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
)

func GetMockData(clusterId string) []byte {

	fileName := "../utils/" + clusterId + ".json"
	cleanFile := filepath.Clean(fileName)
	if !strings.HasPrefix(cleanFile, "../utils") {
		panic(fmt.Errorf("unsafe input"))
	}
	mock_data, err := os.ReadFile(cleanFile)
	if err != nil {
		pwd, _ := os.Getwd()
		glog.Errorf("Error opening %s.json file in the dir %s ", clusterId, pwd)
	}
	return mock_data
}

func GetMockContent() []byte {

	fileName := "../utils/" + "content.json"
	cleanFile := filepath.Clean(fileName)
	if !strings.HasPrefix(cleanFile, "../utils") {
		panic(fmt.Errorf("unsafe input"))
	}
	mock_data, err := os.ReadFile(cleanFile)
	if err != nil {
		pwd, _ := os.Getwd()
		glog.Errorf("Error opening content.json file in the dir %s ", pwd)
	}
	return mock_data
}

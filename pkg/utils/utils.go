// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
package mocks

/*func getMockData(clusterId string) types.ResponseBody {

	var response types.ResponseBody
	jsonFile, err := os.Open("./pkg/utils/" + clusterId + ".json")
	if err != nil {
		pwd, _ := os.Getwd()
		glog.Errorf("Error opening %s.json file in the dir %s ", clusterId, pwd)
	}
	defer jsonFile.Close()
	mock_data, _ := ioutil.ReadAll(jsonFile)
	err = json.Unmarshal(mock_data, &response)
	if err != nil {
		glog.Fatalf("Unable to unmarshal mockclusters json %v", err)
	}
	return response
}*/

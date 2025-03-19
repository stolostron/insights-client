# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project
#!/bin/bash


echo $1

IMAGE_NAME=$1
echo "IMAGE: " $IMAGE_NAME

DEFAULT_NS="open-cluster-management"
HUB_KUBECONFIG=$HOME/.kube/kind-config-hub
WORKDIR=`pwd`
INSIGHTS_CLIENT_CCX_TOKEN="testtoken"

sed_command='sed -i-e -e'
if [[ "$(uname)" == "Darwin" ]]; then
	sed_command='sed -i '-e' -e'
fi

deploy() {
    setup_kubectl_and_oc_command
	create_kind_hub
	initial_setup
	test_content_and_local_cluster_report
	swap_back_to_templates
}


setup_kubectl_and_oc_command() {
	echo "=====Setup kubectl and oc=====" 
	# kubectl required for kind
	# oc client required for installing operators
	# if and when we are feeling ambitious... also download the installer and install ocp, and run our component integration test here	
	# uname -a and grep mac or something...
    # Darwin MacBook-Pro 19.5.0 Darwin Kernel Version 19.5.0: Tue May 26 20:41:44 PDT 2020; root:xnu-6153.121.2~2/RELEASE_X86_64 x86_64
	echo "Install kubectl and oc from openshift mirror (https://mirror.openshift.com/pub/openshift-v4/clients/ocp/4.18.4/openshift-client-mac-4.18.4.tar.gz)" 
	mv README.md README.md.tmp 
    if [[ "$(uname)" == "Darwin" ]]; then # then we are on a Mac 
	    WORKDIR=`pwd`
		curl -LO https://mirror.openshift.com/pub/openshift-v4/clients/ocp/4.18.4/openshift-client-mac-4.18.4.tar.gz 
		tar xzvf openshift-client-mac-4.18.4.tar.gz  # xzf to quiet logs
		rm openshift-client-mac-4.18.4.tar.gz
    elif [[ "$(uname)" == "Linux" ]]; then # we are in prow, building in rhel 
		cat /etc/redhat-release
		cat /etc/os-release
		uname -r
	    WORKDIR=/tmp/insights-client
		curl -LO https://mirror.openshift.com/pub/openshift-v4/clients/ocp/4.18.4/openshift-client-linux-amd64-rhel8.tar.gz
		tar xzvf openshift-client-linux-amd64-rhel8.tar.gz  # xzf to quiet logs
		rm openshift-client-linux-amd64-rhel8.tar.gz
    fi
	# this package has a binary, so:

	echo "Current directory"
	echo $(pwd)
	mv README.md.tmp README.md 
	chmod +x ./kubectl
	if [[ ! -f /usr/bin/kubectl ]]; then
		sudo cp ./kubectl /usr/bin/kubectl
	fi
	chmod +x ./oc
	if [[ ! -f /usr/bin/oc ]]; then
		sudo cp ./oc /usr/bin/oc
	fi
	# kubectl and oc are now installed in current dir 
	echo -n "kubectl version" && kubectl version
 	echo -n "oc version" && oc version 
}
 
create_kind_hub() { 
    if [[ ! -f /usr/local/bin/kind ]]; then
    	echo "=====Create kind cluster=====" 
    	echo "Install kind from (https://kind.sigs.k8s.io/)."
    
    	# uname returns your operating system name
    	# uname -- Print operating system name
    	# -L location, lowercase -o specify output name, uppercase -O. Write output to a local file named like the remote file we get  
    	curl -Lo ./kind "https://kind.sigs.k8s.io/dl/v0.27.0/kind-$(uname)-amd64"
    	chmod +x ./kind
    	sudo cp ./kind /usr/local/bin/kind
    fi
    echo "Delete hub if it exists"
    kind delete cluster --name hub || true
    
    echo "Start hub cluster" 
    rm -rf $HOME/.kube/kind-config-hub
    kind create cluster --kubeconfig $HOME/.kube/kind-config-hub --name hub --config ${WORKDIR}/test-data/e2e/kind-hub-config.yaml
    # kubectl cluster-info --context kind-hub --kubeconfig $(pwd)/.kube/kind-config-hub # confirm connection 
    export KUBECONFIG=$HOME/.kube/kind-config-hub
	echo "KUBECONFIG" && echo $KUBECONFIG
} 


delete_kind_hub() {
	echo "====Delete kind cluster====="
    kind delete cluster --name hub
}

delete_command_binaries(){
	cd ${WORKDIR}/..
	echo "Current directory"
	echo $(pwd)
	rm ./kind
	rm ./kubectl
	rm ./oc
}

initial_setup() {
    echo $WORKDIR
	cd ${WORKDIR}
    echo "=====Deploying insights-client====="
	$sed_command "s~{{ INSIGHTS_CLIENT_IMAGE }}~$IMAGE_NAME~g" ./test-data/e2e/insights-chart/templates/insights-deployment.yaml
	$sed_command "s~{{ INSIGHTS_CLIENT_CCX_TOKEN }}~$INSIGHTS_CLIENT_CCX_TOKEN~g" ./test-data/e2e/insights-chart/templates/insights-deployment.yaml
    
	echo "Current directory"
	echo $(pwd)
	echo -n "Create namespace open-cluster-management: " && kubectl create namespace open-cluster-management
	echo -n "Switch to namespace: " && kubectl config set-context --current --namespace open-cluster-management
    echo -n "Creating pull secret: " && kubectl create secret docker-registry search-operator-pull-secret --docker-server=quay.io --docker-username=$DOCKER_USER --docker-password=$DOCKER_PASS

    #create dummy ssl certs for service 
	./setup.sh
    kubectl create secret generic insights-client-certs --from-file=./sslcert/tls.crt --from-file=./sslcert/tls.key
    echo -n "Applying cluster versions CRD:" && kubectl apply -f ./test-data/e2e/clusterversions.config.openshift.io.yaml
    echo -n "Applying version CR:" && kubectl apply -f ./test-data/e2e/version.yaml
    echo -n "Applying managedclusters CRD:" && kubectl apply -f ./test-data/e2e/managedclusters.yaml
    echo -n "Applying local-cluster managedclusters CR:" && kubectl apply -f ./test-data/e2e/local-clusterCR.yaml

    echo -n "Installing Insights client deployment :" && kubectl apply -f ./test-data/e2e/insights-chart/templates
	cat ./test-data/e2e/insights-chart/templates/insights-deployment.yaml
   
    echo -n "Waiting 100s for initial setup deployment."
    sleep 100s
}

test_content_and_local_cluster_report(){
	pod=$(oc get pods | grep insights-client | cut -d' ' -f1)
	oc logs $pod
	log=$(oc logs $pod) 
	log_msg=$(echo $log| grep "Creating Content Request for cluster 69365d80-c4ef-4999-8417-f14c8e2892c2")
	if [[ "$log_msg" == *"local-cluster"* ]]; then
	    echo "Pod log has policy report"
	else
		echo "Pod log did not have expected output"
	    exit 1
    fi
	
}

swap_back_to_templates(){
# # change the image name back to INSIGHTS_CLIENT_IMAGE in operator deployment for next run
sed -i '-e' "s~$IMAGE_NAME~{{ INSIGHTS_CLIENT_IMAGE }}~g" ./test-data/e2e/insights-chart/templates/insights-deployment.yaml
sed -i '-e' "s~$INSIGHTS_CLIENT_CCX_TOKEN~{{ INSIGHTS_CLIENT_CCX_TOKEN }}~g" ./test-data/e2e/insights-chart/templates/insights-deployment.yaml
}



deploy

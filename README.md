# insights-client

Uses Red Hat Connected Customer Experience (CCX) to provide health check insights . The insights-client will create a Custom Resource (PolicyReport) for each insight specific to each cluster under management.

# Running Local on MacOS
  Run `make build` ( Requires golang version 1.16)
  Setup KUBECONFIG to connect to Kubernetes cluster
  Export CCX_SERVER environemnt variable to connect to your Insights API  
  Run `setup.sh`
  Run `make run`

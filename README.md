# insights-client

Uses Red Hat Connected Customer Experience (CCX) to provide health check insights for all managed cluster (clusters have to be Openshift versioned >= 4.X). The insights-client will create a Custom Resource (PolicyReport) for each cluster that contains all Insight violations.

## Development

1. Install dependencies
    ```
    make deps
    ```
2. Generate self-signed certificate for development
    ```
    sh setup.sh
    ```
3. Log into your development cluster with `oc login ...`.
    > **Alternative:** set the `KUBECONFIG` environment variable to some other kubernetes config file.
4. Run the program
    ```
    make run
    ```

### Environment Variables
Control the behavior of this service with these environment variables.

Name             | Required | Default Value                           | Description
---------------- | -------- | --------------------------------------- | -----------
HTTP_TIMEOUT     | no       | 180000                                  | 3 minute timeout to process a single requests
CCX_SERVER       | no       | http://localhost:8080/api/v1/clusters   | CCX server url (prod will use: `https://cloud.redhat.com/api/insights-results-aggregator/v1`)
CCX_TOKEN        | no       | Not set                                 | If not set client will get cloud.openshift.com token from secret `openshift-config`
POLL_INTERVAL    | no       | 30                                      | 30 minute default polling interval cloud.redhat.com
REQUEST_INTERVAL | no       | 1                                       | 1 second Interval between 2 consecutive Insights requests
CACERT           | no       | Not set                                 | Used for dev & test ONLY


Rebuild: 2021-09-22

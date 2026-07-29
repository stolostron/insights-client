# Network Policies

The `multiclusterhub-operator` deploys a [`NetworkPolicy`](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
for insights-client as part of the Insights Helm chart (`pkg/templates/charts/toggle/insights/`),
following the principle of least privilege: **deny by default, allow only the specific traffic
each component needs.** The policy is created alongside the other Insights resources and is
automatically removed if the Insights component is disabled on the MultiClusterHub CR.

## Design principles

- **Pod-scoped selector.** The `NetworkPolicy` selects only insights-client pods
  (`name: insights-client`), never the whole namespace. This is important because
  insights-client shares a namespace (`open-cluster-management` in a typical ACM install)
  with unrelated ACM components — a namespace-wide policy would inadvertently restrict
  traffic for pods that aren't part of Insights.
- **`policyTypes: [Ingress]` only.** OVN-Kubernetes (the default CNI on OpenShift) handles
  `kubernetes.default.svc` ClusterIP traffic through the OVN service load balancer *before*
  NetworkPolicy evaluation, so no egress rule type can match kube-API traffic. Applying an
  Egress policyType would silently block insights-client from reaching the Kubernetes API.
  insights-client also needs egress to the external CCX/Insights endpoint
  (`console.redhat.com`), which further rules out deny-all egress.
- **Well-known namespace labels.** Ingress rules that reference OpenShift system namespaces
  use the `kubernetes.io/metadata.name` label, which the API server automatically stamps on
  every namespace (Kubernetes 1.21+). This avoids relying on custom labels that may not exist
  in every cluster.

## Component network flows

### insights-client

| Direction | Peer | Port | Rationale |
|---|---|---|---|
| Ingress | *(none allowed)* | — | insights-client does not serve any active endpoints. It runs an HTTP/TLS listener on port 3030 but has no registered route handlers, so no external traffic needs to reach it. The policy selects the pod and declares `Ingress` policyType with no ingress rules, effectively denying all inbound traffic. |
| Egress | *(not restricted — Ingress-only policy)* | — | insights-client requires egress to the Kubernetes API (watch ManagedClusters, read pull-secrets, CRUD PolicyReports, read ClusterVersions and APIServer config) and to the external CCX/Insights endpoint (`console.redhat.com`, port 443) to fetch cluster reports. OVN-Kubernetes handles `kubernetes.default.svc` ClusterIP traffic via the OVN service load balancer before NetworkPolicy evaluation, so no egress rule can match kube-API traffic. Applying an Egress policyType would silently block both API server access and external report retrieval. |

## Testing

Because these policies are enforced by the cluster's CNI plugin (not the Kubernetes API server),
functional verification — confirming that legitimate traffic still flows and that traffic
outside these rules is blocked — requires testing against a real cluster with a
NetworkPolicy-enforcing CNI (e.g. OVN-Kubernetes on OpenShift).

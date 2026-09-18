# Implementation survey

This page compares how Gateway API implementations customize the infrastructure behind a Gateway.

## Comparison at a glance

| Implementation | Attachment | Customization model |
| --- | --- | --- |
| [Cilium](https://docs.cilium.io/en/stable/network/servicemesh/gateway-api/parameterized-gatewayclass/) | `GatewayClass.spec.parametersRef` | Typed CRD |
| [Istio](https://istio.io/latest/docs/tasks/traffic-management/ingress/gateway-api/) | `Gateway.spec.infrastructure.parametersRef` | ConfigMap patches |
| [kgateway](https://kgateway.dev/docs/envoy/latest/setup/customize/) | `GatewayClass.spec.parametersRef` | Typed API plus overlays |
| [Envoy Gateway](https://gateway.envoyproxy.io/docs/tasks/operations/customize-envoyproxy/) | GatewayClass or Gateway `parametersRef` | Typed API plus patches |
| [GKE](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/gatewayclass-capabilities) | Predefined GatewayClasses | Managed cloud capabilities |
| [NGINX Gateway Fabric](https://docs.nginx.com/nginx-gateway-fabric/how-to/data-plane-configuration/) | GatewayClass or Gateway `parametersRef` | Typed API plus patches |
| [Traefik](https://doc.traefik.io/traefik/reference/install-configuration/providers/kubernetes/kubernetes-gateway/) | Helm/provider configuration | Existing Service |
| [Kong Operator](https://docs.konghq.com/gateway-operator/latest/topologies/dbless/) | `GatewayClass.spec.parametersRef` | Typed DataPlane API |

## Service field paths

The paths below are relative to the referenced implementation-specific parameters object. They are not fields on the standard `Gateway` object itself.

<details>
<summary><strong>Cilium</strong></summary>

`CiliumGatewayClassConfig.spec.service.type` supports `LoadBalancer` and `NodePort`.

`CiliumGatewayClassConfig.spec.service.externalTrafficPolicy` sets the generated Service field.

</details>

<details>
<summary><strong>Istio</strong></summary>

Istio uses a ConfigMap strategic merge patch. The paths inside the patch are:

```yaml
data:
  service: |
    spec:
      type: ClusterIP
      externalTrafficPolicy: Local
```

These are patch paths, not typed fields on the ConfigMap itself.

</details>

<details>
<summary><strong>kgateway</strong></summary>

`GatewayParameters.spec.kube.service.type`

`GatewayParameters.spec.kube.service.externalTrafficPolicy`

</details>

<details>
<summary><strong>Envoy Gateway</strong></summary>

`EnvoyProxy.spec.provider.kubernetes.envoyService.type`

`EnvoyProxy.spec.provider.kubernetes.envoyService.externalTrafficPolicy`

</details>

<details>
<summary><strong>GKE</strong></summary>

GKE does not expose a user-managed proxy Service field for its Google-hosted Gateway data plane. Service type and `externalTrafficPolicy` are therefore not the relevant customization path.

</details>

<details>
<summary><strong>NGINX Gateway Fabric</strong></summary>

`NginxProxy.spec.kubernetes.service.type`

`NginxProxy.spec.kubernetes.service.externalTrafficPolicy`

</details>

<details>
<summary><strong>Traefik</strong></summary>

Traefik normally uses an existing Service configured through Helm or Kubernetes. The Gateway API provider does not expose these as per-Gateway typed fields.

</details>

<details>
<summary><strong>Kong Operator</strong></summary>

`GatewayConfiguration.spec.dataplaneOptions.network.services.ingress.type`

`GatewayConfiguration.spec.dataplaneOptions.network.services.ingress.externalTrafficPolicy`

</details>

## The attachment patterns

### Class-wide configuration

```yaml
kind: GatewayClass
spec:
  parametersRef:
    group: example.openshift.io
    kind: GatewayParameters
    name: shared-defaults
    namespace: openshift-ingress
```

This is appropriate for platform-owned defaults shared by every Gateway using the class.

### Per-Gateway configuration

```yaml
kind: Gateway
spec:
  infrastructure:
    parametersRef:
      group: example.openshift.io
      kind: GatewayParameters
      name: private-gateway
```

This is appropriate when one Gateway needs different capacity, exposure, or scheduling. It raises an authorization question: who may reference which parameters object?

### Indirect configuration

Some implementations configure the proxy before the Gateway exists. Helm values, an existing Service, or a cloud-provider load balancer become the source of truth. Traefik and GKE are the clearest examples.

## What gets customized?

| Area | Common implementation choices |
| --- | --- |
| Proxy capacity | replicas, resource requests and limits, HPA, VPA, PDB |
| Pod placement | node selectors, affinity, topology spread, tolerations |
| Service exposure | type, ports, annotations, IP families, load balancer class |
| Traffic entry | `externalTrafficPolicy`, source ranges, readiness or health-check ports |
| Process configuration | environment variables, image, command flags, bootstrap configuration |
| Proxy behavior | logging, telemetry, protocol settings, client-IP handling |

## Field-level infrastructure comparison

This matrix compares the Kubernetes infrastructure customizations exposed through each implementation's Gateway customization mechanism. It is a snapshot of the linked documentation as of September 2026. It does not attempt to compare routing, security, or proxy-runtime policy APIs.

Legend:

- ✅ — first-class, typed field
- 🧩 — supported through a generated-resource patch or Kubernetes template
- 🏷️ — selected indirectly through a predefined GatewayClass
- — — not exposed through the surveyed Gateway customization mechanism

When both a typed field and a patch can configure a value, the matrix shows ✅. The **arbitrary patch** rows identify which implementations additionally expose the underlying Kubernetes object without limiting users to the listed fields.

Abbreviations: **EG** is Envoy Gateway and **NGF** is NGINX Gateway Fabric.

### Service

| Customization | Cilium | Istio | kgateway | EG | GKE | NGF | Traefik | Kong |
| --- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| Labels | — | 🧩 | ✅ | ✅ | — | 🧩 | — | ✅ |
| Annotations | — | 🧩 | ✅ | ✅ | — | 🧩 | — | ✅ |
| Service type | ✅ | 🧩 | ✅ | ✅ | 🏷️ | ✅ | — | ✅ |
| Service `clusterIP` | — | 🧩 | ✅ | 🧩 | — | 🧩 | — | — |
| Listener ports or NodePorts | — | 🧩 | ✅ | 🧩 | — | ✅ | — | ✅ |
| `externalTrafficPolicy` | ✅ | 🧩 | ✅ | ✅ | — | ✅ | — | ✅ |
| `internalTrafficPolicy` | — | 🧩 | 🧩 | 🧩 | — | 🧩 | — | ✅ |
| `trafficDistribution` | ✅ | 🧩 | 🧩 | 🧩 | — | 🧩 | — | ✅ |
| `allocateLoadBalancerNodePorts` | ✅ | 🧩 | 🧩 | ✅ | — | 🧩 | — | — |
| `loadBalancerClass` | ✅ | 🧩 | ✅ | ✅ | — | ✅ | — | — |
| `loadBalancerIP` | — | 🧩 | 🧩 | ✅ | — | ✅ | — | — |
| `loadBalancerSourceRanges` | ✅ | 🧩 | ✅ | ✅ | — | ✅ | — | — |
| Source-range allow or deny policy | ✅ | — | — | — | — | — | — | — |
| IP families and IP-family policy | ✅ | 🧩 | 🧩 | 🧩 | — | 🧩 | — | — |
| Arbitrary Service patch | — | 🧩 | 🧩 | 🧩 | — | 🧩 | — | — |

GKE chooses load-balancer exposure, scope, and implementation through its predefined GatewayClasses rather than exposing a user-managed proxy Service. Traefik's Gateway provider uses the Service created at installation time; Helm configuration is therefore outside this per-Gateway comparison.

### Deployment and Pod

| Customization | Cilium | Istio | kgateway | EG | GKE | NGF | Traefik | Kong |
| --- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| Deployment labels and annotations | — | 🧩 | 🧩 | 🧩 | — | 🧩 | — | ✅ |
| Replicas | — | 🧩 | ✅ | ✅ | — | ✅ | — | ✅ |
| Deployment update strategy | — | 🧩 | ✅ | ✅ | — | 🧩 | — | — |
| Pod labels and annotations | — | 🧩 | ✅ | ✅ | — | 🧩 | — | 🧩 |
| Node selector | — | 🧩 | ✅ | ✅ | — | ✅ | — | 🧩 |
| Affinity | — | 🧩 | ✅ | ✅ | — | ✅ | — | 🧩 |
| Tolerations | — | 🧩 | ✅ | ✅ | — | ✅ | — | 🧩 |
| Topology spread constraints | — | 🧩 | ✅ | ✅ | — | ✅ | — | 🧩 |
| Priority class | — | 🧩 | ✅ | ✅ | — | 🧩 | — | 🧩 |
| Image pull secrets | — | 🧩 | ✅ | ✅ | — | 🧩 | — | 🧩 |
| Pod security context | — | 🧩 | ✅ | ✅ | — | 🧩 | — | 🧩 |
| Termination grace period | — | 🧩 | ✅ | 🧩 | — | ✅ | — | 🧩 |
| Volumes | — | 🧩 | ✅ | ✅ | — | ✅ | — | 🧩 |
| Arbitrary Deployment patch | — | 🧩 | 🧩 | 🧩 | — | 🧩 | — | — |

Kong applies a Kubernetes `PodTemplateSpec` to its generated Deployment using strategic merge patch semantics. The matrix therefore marks Pod-level Kong settings as 🧩 even though the embedded Kubernetes type supplies schema for those fields.

### Proxy container

| Customization | Cilium | Istio | kgateway | EG | GKE | NGF | Traefik | Kong |
| --- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| Image | — | 🧩 | ✅ | ✅ | — | ✅ | — | 🧩 |
| Environment variables | — | 🧩 | ✅ | ✅ | — | 🧩 | — | 🧩 |
| Resource requests and limits | — | 🧩 | ✅ | ✅ | — | ✅ | — | 🧩 |
| Container security context | — | 🧩 | ✅ | ✅ | — | 🧩 | — | 🧩 |
| Volume mounts | — | 🧩 | ✅ | ✅ | — | ✅ | — | 🧩 |
| Startup probe | — | 🧩 | ✅ | 🧩 | — | 🧩 | — | 🧩 |
| Readiness probe | — | 🧩 | ✅ | 🧩 | — | ✅ | — | 🧩 |
| Liveness probe | — | 🧩 | ✅ | 🧩 | — | 🧩 | — | 🧩 |
| Container lifecycle hooks | — | 🧩 | 🧩 | 🧩 | — | ✅ | — | 🧩 |
| Host ports | — | 🧩 | 🧩 | 🧩 | — | ✅ | — | 🧩 |
| Init containers or additional sidecars | — | 🧩 | 🧩 | 🧩 | — | 🧩 | — | 🧩 |

### HorizontalPodAutoscaler

| Customization | Cilium | Istio | kgateway | EG | GKE | NGF | Traefik | Kong |
| --- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| Create or enable an HPA | — | 🧩 | 🧩 | ✅ | — | ✅ | — | ✅ |
| Minimum and maximum replicas | — | 🧩 | 🧩 | ✅ | — | ✅ | — | ✅ |
| CPU or memory utilization targets | — | 🧩 | 🧩 | ✅ | — | ✅ | — | ✅ |
| Arbitrary HPA metrics | — | 🧩 | 🧩 | ✅ | — | ✅ | — | ✅ |
| Scale-up and scale-down behavior | — | 🧩 | 🧩 | ✅ | — | ✅ | — | ✅ |
| Arbitrary HPA patch | — | 🧩 | 🧩 | 🧩 | — | — | — | — |

### PodDisruptionBudget

| Customization | Cilium | Istio | kgateway | EG | GKE | NGF | Traefik | Kong |
| --- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| Create a PDB | — | 🧩 | 🧩 | ✅ | — | ✅ | — | ✅ |
| `minAvailable` or `maxUnavailable` | — | 🧩 | 🧩 | ✅ | — | ✅ | — | ✅ |
| Unhealthy-pod eviction policy | — | 🧩 | 🧩 | 🧩 | — | ✅ | — | ✅ |
| Arbitrary PDB patch | — | 🧩 | 🧩 | 🧩 | — | — | — | — |

### Other generated resources

| Customization | Cilium | Istio | kgateway | EG | GKE | NGF | Traefik | Kong |
| --- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| ServiceAccount name | — | 🧩 | — | ✅ | — | — | — | — |
| ServiceAccount labels or annotations | — | 🧩 | ✅ | — | — | — | — | — |
| Arbitrary ServiceAccount patch | — | 🧩 | 🧩 | — | — | — | — | — |
| Run the proxy as a DaemonSet | — | — | — | ✅ | — | ✅ | — | — |
| Create a VerticalPodAutoscaler | — | — | 🧩 | — | — | — | — | — |

### Scope and interpretation

- Istio accepts strategic merge patches for complete Service, Deployment, ServiceAccount, HPA, and PDB objects. Its 🧩 marks therefore indicate broad Kubernetes API reach, not individually designed or validated Istio fields.
- kgateway combines typed fields with strategic merge overlays. It can patch Service, Deployment, and ServiceAccount resources and can create HPA, VPA, and PDB resources from overlays.
- Envoy Gateway combines typed infrastructure fields with patches on Service, Deployment or DaemonSet, HPA, and PDB resources.
- NGINX Gateway Fabric exposes typed Deployment, DaemonSet, Pod, container, Service, HPA, and PDB fields, plus patches for Deployment, DaemonSet, and Service.
- Kong exposes typed Service, scaling, and PDB fields, and applies a Kubernetes Pod template to the generated Deployment.
- Cilium's parameterized GatewayClass currently concentrates its Kubernetes infrastructure API on the generated Service. Its other parameters configure Envoy behavior and telemetry rather than generated workload objects.
- GKE and Traefik are intentionally sparse in this matrix because neither exposes the same per-Gateway generated-resource customization model: GKE provides managed GatewayClasses, while Traefik uses installation-level workload and Service configuration.

### Sources

- [Cilium `CiliumGatewayClassConfig`](https://docs.cilium.io/en/stable/network/servicemesh/gateway-api/parameterized-gatewayclass/)
- [Istio automated Gateway deployment](https://istio.io/latest/docs/tasks/traffic-management/ingress/gateway-api/#automated-deployment)
- [kgateway customization options](https://kgateway.dev/docs/envoy/latest/setup/customize/options/)
- [Envoy Gateway extension API](https://gateway.envoyproxy.io/docs/api/extension_types/)
- [GKE GatewayClass capabilities](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/gatewayclass-capabilities)
- [NGINX Gateway Fabric API reference](https://docs.nginx.com/nginx-gateway-fabric/reference/api/)
- [Traefik Kubernetes Gateway provider](https://doc.traefik.io/traefik/reference/install-configuration/providers/kubernetes/kubernetes-gateway/)
- [Kong Operator custom-resource reference](https://developer.konghq.com/operator/reference/custom-resources/)

## The key tradeoff

Typed fields are easier to validate, document, and preserve across upgrades. Patches are more expressive, but they expose generated-resource structure and can make upgrades harder to reason about.

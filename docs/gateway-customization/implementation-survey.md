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

## The key tradeoff

Typed fields are easier to validate, document, and preserve across upgrades. Patches are more expressive, but they expose generated-resource structure and can make upgrades harder to reason about.

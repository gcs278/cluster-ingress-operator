# Implementation survey

This page compares how Gateway API implementations customize the infrastructure behind a Gateway.

## Comparison at a glance

| Implementation | Attachment | Service model | Frontend `ClusterIP` | `externalTrafficPolicy` |
| --- | --- | --- | --- | --- |
| [Cilium](https://docs.cilium.io/en/stable/network/servicemesh/gateway-api/parameterized-gatewayclass/) | `GatewayClass.spec.parametersRef` → `CiliumGatewayClassConfig` | Narrow typed subset | Unsupported by `spec.service.type` (only `LoadBalancer`/`NodePort`) | `spec.service.externalTrafficPolicy` |
| [Istio](https://istio.io/latest/docs/tasks/traffic-management/ingress/gateway-api/) | `Gateway.spec.infrastructure.parametersRef` → `ConfigMap` | Strategic merge patches | `service.spec.type` in the ConfigMap patch | `service.spec.externalTrafficPolicy` in the ConfigMap patch |
| [kgateway](https://kgateway.dev/docs/envoy/latest/setup/customize/) | `GatewayClass.spec.parametersRef` → `GatewayParameters` | Typed proxy configuration plus patches | `spec.kube.service.type` | `spec.kube.service.externalTrafficPolicy` |
| [Envoy Gateway](https://gateway.envoyproxy.io/docs/tasks/operations/customize-envoyproxy/) | GatewayClass or Gateway `parametersRef` → `EnvoyProxy` | Typed API plus patches | `spec.provider.kubernetes.envoyService.type` | `spec.provider.kubernetes.envoyService.externalTrafficPolicy` |
| [GKE](https://docs.cloud.google.com/kubernetes-engine/docs/how-to/gatewayclass-capabilities) | Predefined Google-managed GatewayClasses | Managed cloud load balancer | Not applicable to a proxy Service | Usually not applicable |
| [NGINX Gateway Fabric](https://docs.nginx.com/nginx-gateway-fabric/how-to/data-plane-configuration/) | GatewayClass or Gateway `parametersRef` → `NginxProxy` | Typed API plus patches | `spec.kubernetes.service.type` | `spec.kubernetes.service.externalTrafficPolicy` |
| [Traefik](https://doc.traefik.io/traefik/reference/install-configuration/providers/kubernetes/kubernetes-gateway/) | Helm/provider configuration | Existing Service, managed outside Gateway | Indirectly | Indirectly |
| [Kong Operator](https://docs.konghq.com/gateway-operator/latest/topologies/dbless/) | `GatewayClass.spec.parametersRef` → `GatewayConfiguration` | Typed DataPlane and Service configuration | `spec.dataplaneOptions.network.services.ingress.type` | `spec.dataplaneOptions.network.services.ingress.externalTrafficPolicy` |

Field paths are relative to the referenced implementation-specific parameters object unless the row explicitly says that the value is a patch. They are not fields on the standard `Gateway` object itself.

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

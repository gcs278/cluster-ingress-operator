# Implementation survey

This page compares how Gateway API implementations customize the infrastructure behind a Gateway.

## Comparison at a glance

| Implementation | Attachment | Service model | Frontend `ClusterIP` | `externalTrafficPolicy` |
| --- | --- | --- | --- | --- |
| Cilium | `GatewayClass.spec.parametersRef` → `CiliumGatewayClassConfig` | Narrow typed subset | No documented generated type | Typed field |
| Istio | `Gateway.spec.infrastructure.parametersRef` → `ConfigMap` | Strategic merge patches | Via Service patch | Via Service patch |
| kgateway | `GatewayClass.spec.parametersRef` → `GatewayParameters` | Typed proxy configuration plus patches | Typed field | Typed field |
| Envoy Gateway | GatewayClass or Gateway `parametersRef` → `EnvoyProxy` | Typed API plus patches | Typed field | Typed field |
| GKE | Predefined Google-managed GatewayClasses | Managed cloud load balancer | Not applicable to a proxy Service | Usually not applicable |
| NGINX Gateway Fabric | GatewayClass or Gateway `parametersRef` → `NginxProxy` | Typed API plus patches | Typed field | Typed field |
| Traefik | Helm/provider configuration | Existing Service, managed outside Gateway | Indirectly | Indirectly |
| Kong Operator | `GatewayClass.spec.parametersRef` → `GatewayConfiguration` | Typed DataPlane and Service configuration | Typed field | Typed field |

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


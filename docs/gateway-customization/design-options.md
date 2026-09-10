# OpenShift design options

## Option A: reuse `IngressController`

Use the existing OpenShift operator API as the source of proxy configuration.

### Shape

```yaml
kind: GatewayClass
spec:
  parametersRef:
    group: operator.openshift.io
    kind: IngressController
    name: default
```

### Advantages

- Familiar OpenShift API and operational model.
- Existing support for placement, certificates, logging, and capacity concepts.
- Less duplication between Route/Ingress and Gateway implementations.

### Costs

- The resource models an OpenShift ingress controller, not an individual Gateway.
- Its lifecycle and namespace semantics may not fit per-Gateway infrastructure.
- Changes could couple Gateway API evolution to a mature, cluster-level API.

## Option B: create a typed `GatewayParameters` resource

Define an OpenShift-specific resource for Gateway infrastructure.

```yaml
apiVersion: gateway.networking.k8s.io/v1alpha1
kind: GatewayParameters
metadata:
  name: default
spec:
  deployment:
    replicas: 3
    resources:
      requests:
        cpu: 200m
        memory: 256Mi
  service:
    type: LoadBalancer
    externalTrafficPolicy: Local
  autoscaling:
    minReplicas: 2
    maxReplicas: 10
```

### Advantages

- Clear ownership and validation boundaries.
- API can model Gateway-specific concepts directly.
- Easier to document supported fields and compatibility guarantees.

### Costs

- A new API must be versioned, supported, and integrated with the operator.
- New supported use cases require new API fields.

## Initial recommendation

Start with a typed `GatewayParameters` resource, attachable from both `GatewayClass` and `Gateway`, and define explicit precedence:

```text
operator defaults
    < GatewayClass parameters
    < Gateway parameters
```

Translate the typed API to Istio internally. Do not expose the Istio ConfigMap patch mechanism to users.

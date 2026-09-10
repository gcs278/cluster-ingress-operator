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
- Some users will still need escape hatches for unusual Kubernetes fields.

## Option C: typed fields plus a bounded patch

Expose common fields directly and provide a restricted patch for advanced cases.

```yaml
spec:
  deployment:
    resources: {}
    replicas: 3
  service:
    type: ClusterIP
    externalTrafficPolicy: Cluster
  patches:
  - target: Deployment
    patch: |
      spec:
        template:
          spec:
            containers:
            - name: proxy
              env:
              - name: EXAMPLE
                value: value
```

### Advantages

- Common use cases remain strongly typed.
- Less pressure to add a new field for every proxy or platform feature.
- Similar to patterns used by Istio, Envoy Gateway, and NGINX Gateway Fabric.

### Costs

- Patches expose generated-resource details.
- Validation, security, and upgrade behavior need careful limits.
- Patches can conflict with fields controlled by the operator.

## Initial recommendation

Start with a typed `GatewayParameters` resource, attachable from both `GatewayClass` and `Gateway`, and define explicit precedence:

```text
operator defaults
    < GatewayClass parameters
    < Gateway parameters
```

Add a narrow patch mechanism only after identifying use cases that cannot be represented safely with typed fields.


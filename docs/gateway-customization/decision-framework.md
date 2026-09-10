# Design decision framework

The design should be discussed as a sequence of decisions. Each level narrows the choices below it.

## 1. Do we need an API?

**Decision: customization contract**

The likely answer is yes. OpenShift owns and reconciles generated Deployments, Services, HPAs, and related resources. An API gives users supported fields, validation, and implementation independence.

`ClusterIP` and `externalTrafficPolicy` might be handled through Service-specific workarounds, but that creates unclear ownership and reconciliation behavior. They should be evaluated as API fields even if some implementations can support them indirectly.

## 2. Where does customization enter Gateway API?

**Decision: extension-point strategy**

| Option | Meaning |
| --- | --- |
| Parameters API | Use `GatewayClass.spec.parametersRef` and `Gateway.spec.infrastructure.parametersRef`. |
| GatewayClass profiles | Offer predefined classes such as internal, external, or high-availability. |
| Hybrid | Use profiles for broad operating modes and parameters for supported overrides. |

“GatewayClass enumeration” is better described as **GatewayClass profiles**. It is a preset strategy, not a general customization API.

## 3. How is the configuration attached?

**Decision: attachment and ownership**

The cleanest model is:

```text
GatewayClass.parametersRef       -> class-wide defaults
Gateway.infrastructure.parametersRef -> one Gateway’s configuration
```

OpenShift should read these references rather than mutate the user’s Gateway. That avoids GitOps drift. Name matching and label-based discovery can remain implementation details if needed for Istio translation.

## 4. What kind of API is exposed?

**Decision: schema exposure**

| Option | Tradeoff |
| --- | --- |
| Passthrough | Flexible, but exposes Istio/generated-resource details. |
| Fully abstracted | Safest and most portable, but requires fields for each supported use case. |
| Typed plus bounded patches | Good escape hatch, but needs strict validation and ownership rules. |

The recommended starting point is a typed OpenShift API. Add a bounded escape hatch only for demonstrated use cases that cannot be modeled safely.

## 5. How is each field designed?

For every field, record:

- user intent;
- generated resource and Kubernetes field;
- proposed OpenShift field;
- scope: GatewayClass, Gateway, or both;
- defaults and validation;
- mutability and replacement behavior;
- ownership and status reporting.

Suggested field groups:

- capacity: replicas, resources, HPA;
- placement: node selectors, affinity, tolerations;
- Service: type, `ClusterIP`, `NodePort`, traffic policies;
- health: readiness, liveness, external health checks;
- process: environment variables and logging.

See the [field deep dives](deep-dives/external-traffic-policy.md) for examples.

## 6. What are the complete designs?

After the individual decisions, compare complete bundles in [design bundles](design-bundles.md). This keeps one design from hiding several unrelated choices.


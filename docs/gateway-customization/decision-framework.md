# Design decision framework

The design should be discussed as a sequence of decisions. Each level narrows the choices below it.

```mermaid
flowchart TD
    subgraph L1["Level 1 — Need"]
        A[Do we need a supported customization interface?]
        B[Change global defaults or use Istio's existing customization mechanism]
        A -->|No| B
    end

    subgraph L2["Level 2 — Extension point"]
        C[Choose extension-point strategy]
        D[Parameters API]
        E[GatewayClass profiles]
        F[Hybrid profiles plus parameters]
        C --> D
        C --> E
        C --> F
    end

    subgraph L3["Level 3 — Attachment"]
        G[Choose attachment and ownership]
        H[GatewayClass parametersRef]
        T[GatewayClass plus Gateway parametersRef]
        R[Resource Selection<br/>(targetRef)]
        G --> H
        G --> T
        G --> R
    end

    subgraph L4["Level 4 — Schema"]
        I[Choose schema exposure]
        J[Abstracted, typed OpenShift API]
        K[Implementation passthrough]
        I --> J
        I --> K
    end

    subgraph L5["Level 5 — Fields"]
        M[Design individual fields]
    end

    subgraph L6["Level 6 — Lifecycle"]
        N[Define lifecycle, status, and ownership]
        O[Compare complete design bundles]
        N --> O
    end

    A -->|Yes| C
    D --> G
    F --> G
    E --> N
    H --> I
    T --> I
    R --> I
    J --> M
    K --> N
    M --> N
```

The GatewayClass profiles branch is discussed in the [Gateway API implementation-specific GatewayClass proposal](https://github.com/openshift/enhancements/pull/1990).

---

## Level 1 — Do we need a supported customization interface?

**Decision: supported customization interface**

The likely answer is yes. OpenShift owns and reconciles generated Deployments, Services, HPAs, and related resources. An API gives users supported fields, validation, and implementation independence.

`ClusterIP` and `externalTrafficPolicy` might be handled through Service-specific workarounds, but that creates unclear ownership and reconciliation behavior. They should be evaluated as API fields even if some implementations can support them indirectly.

---

## Level 2 — Where does customization enter Gateway API?

**Decision: extension-point strategy**

| Option | Meaning |
| --- | --- |
| Parameters API | Use `GatewayClass.spec.parametersRef` and `Gateway.spec.infrastructure.parametersRef`. |
| GatewayClass profiles | Offer predefined classes such as internal, external, or high-availability. |
| Hybrid | Use profiles for broad operating modes and parameters for supported overrides. |

“GatewayClass enumeration” is better described as **GatewayClass profiles**. It is a preset strategy, not a general customization API.

---

## Level 3 — How is the configuration attached?

**Decision: attachment and ownership**

The main options are:

| Option | Meaning |
| --- | --- |
| GatewayClass `parametersRef` | The `GatewayClass` points to class-wide parameters. |
| GatewayClass plus Gateway `parametersRef` | The class provides defaults and the Gateway provides per-Gateway overrides. |
| Resource Selection (`targetRef`) | The customization object points to a `Gateway` or `GatewayClass` using a reference or selector. |
| Name matching | A convention such as `GatewayCustomization/<GatewayClass name>` selects the target. |

The GatewayClass-only model is:

```text
GatewayClass.parametersRef -> class-wide defaults
```

The two-level model is:

```text
GatewayClass.parametersRef            -> class-wide defaults
Gateway.infrastructure.parametersRef -> one Gateway’s overrides
```

OpenShift should read these references rather than mutate the user’s Gateway. That avoids GitOps drift. Name matching and label-based discovery can remain implementation details if needed for Istio translation.

With Resource Selection, the customization object owns the relationship:

```yaml
kind: GatewayCustomization
spec:
  targetRef:
    kind: Gateway
    name: internal
```

This can be useful when the customization must be managed by a platform namespace, but it raises cross-namespace authorization questions. Mutating the Gateway to add a generated `parametersRef` can also create GitOps drift.

---

## Level 4 — What kind of API is exposed?

**Decision: schema exposure**

| Option | Tradeoff |
| --- | --- |
| Passthrough | Flexible, but exposes Istio/generated-resource details. |
| Abstracted, typed API | Safest and most portable, but requires fields for each supported use case. |

The proposed direction is an abstracted, typed OpenShift API. Users should not patch generated Istio or Kubernetes resources. New supported use cases should be added as reviewed API fields.

---

## Level 5 — How is each field designed?

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

---

## Level 6 — What are the complete designs?

After the individual decisions, compare complete bundles in [design bundles](design-bundles.md). This keeps one design from hiding several unrelated choices.

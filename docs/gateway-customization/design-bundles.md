# Complete design bundles

These bundles combine the decisions from the [decision framework](decision-framework.md).

## A. Parameters API

Use standard Gateway API references to attach a typed OpenShift resource:

```text
GatewayClass.parametersRef
Gateway.infrastructure.parametersRef
        |
        v
OpenShift GatewayParameters
        |
        v
Istio ConfigMap and generated resources
```

This provides class defaults, per-Gateway overrides, validation, and a translation layer to Istio.

## B. GatewayClass profiles

Provide platform-owned profiles such as `openshift-internal`, `openshift-external`, and `openshift-high-availability`.

This is simple and safe, but it does not scale well when users need independent combinations of replicas, HPA, resources, Service type, and placement.

## C. Profiles plus parameters

Use the GatewayClass for the broad operating mode and a parameters object for supported details.

This offers the most flexibility, but requires clear precedence between profile defaults, class parameters, and Gateway parameters.

## D. OpenShift translation to Istio

Expose a typed OpenShift API, then translate those fields internally into the resources Istio requires.

This keeps the public API independent of Istio. Users do not create or edit the Istio ConfigMap directly.

## Initial direction

The strongest starting point is **A plus D**:

- use standard `parametersRef` attachment;
- expose a typed OpenShift API;
- translate that API to Istio internally;
- keep generated-resource patches internal to the implementation.

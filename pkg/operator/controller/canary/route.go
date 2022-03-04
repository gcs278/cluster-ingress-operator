package canary

import (
	"context"
	"fmt"
	operatorv1 "github.com/openshift/api/operator/v1"

	"github.com/openshift/cluster-ingress-operator/pkg/manifests"
	"github.com/openshift/cluster-ingress-operator/pkg/operator/controller"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	routev1 "github.com/openshift/api/route/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
)

// ensureCanaryRoute ensures the canary route exists
func (r *reconciler) ensureCanaryRoute(service *corev1.Service, ic *operatorv1.IngressController) (bool, *routev1.Route, error) {
	desired, err := desiredCanaryRoute(service, ic)
	if err != nil {
		return false, nil, fmt.Errorf("failed to build canary route: %v", err)
	}

	haveRoute, current, err := r.currentCanaryRoute()
	if err != nil {
		return false, nil, err
	}

	switch {
	case !haveRoute:
		if err := r.createCanaryRoute(desired); err != nil {
			return false, nil, err
		}
		return r.currentCanaryRoute()
	case haveRoute:
		if updated, err := r.updateCanaryRoute(current, desired); err != nil {
			return true, current, err
		} else if updated {
			return r.currentCanaryRoute()
		}
	}

	return true, current, nil
}

// currentCanaryRoute gets the current canary route resource
func (r *reconciler) currentCanaryRoute() (bool, *routev1.Route, error) {
	route := &routev1.Route{}
	if err := r.client.Get(context.TODO(), controller.CanaryRouteName(), route); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, route, nil
}

// createCanaryRoute creates the given route
func (r *reconciler) createCanaryRoute(route *routev1.Route) error {
	if err := r.client.Create(context.TODO(), route); err != nil {
		return fmt.Errorf("failed to create canary route %s/%s: %v", route.Namespace, route.Name, err)
	}

	log.Info("created canary route", "namespace", route.Namespace, "name", route.Name)
	return nil
}

// updateCanaryRoute updates the canary route if an appropriate change
// has been detected
func (r *reconciler) updateCanaryRoute(current, desired *routev1.Route) (bool, error) {
	changed, updated := canaryRouteChanged(current, desired)
	if !changed {
		return false, nil
	}

	// Diff before updating because the client may mutate the object.
	diff := cmp.Diff(current, updated, cmpopts.EquateEmpty())
	if err := r.client.Update(context.TODO(), updated); err != nil {
		return false, fmt.Errorf("failed to update canary route %s/%s: %v", updated.Namespace, updated.Name, err)
	}
	log.Info("updated canary route", "namespace", updated.Namespace, "name", updated.Name, "diff", diff)
	return true, nil
}

// deleteCanaryRoute deletes a given route
func (r *reconciler) deleteCanaryRoute(route *routev1.Route) (bool, error) {

	if err := r.client.Delete(context.TODO(), route); err != nil {
		return false, fmt.Errorf("failed to delete canary route %s/%s: %v", route.Namespace, route.Name, err)
	}

	log.Info("deleted canary route", "namespace", route.Namespace, "name", route.Name)
	return true, nil
}

// canaryRouteChanged returns true if current and expected differ by Spec.Port,
// Spec.To, or Spec.TLS.
func canaryRouteChanged(current, expected *routev1.Route) (bool, *routev1.Route) {
	changed := false
	updated := current.DeepCopy()

	if !cmp.Equal(current.Spec.Port, expected.Spec.Port, cmpopts.EquateEmpty()) {
		updated.Spec.Port = expected.Spec.Port
		changed = true
	}

	if !cmp.Equal(current.Spec.To, expected.Spec.To, cmpopts.EquateEmpty()) {
		updated.Spec.To = expected.Spec.To
		changed = true
	}

	if !cmp.Equal(current.Spec.TLS, expected.Spec.TLS, cmpopts.EquateEmpty()) {
		updated.Spec.TLS = expected.Spec.TLS
		changed = true
	}

	if !cmp.Equal(current.ObjectMeta.Labels, expected.ObjectMeta.Labels, cmpopts.EquateEmpty()) {
		updated.ObjectMeta.Labels = expected.ObjectMeta.Labels
		changed = true
	}

	if !changed {
		return false, nil
	}
	return true, updated
}

// desiredCanaryRoute returns the desired canary route read in
// from manifests
func desiredCanaryRoute(service *corev1.Service, ic *operatorv1.IngressController) (*routev1.Route, error) {
	route := manifests.CanaryRoute()

	name := controller.CanaryRouteName()

	route.Namespace = name.Namespace
	route.Name = name.Name

	if service == nil {
		return route, fmt.Errorf("expected non-nil canary service for canary route %s/%s", route.Namespace, route.Name)
	}

	route.Labels = map[string]string{
		// associate the route with the canary controller
		manifests.OwningIngressCanaryCheckLabel: canaryControllerName,
	}

	// BZ2024946 & BZ2021446: Add ingress controller's route selectors to route labels.
	if ic.Spec.RouteSelector != nil && ic.Spec.RouteSelector.MatchLabels != nil {
		for k, v := range ic.Spec.RouteSelector.MatchLabels {
			route.Labels[k] = v
		}
	}

	route.Spec.To.Name = controller.CanaryServiceName().Name

	// Set spec.port.targetPort to the first port available in the canary service.
	// The canary controller may toggle which targetPort the route targets
	// to test > 1 endpoint, so it does not matter which port is selected as long
	// as the canary service has > 1 ports available. If the canary service only has one
	// available port, then route.Spec.Port.TargetPort will remain unchanged.
	if len(service.Spec.Ports) == 0 {
		return route, fmt.Errorf("expected spec.ports to be non-empty for canary service %s/%s", service.Namespace, service.Name)
	}
	route.Spec.Port.TargetPort = service.Spec.Ports[0].TargetPort

	route.SetOwnerReferences(service.OwnerReferences)

	return route, nil
}

// checkRouteAdmitted returns true if a given route has been admitted
// by the default Ingress Controller.
func checkRouteAdmitted(route *routev1.Route) bool {
	for _, routeIngress := range route.Status.Ingress {
		if routeIngress.RouterName != manifests.DefaultIngressControllerName {
			continue
		}
		conditions := routeIngress.Conditions
		for _, cond := range conditions {
			if cond.Type == routev1.RouteAdmitted && cond.Status == corev1.ConditionTrue {
				return true
			}
		}
	}

	return false
}

//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	operatorv1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	routev1client "github.com/openshift/client-go/route/clientset/versioned"
	"github.com/openshift/library-go/test/library/metrics"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// TestRouteMetricsControllerOnlyRouteSelector creates an Ingress Controller with only RouteSelector and creates
// a Route with a label matching the RouteSelector. The Ingress Controller is modified to change the RouteSelector
// and then again the RouteSelector is changed back to the original value. The Route is then deleted and after that
// the Ingress Controller is deleted. For all the events (Create, Update, Delete), the metrics is verified with the
// expected metrics value.
func TestRouteMetricsControllerOnlyRouteSelector(t *testing.T) {
	t.Parallel()

	testRouteMetricsControllerLabelSelector(t, true, false)
}

// TestRouteMetricsControllerOnlyNamespaceSelector creates an Ingress Controller with only NamespaceSelector and creates
// a Route in a Namespace with a label matching the NamespaceSelector. The Ingress Controller is modified to change the
// NamespaceSelector and then again the NamespaceSelector is changed back to the original value. The Route is then
// deleted and after that the Ingress Controller is deleted. For all the events (Create, Update, Delete), the metrics is
// verified with the expected metrics value.
func TestRouteMetricsControllerOnlyNamespaceSelector(t *testing.T) {
	t.Parallel()

	testRouteMetricsControllerLabelSelector(t, false, true)
}

// TestRouteMetricsControllerRouteAndNamespaceSelector creates an Ingress Controller with both RouteSelector and
// NamespaceSelector and creates a Route with a label matching the RouteSelector and in a Namespace with a label
// matching the NamespaceSelector. The Ingress Controller is modified to change the RouteSelector and then again
// the RouteSelector is changed back to the original value. The Ingress Controller is then modified to change the
// NamespaceSelector and then again the NamespaceSelector is changed back to the original value. The Route is then
// deleted and after that the Ingress Controller is deleted. For all the events (Create, Update, Delete), the
// metrics is verified with the expected metrics value.
func TestRouteMetricsControllerRouteAndNamespaceSelector(t *testing.T) {
	t.Parallel()

	testRouteMetricsControllerLabelSelector(t, true, true)
}

// testRouteMetricsControllerLabelSelector is the common function for testing three different test cases. For testing
// the case where IC has only RouteLabelSelector then testRS should be set to true and testNS should be set to false.
// For testing the case where IC has only NamespaceLabelSelector then testRS should be set to false and testNS should
// be set to true. For testing the case where IC has both RouteLabelSelector and NamespaceLabelSelector then both testRS
// and testNS should be set to true. This function should not be called with both testRS and testNS set to false.
func testRouteMetricsControllerLabelSelector(t *testing.T, testRS, testNS bool) {

	// Create a new prometheus client for fetching metrics.
	kubeConfig, err := config.GetConfig()
	if err != nil {
		t.Fatalf("failed to get kube config: %s", err)
	}
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		t.Fatal(err)
	}
	routeClient, err := routev1client.NewForConfig(kubeConfig)
	if err != nil {
		t.Fatal(err)
	}
	prometheusClient, err := metrics.NewPrometheusClient(context.TODO(), kubeClient, routeClient)
	if err != nil {
		t.Fatal(err)
	}

	// Assign the correct values to the strings depending on the values of testRS and testNS.
	icNameStr := ""
	correctLabel := ""
	incorrectLabel := ""
	routeNameStr := ""
	if testRS && testNS {
		icNameStr = "ic-rs-ns-metrics-test"
		correctLabel = "rs-ns-foo"
		incorrectLabel = "rs-ns-bar"
		routeNameStr = "route-rs-ns-foo-label"
	} else if testNS {
		icNameStr = "ic-ns-metrics-test"
		correctLabel = "ns-foo"
		incorrectLabel = "ns-bar"
		routeNameStr = "route-ns-foo-label"
	} else if testRS {
		icNameStr = "ic-rs-metrics-test"
		correctLabel = "rs-foo"
		incorrectLabel = "rs-bar"
		routeNameStr = "route-rs-foo-label"
	} else {
		t.Fatalf("either of testRS or testNS should be set to true. testRS: %t, testNS: %t", testRS, testNS)
	}

	// Create an Ingress Controller that can admit our Route.
	icName := types.NamespacedName{Namespace: operatorNamespace, Name: icNameStr}
	domain := icName.Name + "." + dnsConfig.Spec.BaseDomain
	ic := newPrivateController(icName, domain)

	// Add RouteSelector if testRS is set to true.
	if testRS {
		ic.Spec.RouteSelector = &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"label": correctLabel,
			},
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "expression",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{correctLabel},
				},
			},
		}
	}

	// Add NamespaceSelector if testNS is set to true.
	if testNS {
		ic.Spec.NamespaceSelector = &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"label": correctLabel,
			},
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      "expression",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{correctLabel},
				},
			},
		}
	}

	if err := kclient.Create(context.TODO(), ic); err != nil {
		t.Fatalf("failed to create ingresscontroller: %v", err)
	}

	// Cleanup step - delete the Ingress Controller.
	defer assertIngressControllerDeleted(t, kclient, ic)

	// Wait for metrics to be added and set to 0.
	if err := waitForRouteMetricsAddorUpdate(t, prometheusClient, ic.Name, 0); err != nil {
		t.Fatalf("failed to fetch expected metrics: %v", err)
	}

	// Create a new namespace for the Route.
	nsName := types.NamespacedName{Name: names.SimpleNameGenerator.GenerateName("test-e2e-metrics-")}
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName.Name,
		},
	}

	// Add Label to the Namespace if testRS is set to true.
	if testNS {
		ns.Labels = map[string]string{
			"label":      correctLabel,
			"expression": correctLabel,
		}
	}
	if err := kclient.Create(context.TODO(), ns); err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	// Cleanup step - delete the Namespace.
	defer func() {
		if err := kclient.Delete(context.TODO(), ns); err != nil {
			t.Fatalf("failed to delete test namespace %s: %v", ns.Name, err)
		}
	}()

	// Create a Route to be immediately admitted by this Ingress Controller.
	routeFooLabelName := types.NamespacedName{Namespace: ns.Name, Name: routeNameStr}
	routeFooLabel := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: routeFooLabelName.Namespace,
			Name:      routeFooLabelName.Name,
			Labels: map[string]string{
				"label":      correctLabel,
				"expression": correctLabel,
			},
		},
		Spec: routev1.RouteSpec{
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: "foo",
			},
		},
	}

	if err := kclient.Create(context.TODO(), routeFooLabel); err != nil {
		t.Fatalf("failed to create route: %v", err)
	}

	// Cleanup step - delete the Route.
	defer func() {
		if err := kclient.Delete(context.TODO(), routeFooLabel); err != nil {
			if apierrors.IsNotFound(err) {
				return
			} else {
				t.Fatalf("failed to delete route %s: %v", routeFooLabelName, err)
			}
		}
	}()

	// Wait for metrics to be updated to 1 as the Route will get admitted by the IC.
	if err := waitForRouteMetricsAddorUpdate(t, prometheusClient, ic.Name, 1); err != nil {
		t.Fatalf("failed to fetch expected metrics: %v", err)
	}

	if testNS {
		// Update the NamespaceSelector of the Ingress Controller so that the Route gets un-admitted.
		if err := updateIngressControllerSpecWithRetryOnConflict(t, icName, 5*time.Minute, func(ic *operatorv1.IngressControllerSpec) {
			ic.NamespaceSelector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": incorrectLabel,
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "expression",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{incorrectLabel},
					},
				},
			}
		}); err != nil {
			t.Fatalf("failed to update ingresscontroller %s: %v", icName, err)
		}

		// Wait for metrics to be updated to 0 as the Route will get un-admitted by the IC.
		if err := waitForRouteMetricsAddorUpdate(t, prometheusClient, ic.Name, 0); err != nil {
			t.Fatalf("failed to fetch expected metrics: %v", err)
		}

		// Update the NamespaceSelector of the Ingress Controller so that the Route gets admitted again.
		if err := updateIngressControllerSpecWithRetryOnConflict(t, icName, 5*time.Minute, func(ic *operatorv1.IngressControllerSpec) {
			ic.NamespaceSelector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": correctLabel,
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "expression",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{correctLabel},
					},
				},
			}
		}); err != nil {
			t.Fatalf("failed to update ingresscontroller %s: %v", icName, err)
		}

		// Wait for metrics to be updated to 1 as the Route will get admitted by the IC again.
		if err := waitForRouteMetricsAddorUpdate(t, prometheusClient, ic.Name, 1); err != nil {
			t.Fatalf("failed to fetch expected metrics: %v", err)
		}

		// Update the label of the namespace so that Route gets un-admitted from the Ingress Controller.
		if err := updateNamespaceLabelsWithRetryOnConflict(t, nsName, 5*time.Minute, func(labels map[string]string) {
			labels["label"] = incorrectLabel
			labels["expression"] = incorrectLabel
		}); err != nil {
			t.Fatalf("failed to update namespace %s: %v", routeFooLabelName, err)
		}

		// Wait for metrics to be updated to 0 as the Route will get un-admitted by the IC.
		if err := waitForRouteMetricsAddorUpdate(t, prometheusClient, ic.Name, 0); err != nil {
			t.Fatalf("failed to fetch expected metrics: %v", err)
		}

		// Update the label of the namespace so that Route gets admitted from the Ingress Controller.
		if err := updateNamespaceLabelsWithRetryOnConflict(t, nsName, 5*time.Minute, func(labels map[string]string) {
			labels["label"] = correctLabel
			labels["expression"] = correctLabel
		}); err != nil {
			t.Fatalf("failed to update namespace %s: %v", routeFooLabelName, err)
		}

		// Wait for metrics to be updated to 1 as the Route will get admitted by the IC again.
		if err := waitForRouteMetricsAddorUpdate(t, prometheusClient, ic.Name, 1); err != nil {
			t.Fatalf("failed to fetch expected metrics: %v", err)
		}
	}

	if testRS {
		// Update the RouteSelector of the Ingress Controller so that the Route gets un-admitted again.
		if err := updateIngressControllerSpecWithRetryOnConflict(t, icName, 5*time.Minute, func(ic *operatorv1.IngressControllerSpec) {
			ic.RouteSelector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": incorrectLabel,
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "expression",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{incorrectLabel},
					},
				},
			}
		}); err != nil {
			t.Fatalf("failed to update ingresscontroller %s: %v", icName, err)
		}

		// Wait for metrics to be updated to 0 as the Route will get un-admitted by the IC again.
		if err := waitForRouteMetricsAddorUpdate(t, prometheusClient, ic.Name, 0); err != nil {
			t.Fatalf("failed to fetch expected metrics: %v", err)
		}

		// Update the RouteSelector of the Ingress Controller so that the Route gets admitted again.
		if err := updateIngressControllerSpecWithRetryOnConflict(t, icName, 5*time.Minute, func(ic *operatorv1.IngressControllerSpec) {
			ic.RouteSelector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": correctLabel,
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "expression",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{correctLabel},
					},
				},
			}
		}); err != nil {
			t.Fatalf("failed to update ingresscontroller %s: %v", icName, err)
		}

		// Wait for metrics to be updated to 1 as the Route will get admitted by the IC again.
		if err := waitForRouteMetricsAddorUpdate(t, prometheusClient, ic.Name, 1); err != nil {
			t.Fatalf("failed to fetch expected metrics: %v", err)
		}

		// Update the label of the route so that it gets un-admitted from the Ingress Controller.
		if err := updateRouteLabelsWithRetryOnConflict(t, routeFooLabelName, 5*time.Minute, func(labels map[string]string) {
			labels["label"] = incorrectLabel
			labels["expression"] = incorrectLabel
		}); err != nil {
			t.Fatalf("failed to update route %s: %v", routeFooLabelName, err)
		}

		// Wait for metrics to be updated to 0 as the Route will get un-admitted by the IC.
		if err := waitForRouteMetricsAddorUpdate(t, prometheusClient, ic.Name, 0); err != nil {
			t.Fatalf("failed to fetch expected metrics: %v", err)
		}

		// Update the label of the route so that it gets admitted by the Ingress Controller.
		if err := updateRouteLabelsWithRetryOnConflict(t, routeFooLabelName, 5*time.Minute, func(labels map[string]string) {
			labels["label"] = correctLabel
			labels["expression"] = correctLabel
		}); err != nil {
			t.Fatalf("failed to update route %s: %v", routeFooLabelName, err)
		}

		// Wait for metrics to be updated to 1 as the Route will get admitted by the IC again.
		if err := waitForRouteMetricsAddorUpdate(t, prometheusClient, ic.Name, 1); err != nil {
			t.Fatalf("failed to fetch expected metrics: %v", err)
		}
	}

	// Delete the Route routeFooLabel.
	func() {
		if err := kclient.Delete(context.TODO(), routeFooLabel); err != nil {
			if apierrors.IsNotFound(err) {
				return
			} else {
				t.Fatalf("failed to delete route %s: %v", routeFooLabelName, err)
			}
		}
	}()

	// Wait for metrics to be updated to zero as the admitted Route is deleted.
	if err := waitForRouteMetricsAddorUpdate(t, prometheusClient, ic.Name, 0); err != nil {
		t.Fatalf("failed to fetch expected metrics: %v", err)
	}

	// Delete the IC.
	assertIngressControllerDeleted(t, kclient, ic)

	// Wait for metrics corresponding to the IC to be deleted.
	if err := waitForRouteMetricsDelete(t, prometheusClient, ic.Name); err != nil {
		t.Fatalf("failed to fetch expected metrics: %v", err)
	}
}

// waitForRouteMetricsAddorUpdate waits for the metrics for the corresponding shard to be added or updated to the expected value.
func waitForRouteMetricsAddorUpdate(t *testing.T, prometheusClient prometheusv1.API, shardName string, value int) error {
	t.Logf("waiting for route_metrics_controller_routes_per_shard{shard_name=%s} to become %d", shardName, value)
	if err := wait.PollImmediate(1*time.Second, 2*time.Minute, func() (bool, error) {
		result, _, err := prometheusClient.Query(context.TODO(), fmt.Sprintf(`route_metrics_controller_routes_per_shard{shard_name="%s"}`, shardName), time.Now())
		if err != nil {
			t.Logf("failed to fetch metrics: %v", err)
			return false, nil
		}

		// Check if fetched metrics is of Vector type.
		vec, ok := result.(model.Vector)
		if !ok {
			return ok, nil
		}

		// Check if length of returned metric Vector is zero.
		if len(vec) == 0 {
			t.Logf("metric vector is empty...retrying")
			return false, nil
		}

		// Check if metrics is updated.
		if !vec[0].Value.Equal(model.SampleValue(float64(value))) {
			t.Logf("expected metric value %d, but got %d...retrying", value, int(vec[0].Value))
			return false, nil
		}

		return true, nil
	}); err != nil {
		return fmt.Errorf("error waiting for route metrics: %w", err)
	}
	return nil
}

// waitForRouteMetricsDelete waits for the metrics for the corresponding shard to be deleted.
func waitForRouteMetricsDelete(t *testing.T, prometheusClient prometheusv1.API, shardName string) error {
	if err := wait.PollImmediate(1*time.Second, 2*time.Minute, func() (bool, error) {
		result, _, err := prometheusClient.Query(context.TODO(), fmt.Sprintf(`route_metrics_controller_routes_per_shard{shard_name="%s"}`, shardName), time.Now())
		if err != nil {
			t.Logf("failed to fetch metrics: %v", err)
			return false, nil
		}

		// Check if fetched metrics is of Vector type.
		vec, ok := result.(model.Vector)
		if !ok {
			return ok, nil
		}

		// Check if metrics is deleted.
		if len(vec) != 0 {
			return false, nil
		}

		return true, nil
	}); err != nil {
		return fmt.Errorf("error waiting for route metrics: %w", err)
	}
	return nil
}

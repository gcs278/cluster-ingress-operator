//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/api/features"
	machinev1 "github.com/openshift/api/machine/v1beta1"
	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/cluster-ingress-operator/pkg/operator/controller"
	"github.com/openshift/cluster-ingress-operator/pkg/operator/controller/ingress"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apimachinery/pkg/util/wait"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	awsLBSubnetAnnotation = "service.beta.kubernetes.io/aws-load-balancer-subnets"
)

// TestAWSLBSubnets creates an IngressController with various subnets in AWS.
// The test verifies the provisioning of the LB-type Service, confirms ingress connectivity, and
// confirms that the service.beta.kubernetes.io/aws-load-balancer-subnets as well as the IngressController status is
// correctly configured.
func TestAWSLBSubnets(t *testing.T) {
	t.Parallel()

	if infraConfig.Status.PlatformStatus == nil {
		t.Skip("test skipped on nil platform")
	}
	if infraConfig.Status.PlatformStatus.Type != configv1.AWSPlatformType {
		t.Skipf("test skipped on platform %q", infraConfig.Status.PlatformStatus.Type)
	}
	if enabled, err := isFeatureGateEnabled(features.FeatureGateIngressControllerLBSubnetsAWS); err != nil {
		t.Fatalf("failed to get feature gate: %v", err)
	} else if !enabled {
		t.Skipf("test skipped because %q feature gate is not enabled", features.FeatureGateIngressControllerLBSubnetsAWS)
	}

	// First, let's get the list of public subnets to use for the LB.
	publicSubnets, _, err := getClusterSubnets()
	if err != nil {
		t.Fatalf("failed to get cluster subnets: %v", err)
	}
	t.Logf("discovered the following public subnets: %q", publicSubnets.Names)

	// Next, create the IngressController using a CLB with the public subnets we discovered.
	icName := types.NamespacedName{Namespace: operatorNamespace, Name: "aws-subnets"}
	t.Logf("creating ingresscontroller %q using CLB with public subnets", icName.Name)
	domain := icName.Name + "." + dnsConfig.Spec.BaseDomain
	ic := newLoadBalancerController(icName, domain)
	ic.Spec.EndpointPublishingStrategy.LoadBalancer = &operatorv1.LoadBalancerStrategy{
		Scope:               operatorv1.ExternalLoadBalancer,
		DNSManagementPolicy: operatorv1.ManagedLoadBalancerDNS,
		ProviderParameters: &operatorv1.ProviderLoadBalancerParameters{
			Type: operatorv1.AWSLoadBalancerProvider,
			AWS: &operatorv1.AWSLoadBalancerParameters{
				Type: operatorv1.AWSClassicLoadBalancer,
				ClassicLoadBalancerParameters: &operatorv1.AWSClassicLoadBalancerParameters{
					Subnets: publicSubnets,
				},
			},
		},
	}
	if err = kclient.Create(context.Background(), ic); err != nil {
		t.Fatalf("expected ingresscontroller creation failed: %v", err)
	}
	t.Cleanup(func() { assertIngressControllerDeleted(t, kclient, ic) })

	// Wait for the load balancer and DNS to be ready.
	if err = waitForIngressControllerCondition(t, kclient, 10*time.Minute, icName, availableConditionsForIngressControllerWithLoadBalancer...); err != nil {
		t.Fatalf("failed to observe expected conditions: %v", err)
	}

	// Ensure the expected subnet annotation is on the service.
	if err = waitForLBSubnetAnnotation(t, ic, publicSubnets); err != nil {
		t.Fatalf("failed to wait for expected subnet annotation on service: %v", err)
	}

	// Verify the subnets status field is configured to what we expect.
	if err = verifyIngressControllerSubnetStatus(t, icName); err != nil {
		t.Fatalf("failed to verify ingresscontroller subnet status: %v", err)
	}

	// Verify we can reach the CLB with the provided public subnets.
	externalTestPodName := types.NamespacedName{Name: icName.Name + "-external-verify", Namespace: icName.Namespace}
	testHostname := "apps." + ic.Spec.Domain
	t.Logf("verifying external connectivity for ingresscontroller %q using an CLB with specified public subnets", ic.Name)
	verifyExternalIngressController(t, externalTestPodName, testHostname, testHostname)

	// Now, update the IngressController to use invalid (non-existent) subnets.
	t.Logf("updating ingresscontroller %q to use invalid subnets while", ic.Name)
	invalidSubnets := &operatorv1.AWSSubnets{
		Names: []operatorv1.AWSSubnetName{"subnetfoo", "subnetbar"},
	}
	if err = updateIngressControllerWithRetryOnConflict(t, icName, 5*time.Minute, func(ic *operatorv1.IngressController) {
		ic.Spec.EndpointPublishingStrategy.LoadBalancer.ProviderParameters = &operatorv1.ProviderLoadBalancerParameters{
			Type: operatorv1.AWSLoadBalancerProvider,
			AWS: &operatorv1.AWSLoadBalancerParameters{
				Type: operatorv1.AWSClassicLoadBalancer,
				ClassicLoadBalancerParameters: &operatorv1.AWSClassicLoadBalancerParameters{
					Subnets: invalidSubnets,
				},
			},
		}
	}); err != nil {
		t.Fatalf("failed to update ingresscontroller: %v", err)
	}

	// Wait for the LoadBalancerProgressing to become True, which represents the message telling
	// the cluster admin to delete the service to effectuate the subnets.
	loadBalancerProgressingTrue := operatorv1.OperatorCondition{
		Type:   ingress.IngressControllerLoadBalancerProgressingConditionType,
		Status: operatorv1.ConditionTrue,
	}
	if err = waitForIngressControllerCondition(t, kclient, 5*time.Minute, icName, loadBalancerProgressingTrue); err != nil {
		t.Fatalf("failed to observe expected conditions: %v", err)
	}

	t.Logf("recreating the service to effectuate the subnets: %s/%s", controller.LoadBalancerServiceName(ic).Namespace, controller.LoadBalancerServiceName(ic).Namespace)
	if err = recreateIngressControllerService(t, ic); err != nil {
		t.Fatalf("failed to delete and recreate service: %v", err)
	}

	// Ensure the service's load-balancer status changes, and verify we get the expected subnet annotation on the service.
	if err = waitForLBSubnetAnnotation(t, ic, invalidSubnets); err != nil {
		t.Fatalf("failed to wait for expected subnet annotation on service: %v", err)
	}

	// Verify the subnets status field is configured to what we expect.
	if err = verifyIngressControllerSubnetStatus(t, icName); err != nil {
		t.Fatalf("failed to verify ingresscontroller subnet status: %v", err)
	}

	// Since the subnets are invalid, the load balancer will fail to provision and set LoadBalancerReady=False.
	loadBalancerReadyFalse := operatorv1.OperatorCondition{
		Type:   operatorv1.LoadBalancerReadyIngressConditionType,
		Status: operatorv1.ConditionFalse,
	}
	if err = waitForIngressControllerCondition(t, kclient, 5*time.Minute, icName, loadBalancerReadyFalse); err != nil {
		t.Fatalf("failed to observe expected conditions: %v", err)
	}

	// Now, update the IngressController to change to use a NLB and the public subnets again.
	t.Logf("updating ingresscontroller %q to use an NLB and public subnets", ic.Name)
	if err = updateIngressControllerWithRetryOnConflict(t, icName, 5*time.Minute, func(ic *operatorv1.IngressController) {
		ic.Spec.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS.Type = operatorv1.AWSNetworkLoadBalancer
		ic.Spec.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS.NetworkLoadBalancerParameters = &operatorv1.AWSNetworkLoadBalancerParameters{
			Subnets: publicSubnets,
		}
	}); err != nil {
		t.Fatalf("failed to update ingresscontroller: %v", err)
	}

	// When changing load balancer type which results in different subnets, the subnets are effectuated immediately.
	// There's no need to delete the service.
	if err = waitForLBSubnetAnnotation(t, ic, publicSubnets); err != nil {
		t.Fatalf("failed to wait for expected subnet annotation on service: %v", err)
	}

	// Expect the load balancer to provision successfully with the new subnets.
	if err = waitForIngressControllerCondition(t, kclient, 10*time.Minute, icName, availableConditionsForIngressControllerWithLoadBalancer...); err != nil {
		t.Fatalf("failed to observe expected conditions: %v", err)
	}

	// Verify the subnets status field is configured to what we expect.
	if err = verifyIngressControllerSubnetStatus(t, icName); err != nil {
		t.Fatalf("failed to verify ingresscontroller subnet status: %v", err)
	}

	// Verify we can still reach the NLB with the provided public subnets.
	t.Logf("verifying external connectivity for ingresscontroller %q using an NLB with specified public subnets", ic.Name)
	verifyExternalIngressController(t, externalTestPodName, testHostname, testHostname)

	// Now, update the IngressController to not specify subnets, but let's use the
	// auto-delete-load-balancer annotation, so we don't have to manually delete the service.
	t.Logf("updating ingresscontroller %q to remove the subnets while using the auto-delete-load-balancer annotation", ic.Name)
	if err = updateIngressControllerWithRetryOnConflict(t, icName, 5*time.Minute, func(ic *operatorv1.IngressController) {
		if ic.Annotations == nil {
			ic.Annotations = map[string]string{}
		}
		ic.Annotations["ingress.operator.openshift.io/auto-delete-load-balancer"] = ""
		// Remove subnets by not specifying them.
		ic.Spec.EndpointPublishingStrategy.LoadBalancer.ProviderParameters = &operatorv1.ProviderLoadBalancerParameters{
			Type: operatorv1.AWSLoadBalancerProvider,
		}
	}); err != nil {
		t.Fatalf("failed to update ingresscontroller: %v", err)
	}

	// Verify the subnet annotation is removed on the service.
	if err = waitForLBSubnetAnnotation(t, ic, nil); err != nil {
		t.Fatalf("failed to wait for expected subnet annotation on service: %v", err)
	}

	// Expect the load balancer to provision successfully with the subnets removed.
	if err = waitForIngressControllerCondition(t, kclient, 10*time.Minute, icName, availableConditionsForIngressControllerWithLoadBalancer...); err != nil {
		t.Fatalf("failed to observe expected conditions: %v", err)
	}

	// Verify the subnets status field is configured to what we expect.
	if err = verifyIngressControllerSubnetStatus(t, icName); err != nil {
		t.Fatalf("failed to verify ingresscontroller subnet status: %v", err)
	}
}

// waitForLBSubnetAnnotation waits for the provided subnets to appear on the LoadBalancer-type service for the
// IngressController. It will return an error if it fails to observe the updated subnet annotation.
func waitForLBSubnetAnnotation(t *testing.T, ic *operatorv1.IngressController, subnets *operatorv1.AWSSubnets) error {
	t.Helper()
	lbService := &corev1.Service{}
	expectedSubnetAnnotation := ingress.JoinAWSSubnets(subnets, ",")
	t.Logf("waiting for %q service to have subnet annotation of %q", controller.LoadBalancerServiceName(ic), expectedSubnetAnnotation)
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 5*time.Minute, false, func(ctx context.Context) (bool, error) {
		if err := kclient.Get(ctx, controller.LoadBalancerServiceName(ic), lbService); err != nil {
			t.Logf("failed to get %q service: %v, retrying ...", controller.LoadBalancerServiceName(ic), err)
			return false, nil
		}
		val, ok := lbService.Annotations[awsLBSubnetAnnotation]
		// Handle the case where subnets are expected to be nil (annotation should not exist).
		if subnets == nil {
			if ok {
				t.Logf("expected %q annotation to be removed got %q, retrying...", awsLBSubnetAnnotation, val)
				return false, nil
			}
			return true, nil
		}
		// Handle the case where subnets are expected (annotation should exist and match).
		if !ok || val != expectedSubnetAnnotation {
			t.Logf("expected %q annotation %q got %q, retrying...", awsLBSubnetAnnotation, expectedSubnetAnnotation, val)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("error updating the %q service: %v", controller.LoadBalancerServiceName(ic), err)
	}
	return nil
}

// verifyIngressControllerSubnetStatus verifies that the IngressController subnet status fields are
// equal to the appropriate subnet spec fields when an IngressController is in a LoadBalancerProgressing=False state.
func verifyIngressControllerSubnetStatus(t *testing.T, icName types.NamespacedName) error {
	t.Helper()
	t.Logf("verifying ingresscontroller %q subnet status field match the appropriate subnet spec field", icName.Name)
	// First, ensure the LoadBalancerProgressing is False. If LoadBalancerProgressing is True
	// (due to a non-effectuated subnet update), our subnet status checks will surely fail.
	loadBalancerProgressingFalse := operatorv1.OperatorCondition{
		Type:   ingress.IngressControllerLoadBalancerProgressingConditionType,
		Status: operatorv1.ConditionFalse,
	}
	if err := waitForIngressControllerCondition(t, kclient, 5*time.Minute, icName, loadBalancerProgressingFalse); err != nil {
		return fmt.Errorf("failed to observe expected conditions: %v", err)
	}
	// Poll until the subnet status is what we expect.
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 3*time.Minute, false, func(ctx context.Context) (bool, error) {
		ic := &operatorv1.IngressController{}
		if err := kclient.Get(context.Background(), icName, ic); err != nil {
			t.Logf("failed to get ingresscontroller: %v, retrying...", err)
			return false, nil
		}
		// Verify the subnets status field is configured to what we expect based on the load balancer type.
		var subnetSpec, subnetStatus *operatorv1.AWSSubnets
		switch ic.Status.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS.Type {
		case operatorv1.AWSClassicLoadBalancer:
			if classicLoadBalancerParametersSpecExist(ic) {
				subnetSpec = ic.Spec.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS.ClassicLoadBalancerParameters.Subnets
			}
			if classicLoadBalancerParametersStatusExists(ic) {
				subnetStatus = ic.Status.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS.ClassicLoadBalancerParameters.Subnets
			}
			if networkLoadBalancerParametersStatusExist(ic) {
				t.Logf("expected subnets in NetworkLoadBalancerParameters to be nil when LB type is Classic, got: %v, retrying...", ic.Status.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS.NetworkLoadBalancerParameters.Subnets)
				return false, nil
			}
		case operatorv1.AWSNetworkLoadBalancer:
			if networkLoadBalancerParametersSpecExists(ic) {
				subnetSpec = ic.Spec.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS.NetworkLoadBalancerParameters.Subnets
			}
			if networkLoadBalancerParametersStatusExist(ic) {
				subnetStatus = ic.Status.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS.NetworkLoadBalancerParameters.Subnets
			}
			if classicLoadBalancerParametersStatusExists(ic) {
				t.Logf("expected subnets in ClassicLoadBalancerParameters to be nil when LB type is NLB, got: %v, retrying...", ic.Status.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS.ClassicLoadBalancerParameters.Subnets)
				return false, nil
			}
		}

		// Check if the subnetSpec and subnetStatus are equal.
		if !reflect.DeepEqual(subnetSpec, subnetStatus) {
			t.Logf("expected ingresscontroller subnet status to be %q, got %q, retrying...", subnetSpec, subnetStatus)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("error verifying subnet status for IngressController %q: %v", icName.Name, err)
	}
	return nil
}

// recreateIngressControllerService deletes the service for the IngressController and waits for it
// to be recreated by the Ingress Operator.
func recreateIngressControllerService(t *testing.T, ic *operatorv1.IngressController) error {
	lbService := &corev1.Service{}
	serviceName := controller.LoadBalancerServiceName(ic)
	if err := kclient.Get(context.Background(), serviceName, lbService); err != nil {
		return fmt.Errorf("failed to get service: %v", err)
	}
	oldUid := lbService.UID

	if err := kclient.Delete(context.Background(), lbService); err != nil {
		return fmt.Errorf("failed to delete service: %v", err)
	}

	// Wait for the service to be recreated by Ingress Operator.
	err := wait.PollUntilContextTimeout(context.Background(), 10*time.Second, 3*time.Minute, false, func(ctx context.Context) (bool, error) {
		if err := kclient.Get(context.Background(), controller.LoadBalancerServiceName(ic), lbService); err != nil {
			t.Logf("failed to get service %s/%s: %v, retrying ...", serviceName.Namespace, serviceName.Namespace, err)
			return false, nil
		}
		if oldUid == lbService.UID {
			t.Logf("service %s/%s UID has not changed yet, retrying...", serviceName.Namespace, serviceName.Namespace)
			return false, nil
		}
		return true, nil
	})
	return err
}

// classicLoadBalancerParametersSpecExist checks if ClassicLoadBalancerParameters exist in the IngressController spec.
func classicLoadBalancerParametersSpecExist(ic *operatorv1.IngressController) bool {
	return ic.Spec.EndpointPublishingStrategy != nil &&
		ic.Spec.EndpointPublishingStrategy.LoadBalancer != nil &&
		ic.Spec.EndpointPublishingStrategy.LoadBalancer.ProviderParameters != nil &&
		ic.Spec.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS != nil &&
		ic.Spec.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS.ClassicLoadBalancerParameters != nil
}

// networkLoadBalancerParametersSpecExists checks if NetworkLoadBalancerParameters exist in the IngressController spec.
func networkLoadBalancerParametersSpecExists(ic *operatorv1.IngressController) bool {
	return ic.Spec.EndpointPublishingStrategy != nil &&
		ic.Spec.EndpointPublishingStrategy.LoadBalancer != nil &&
		ic.Spec.EndpointPublishingStrategy.LoadBalancer.ProviderParameters != nil &&
		ic.Spec.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS != nil &&
		ic.Spec.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS.NetworkLoadBalancerParameters != nil
}

// networkLoadBalancerParametersStatusExist checks if NetworkLoadBalancerParameters exist in the IngressController
// status.
func networkLoadBalancerParametersStatusExist(ic *operatorv1.IngressController) bool {
	return ic.Status.EndpointPublishingStrategy != nil &&
		ic.Status.EndpointPublishingStrategy.LoadBalancer != nil &&
		ic.Status.EndpointPublishingStrategy.LoadBalancer.ProviderParameters != nil &&
		ic.Status.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS != nil &&
		ic.Status.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS.NetworkLoadBalancerParameters != nil &&
		ic.Status.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS.NetworkLoadBalancerParameters.Subnets != nil
}

// classicLoadBalancerParametersStatusExists checks if ClassicLoadBalancerParameters exist in the IngressController
// status.
func classicLoadBalancerParametersStatusExists(ic *operatorv1.IngressController) bool {
	return ic.Status.EndpointPublishingStrategy != nil &&
		ic.Status.EndpointPublishingStrategy.LoadBalancer != nil &&
		ic.Status.EndpointPublishingStrategy.LoadBalancer.ProviderParameters != nil &&
		ic.Status.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS != nil &&
		ic.Status.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS.ClassicLoadBalancerParameters != nil &&
		ic.Status.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS.ClassicLoadBalancerParameters.Subnets != nil
}

// getClusterSubnets populates public and private AWSSubnets objects with a list of
// the public and private cluster subnets names. Unfortunately these subnets aren't
// easily accessible in any K8S API objects. However, by using the installer-created subnet
// naming convention, we can generate the names of the public and private subnets.
func getClusterSubnets() (public *operatorv1.AWSSubnets, private *operatorv1.AWSSubnets, err error) {
	// Subnet Naming Convention: <clusterName>-subnet-[public|private]-<availabilityZone>
	machineSets := &machinev1.MachineSetList{}
	listOpts := []client.ListOption{
		client.InNamespace("openshift-machine-api"),
	}
	if err := kclient.List(context.Background(), machineSets, listOpts...); err != nil {
		return nil, nil, fmt.Errorf("failed to get machineSets: %w", err)
	}
	// The availability zones can be derived from the providerSpec on MachineSet object.
	publicSubnets := &operatorv1.AWSSubnets{}
	privateSubnets := &operatorv1.AWSSubnets{}
	for _, machineset := range machineSets.Items {
		providerSpec := &machinev1.AWSMachineProviderConfig{}
		if err := unmarshalInto(&machineset, providerSpec); err != nil {
			return nil, nil, fmt.Errorf("failure to unmarshal machineset %q provider spec: %w", machineset.Name, err)
		}

		if len(providerSpec.Placement.AvailabilityZone) == 0 {
			return nil, nil, fmt.Errorf("machineset %q availability zone is empty", machineset.Name)
		}
		// Build the public and private subnet name according to the installer naming convention.
		publicSubnet := infraConfig.Status.InfrastructureName + "-subnet-public-" + providerSpec.Placement.AvailabilityZone
		publicSubnets.Names = append(publicSubnets.Names, operatorv1.AWSSubnetName(publicSubnet))
		privateSubnet := infraConfig.Status.InfrastructureName + "-subnet-private-" + providerSpec.Placement.AvailabilityZone
		privateSubnets.Names = append(privateSubnets.Names, operatorv1.AWSSubnetName(privateSubnet))
	}
	return publicSubnets, privateSubnets, nil
}

// unmarshalInto unmarshals a MachineSet's ProviderSpec into the provided providerSpec parameter.
func unmarshalInto(m *machinev1.MachineSet, providerSpec interface{}) *field.Error {
	if m.Spec.Template.Spec.ProviderSpec.Value == nil {
		return field.Required(field.NewPath("providerSpec", "value"), "a value must be provided")
	}

	if err := json.Unmarshal(m.Spec.Template.Spec.ProviderSpec.Value.Raw, &providerSpec); err != nil {
		return field.Invalid(field.NewPath("providerSpec", "value"), providerSpec, err.Error())
	}
	return nil
}

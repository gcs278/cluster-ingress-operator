/*
This cmd line is intended to run the sail installer as if it was part of a Gateway class
reconciliation on CIO, but without needing a full OCP cluster and/or CIO deployed.

It must be called as following:
VERSIONS_YAML_FILE=versions.ossm.yaml go run cmd/sail-reconciler/main.go

The versions file MUST be passed as environment variable, otherwise sail installer won't be
able to provision the right manifests.

Additionally, one can pass extra infrastructure configurations to simulate behaviors
like different topologies, different proxies, etc

*/

package main

import (
	"context"

	"github.com/istio-ecosystem/sail-operator/chart"
	"github.com/istio-ecosystem/sail-operator/pkg/install"
	"github.com/istio-ecosystem/sail-operator/resources"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/cluster-ingress-operator/pkg/operator/controller/gatewayclass"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var (
	scheme = runtime.NewScheme()
)

func main() {
	ctx := context.Background()

	cfg := config.GetConfigOrDie()

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		panic(err)
	}

	kCache, err := cache.New(cfg, cache.Options{
		Scheme: scheme,
	})

	if err != nil {
		panic(err)
	}

	go func() {
		if err := kCache.Start(ctx); err != nil {
			panic(err)
		}
	}()

	kCache.WaitForCacheSync(ctx)

	installer, err := install.New(cfg, resources.FS, chart.CRDsFS)
	if err != nil {
		panic(err)
	}
	notifyCh, err := installer.Start(ctx)
	if err != nil {
		panic(err)
	}

	config := gatewayclass.StdConfig{
		Cfg: gatewayclass.Config{
			OperandNamespace:            "istio-system",
			GatewayAPIWithoutOLMEnabled: true,
		},
		Client:    k8sClient,
		Cache:     kCache,
		Installer: installer,
		InfraConfig: &configv1.Infrastructure{
			Spec: configv1.InfrastructureSpec{},
		},
	}

	if err := gatewayclass.Standalone(ctx, "v1.30-latest", config); err != nil {
		panic(err)
	}
	<-notifyCh
}

func init() {
	if err := apiextensions.AddToScheme(scheme); err != nil {
		panic(err)
	}

	if err := configv1.AddToScheme(scheme); err != nil {
		panic(err)
	}
}

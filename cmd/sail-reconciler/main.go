/*
Standalone sail-reconciler with TLS profile testing support.

Usage:
  VERSIONS_YAML_FILE=versions.ossm.yaml go run cmd/sail-reconciler/main.go [flags]

Flags:
  --istio-version    Istio version to install (default: v1.30-latest)
  --tls-profile      TLS profile type: Old, Intermediate, Modern (default: none)
  --tls-min-version  Minimum TLS version: VersionTLS10, VersionTLS11, VersionTLS12, VersionTLS13
  --tls-adherence    TLS adherence policy: NoOpinion, LegacyAdheringComponentsOnly, StrictAllComponents
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/istio-ecosystem/sail-operator/chart"
	"github.com/istio-ecosystem/sail-operator/pkg/install"
	"github.com/istio-ecosystem/sail-operator/resources"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/cluster-ingress-operator/pkg/operator/controller/gatewayclass"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var (
	scheme = runtime.NewScheme()
)

func main() {
	istioVersion := flag.String("istio-version", "v1.30-latest", "Istio version to install")
	tlsProfile := flag.String("tls-profile", "", "TLS profile type: Old, Intermediate, Modern")
	tlsMinVersion := flag.String("tls-min-version", "", "Minimum TLS version (e.g. VersionTLS12)")
	tlsAdherence := flag.String("tls-adherence", "", "TLS adherence policy")
	flag.Parse()

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

	// Build APIServer config with TLS settings
	apiServerConfig := buildAPIServerConfig(*tlsProfile, *tlsMinVersion, *tlsAdherence)
	if apiServerConfig.Spec.TLSSecurityProfile != nil {
		fmt.Printf("TLS Profile: %s\n", apiServerConfig.Spec.TLSSecurityProfile.Type)
		if apiServerConfig.Spec.TLSSecurityProfile.Type == configv1.TLSProfileCustomType {
			fmt.Printf("  MinTLSVersion: %s\n", apiServerConfig.Spec.TLSSecurityProfile.Custom.MinTLSVersion)
			fmt.Printf("  Ciphers: %v\n", apiServerConfig.Spec.TLSSecurityProfile.Custom.Ciphers)
		}
	}
	if apiServerConfig.Spec.TLSAdherence != "" {
		fmt.Printf("TLS Adherence: %s\n", apiServerConfig.Spec.TLSAdherence)
	}

	// Create the APIServer resource in the cluster so ensureIstio can read it
	if err := ensureAPIServerResource(ctx, k8sClient, apiServerConfig); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not create APIServer resource: %v\n", err)
		fmt.Fprintf(os.Stderr, "TLS config will use defaults\n")
	}

	stdConfig := gatewayclass.StdConfig{
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

	fmt.Printf("Installing Istio %s...\n", *istioVersion)
	if err := gatewayclass.Standalone(ctx, *istioVersion, stdConfig); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Waiting for reconciliation...")
	<-notifyCh
	fmt.Println("Done.")
}

func buildAPIServerConfig(profileType, minVersion, adherence string) *configv1.APIServer {
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
	}

	if adherence != "" {
		apiServer.Spec.TLSAdherence = configv1.TLSAdherencePolicy(adherence)
	}

	switch configv1.TLSProfileType(profileType) {
	case configv1.TLSProfileOldType:
		apiServer.Spec.TLSSecurityProfile = &configv1.TLSSecurityProfile{
			Type: configv1.TLSProfileOldType,
		}
	case configv1.TLSProfileIntermediateType:
		apiServer.Spec.TLSSecurityProfile = &configv1.TLSSecurityProfile{
			Type: configv1.TLSProfileIntermediateType,
		}
	case configv1.TLSProfileModernType:
		apiServer.Spec.TLSSecurityProfile = &configv1.TLSSecurityProfile{
			Type: configv1.TLSProfileModernType,
		}
	case configv1.TLSProfileCustomType:
		profile := configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
		if minVersion != "" {
			profile.MinTLSVersion = configv1.TLSProtocolVersion(minVersion)
		}
		apiServer.Spec.TLSSecurityProfile = &configv1.TLSSecurityProfile{
			Type: configv1.TLSProfileCustomType,
			Custom: &configv1.CustomTLSProfile{
				TLSProfileSpec: *profile,
			},
		}
	}

	return apiServer
}

func ensureAPIServerResource(ctx context.Context, cl client.Client, apiServer *configv1.APIServer) error {
	existing := &configv1.APIServer{}
	err := cl.Get(ctx, client.ObjectKeyFromObject(apiServer), existing)
	if err != nil {
		return cl.Create(ctx, apiServer)
	}
	existing.Spec = apiServer.Spec
	return cl.Update(ctx, existing)
}

func init() {
	if err := apiextensions.AddToScheme(scheme); err != nil {
		panic(err)
	}
	if err := configv1.AddToScheme(scheme); err != nil {
		panic(err)
	}
}

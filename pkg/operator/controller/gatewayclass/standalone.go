package gatewayclass

import (
	"context"

	configv1 "github.com/openshift/api/config/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	v1 "sigs.k8s.io/gateway-api/apis/v1"
)

type StdConfig struct {
	Cfg         Config
	Client      client.Client
	Cache       cache.Cache
	Installer   SailLibraryInstaller
	InfraConfig *configv1.Infrastructure
}

func Standalone(ctx context.Context, istioVersion string, config StdConfig) error {
	reconciler := &reconciler{
		config:        config.Cfg,
		client:        config.Client,
		cache:         config.Cache,
		sailInstaller: config.Installer,
	}

	if config.InfraConfig == nil {
		config.InfraConfig = &configv1.Infrastructure{
			Spec: configv1.InfrastructureSpec{},
		}
	}

	return reconciler.ensureIstio(ctx, istioVersion, []v1.GatewayClass{}, config.InfraConfig)
}

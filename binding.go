package aegis

import (
	"context"
	"fmt"
	"sort"
)

func BindResources(config Config, drivers *DriverRegistry, resources *ResourceManager) error {
	if drivers == nil {
		return newKernelError(CodeInvalidConfig, "driver registry cannot be nil", nil)
	}
	if resources == nil {
		return newKernelError(CodeInvalidConfig, "resource manager cannot be nil", nil)
	}

	for name, binding := range config.Resources.Storage {
		resource, info, err := resolveStorageBinding(name, name, binding, drivers)
		if err != nil {
			return err
		}
		if err := resources.RegisterStorageWithBinding(name, resource, info); err != nil {
			return err
		}
	}

	return nil
}

func resolveStorageBinding(resourceName, providerName string, binding StorageBinding, drivers *DriverRegistry) (StorageResource, StorageBindingInfo, error) {
	binding = binding.clone()
	if binding.Provider != "" {
		providerName = binding.Provider
	}

	if len(binding.Tenant) > 0 {
		return resolveTenantStorageBinding(resourceName, providerName, binding, drivers)
	}

	if binding.Driver == "layered" || len(binding.Layers) > 0 {
		return resolveLayeredStorageBinding(resourceName, providerName, binding, drivers)
	}

	if binding.Driver == "" {
		return nil, StorageBindingInfo{}, newKernelError(
			CodeInvalidConfig,
			fmt.Sprintf("storage resource %q must declare a driver", resourceName),
			nil,
		)
	}

	factory, ok := drivers.storageFactory(binding.Driver)
	if !ok {
		return nil, StorageBindingInfo{}, newKernelError(
			CodeDriverNotRegistered,
			fmt.Sprintf("storage driver %q is not registered", binding.Driver),
			nil,
		)
	}

	driver := factory()
	if driver == nil {
		return nil, StorageBindingInfo{}, newKernelError(
			CodeBootstrapFailed,
			fmt.Sprintf("storage driver %q factory returned nil", binding.Driver),
			nil,
		)
	}

	if err := driver.Init(binding.Config); err != nil {
		return nil, StorageBindingInfo{}, newKernelError(
			CodeBootstrapFailed,
			fmt.Sprintf("failed to initialize storage provider %q", providerName),
			err,
		)
	}

	observed := newObservedStorageDriver(providerName, binding.Driver, driver)
	if err := observed.HealthCheck(context.Background()); err != nil {
		return nil, StorageBindingInfo{}, newKernelError(
			CodeDriverUnhealthy,
			fmt.Sprintf("storage provider %q failed health check", providerName),
			err,
		)
	}

	info := StorageBindingInfo{
		Name:         resourceName,
		Driver:       binding.Driver,
		Provider:     providerName,
		HotSwappable: binding.HotSwappable,
		Providers: []StorageProviderInfo{
			{
				Name:   providerName,
				Driver: binding.Driver,
				Config: cloneMap(binding.Config),
				Stats:  observed.Stats(),
			},
		},
	}
	return observed, info, nil
}

func resolveLayeredStorageBinding(resourceName, providerName string, binding StorageBinding, drivers *DriverRegistry) (StorageResource, StorageBindingInfo, error) {
	if len(binding.Layers) == 0 {
		return nil, StorageBindingInfo{}, newKernelError(
			CodeInvalidConfig,
			fmt.Sprintf("layered storage resource %q requires at least one layer", resourceName),
			nil,
		)
	}

	layers := make([]StorageResource, 0, len(binding.Layers))
	providers := make([]StorageProviderInfo, 0)
	for index, layerBinding := range binding.Layers {
		layerProviderName := layerBinding.ProviderName(fmt.Sprintf("%s.layer.%d", providerName, index))
		resource, info, err := resolveStorageBinding(resourceName, layerProviderName, layerBinding, drivers)
		if err != nil {
			return nil, StorageBindingInfo{}, err
		}
		layers = append(layers, resource)
		providers = mergeStorageProviderInfo(providers, info.Providers)
	}

	info := StorageBindingInfo{
		Name:         resourceName,
		Driver:       "layered",
		Provider:     providerName,
		Providers:    providers,
		Layered:      true,
		HotSwappable: binding.HotSwappable,
	}
	return layeredStorageResource{layers: layers}, info, nil
}

func resolveTenantStorageBinding(resourceName, providerName string, binding StorageBinding, drivers *DriverRegistry) (StorageResource, StorageBindingInfo, error) {
	tenants := make(map[string]StorageResource, len(binding.Tenant))
	providers := make([]StorageProviderInfo, 0)
	tenantNames := make([]string, 0, len(binding.Tenant))

	var defaultResource StorageResource
	if binding.Driver != "" || len(binding.Layers) > 0 {
		resource, info, err := resolveStorageBinding(resourceName, providerName, StorageBinding{
			Driver:       binding.Driver,
			Provider:     binding.Provider,
			Config:       binding.Config,
			Layers:       binding.Layers,
			HotSwappable: binding.HotSwappable,
		}, drivers)
		if err != nil {
			return nil, StorageBindingInfo{}, err
		}
		defaultResource = resource
		providers = mergeStorageProviderInfo(providers, info.Providers)
	}

	for tenantID, tenantBinding := range binding.Tenant {
		resource, info, err := resolveStorageBinding(resourceName, tenantBinding.ProviderName(providerName+"."+tenantID), tenantBinding, drivers)
		if err != nil {
			return nil, StorageBindingInfo{}, err
		}
		tenants[tenantID] = resource
		tenantNames = append(tenantNames, tenantID)
		providers = mergeStorageProviderInfo(providers, info.Providers)
	}
	sort.Strings(tenantNames)

	info := StorageBindingInfo{
		Name:         resourceName,
		Driver:       binding.Driver,
		Provider:     providerName,
		Providers:    providers,
		MultiTenant:  true,
		HotSwappable: binding.HotSwappable,
		Tenants:      tenantNames,
	}
	if info.Driver == "" && len(binding.Layers) > 0 {
		info.Driver = "layered"
	}
	if info.Driver == "" {
		info.Driver = "tenant"
	}

	return tenantStorageResource{
		name:          resourceName,
		defaultTenant: defaultResource,
		tenants:       tenants,
	}, info, nil
}

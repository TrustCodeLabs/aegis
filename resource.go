package aegis

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

type ResourceResolver interface {
	Storage(name string) StorageResource
	Cache(name string) CacheResource
	SQL(name string) SQLResource
	External(name string) HTTPResource
}

type StorageResource interface {
	Write(ctx context.Context, path string, data []byte) error
	Read(ctx context.Context, path string) ([]byte, error)
	Delete(ctx context.Context, path string) error
}

type CacheResource interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte) error
	Delete(ctx context.Context, key string) error
}

type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

type Result interface {
	RowsAffected() (int64, error)
}

type SQLResource interface {
	Query(ctx context.Context, query string, args ...any) (Rows, error)
	Exec(ctx context.Context, query string, args ...any) (Result, error)
}

type HTTPResource interface {
	Do(req *http.Request) (*http.Response, error)
}

type DriverRegistry struct {
	storage map[string]StorageDriverFactory
}

func NewDriverRegistry() *DriverRegistry {
	return &DriverRegistry{
		storage: make(map[string]StorageDriverFactory),
	}
}

func (r *DriverRegistry) RegisterStorage(name string, factory StorageDriverFactory) error {
	if name == "" {
		return newKernelError(CodeInvalidConfig, "storage driver name cannot be empty", nil)
	}
	if factory == nil {
		return newKernelError(CodeInvalidConfig, fmt.Sprintf("storage driver %q has a nil factory", name), nil)
	}
	if _, exists := r.storage[name]; exists {
		return newKernelError(CodeDuplicateResource, fmt.Sprintf("storage driver %q is already registered", name), nil)
	}
	r.storage[name] = factory
	return nil
}

func (r *DriverRegistry) storageFactory(name string) (StorageDriverFactory, bool) {
	factory, ok := r.storage[name]
	return factory, ok
}

type ResourceManager struct {
	mu      sync.RWMutex
	storage map[string]storageRegistration
}

type StorageProviderInfo struct {
	Name   string
	Driver string
	Config map[string]any
	Stats  DriverStats
}

type StorageBindingInfo struct {
	Name         string
	Driver       string
	Provider     string
	Providers    []StorageProviderInfo
	Layered      bool
	MultiTenant  bool
	HotSwappable bool
	Tenants      []string
}

type storageRegistration struct {
	resource StorageResource
	binding  StorageBindingInfo
}

func NewResourceManager() *ResourceManager {
	return &ResourceManager{
		storage: make(map[string]storageRegistration),
	}
}

func (m *ResourceManager) RegisterStorage(name string, resource StorageResource) error {
	return m.RegisterStorageWithBinding(name, resource, StorageBindingInfo{
		Name:   name,
		Driver: "direct",
	})
}

func (m *ResourceManager) RegisterStorageWithBinding(name string, resource StorageResource, binding StorageBindingInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if name == "" {
		return newKernelError(CodeInvalidConfig, "storage resource name cannot be empty", nil)
	}
	if resource == nil {
		return newKernelError(CodeInvalidConfig, fmt.Sprintf("storage resource %q cannot be nil", name), nil)
	}
	if _, exists := m.storage[name]; exists {
		return newKernelError(CodeDuplicateResource, fmt.Sprintf("storage resource %q is already registered", name), nil)
	}
	binding.Name = name
	if binding.Provider == "" {
		binding.Provider = name
	}
	m.storage[name] = storageRegistration{
		resource: resource,
		binding:  cloneStorageBindingInfo(binding),
	}
	return nil
}

func (m *ResourceManager) SwapStorage(name string, resource StorageResource, binding StorageBindingInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	registration, exists := m.storage[name]
	if !exists {
		return newKernelError(CodeResourceNotFound, fmt.Sprintf("storage resource %q was not found", name), nil)
	}
	if !registration.binding.HotSwappable {
		return newKernelError(CodeHotSwapDenied, fmt.Sprintf("storage resource %q is not hot-swappable", name), nil)
	}

	binding.Name = name
	if binding.Provider == "" {
		binding.Provider = name
	}
	m.storage[name] = storageRegistration{
		resource: resource,
		binding:  cloneStorageBindingInfo(binding),
	}
	return nil
}

func (m *ResourceManager) LookupStorage(name string) (StorageResource, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	registration, ok := m.storage[name]
	if !ok {
		return nil, false
	}
	return registration.resource, true
}

func (m *ResourceManager) StorageBindingInfo(name string) (StorageBindingInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	registration, ok := m.storage[name]
	if !ok {
		return StorageBindingInfo{}, false
	}
	return cloneStorageBindingInfo(registration.binding), true
}

type executionResources struct {
	manager        *ResourceManager
	caps           *CapabilityManager
	exec           *ExecutionContext
	effects        EffectTracker
	declaredEffect map[string]EffectSpec
}

func newExecutionResources(manager *ResourceManager, caps *CapabilityManager, effects EffectTracker, descriptor OperationDescriptor) *executionResources {
	declared := make(map[string]EffectSpec, len(descriptor.Effects))
	for _, effect := range descriptor.Effects {
		declared[effect.Name] = effect
	}

	return &executionResources{
		manager:        manager,
		caps:           caps,
		effects:        effects,
		declaredEffect: declared,
	}
}

func (r *executionResources) bind(exec *ExecutionContext) {
	r.exec = exec
}

func (r *executionResources) Storage(name string) StorageResource {
	backend, ok := r.manager.LookupStorage(name)
	if !ok {
		return missingStorageResource{name: name}
	}
	return guardedStorageResource{
		name:    name,
		backend: backend,
		caps:    r.caps,
		exec:    r.exec,
		effects: r.effects,
		specFor: r.effectSpec,
	}
}

func (r *executionResources) Cache(name string) CacheResource {
	return notImplementedCacheResource{name: name}
}

func (r *executionResources) SQL(name string) SQLResource {
	return notImplementedSQLResource{name: name}
}

func (r *executionResources) External(name string) HTTPResource {
	return notImplementedHTTPResource{name: name}
}

func (r *executionResources) Commit() error {
	return nil
}

func (r *executionResources) effectSpec(name, kind string, requiredCap CapabilityRef, metadata map[string]any) EffectSpec {
	if spec, ok := r.declaredEffect[name]; ok {
		if spec.RequiredCap == "" {
			spec.RequiredCap = requiredCap
		}
		for key, value := range metadata {
			spec.Metadata[key] = value
		}
		return spec
	}

	return EffectSpec{
		Name:        name,
		Kind:        kind,
		RequiredCap: requiredCap,
		Critical:    strings.HasSuffix(kind, ".write") || strings.HasSuffix(kind, ".delete") || strings.HasPrefix(kind, "storage.write"),
		Metadata:    cloneMap(metadata),
		Declared:    false,
	}
}

type guardedStorageResource struct {
	name    string
	backend StorageResource
	caps    *CapabilityManager
	exec    *ExecutionContext
	effects EffectTracker
	specFor func(name string, kind string, requiredCap CapabilityRef, metadata map[string]any) EffectSpec
}

func (r guardedStorageResource) Write(ctx context.Context, path string, data []byte) error {
	requiredCap := CapabilityRef("storage.write:" + r.name)
	if err := r.caps.Check(*r.exec, requiredCap); err != nil {
		return err
	}

	spec := r.specFor("storage.write."+r.name, "storage.write", requiredCap, map[string]any{
		"resource": r.name,
		"path":     path,
		"bytes":    len(data),
	})
	resource := ResourceRef{
		Kind:     "storage",
		ID:       r.name,
		Module:   r.exec.Module,
		TenantID: r.exec.TenantID,
		Attributes: map[string]any{
			"path": path,
		},
	}
	record, err := r.effects.Before(*r.exec, spec, resource)
	if err != nil {
		return err
	}

	err = r.backend.Write(ctx, path, data)
	if afterErr := r.effects.After(record, err); afterErr != nil {
		return afterErr
	}
	if err != nil {
		return err
	}
	return nil
}

func (r guardedStorageResource) Read(ctx context.Context, path string) ([]byte, error) {
	requiredCap := CapabilityRef("storage.read:" + r.name)
	if err := r.caps.Check(*r.exec, requiredCap); err != nil {
		return nil, err
	}

	spec := r.specFor("storage.read."+r.name, "storage.read", requiredCap, map[string]any{
		"resource": r.name,
		"path":     path,
	})
	resource := ResourceRef{
		Kind:     "storage",
		ID:       r.name,
		Module:   r.exec.Module,
		TenantID: r.exec.TenantID,
		Attributes: map[string]any{
			"path": path,
		},
	}
	record, err := r.effects.Before(*r.exec, spec, resource)
	if err != nil {
		return nil, err
	}

	data, err := r.backend.Read(ctx, path)
	if afterErr := r.effects.After(record, err); afterErr != nil {
		return nil, afterErr
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (r guardedStorageResource) Delete(ctx context.Context, path string) error {
	requiredCap := CapabilityRef("storage.write:" + r.name)
	if err := r.caps.Check(*r.exec, requiredCap); err != nil {
		return err
	}

	spec := r.specFor("storage.delete."+r.name, "storage.write", requiredCap, map[string]any{
		"resource": r.name,
		"path":     path,
	})
	resource := ResourceRef{
		Kind:     "storage",
		ID:       r.name,
		Module:   r.exec.Module,
		TenantID: r.exec.TenantID,
		Attributes: map[string]any{
			"path": path,
		},
	}
	record, err := r.effects.Before(*r.exec, spec, resource)
	if err != nil {
		return err
	}

	err = r.backend.Delete(ctx, path)
	if afterErr := r.effects.After(record, err); afterErr != nil {
		return afterErr
	}
	if err != nil {
		return err
	}
	return nil
}

type missingStorageResource struct {
	name string
}

func (r missingStorageResource) Write(ctx context.Context, path string, data []byte) error {
	return newKernelError(CodeResourceNotFound, fmt.Sprintf("storage resource %q was not found", r.name), nil)
}

func (r missingStorageResource) Read(ctx context.Context, path string) ([]byte, error) {
	return nil, newKernelError(CodeResourceNotFound, fmt.Sprintf("storage resource %q was not found", r.name), nil)
}

func (r missingStorageResource) Delete(ctx context.Context, path string) error {
	return newKernelError(CodeResourceNotFound, fmt.Sprintf("storage resource %q was not found", r.name), nil)
}

type layeredStorageResource struct {
	layers []StorageResource
}

func (r layeredStorageResource) Write(ctx context.Context, path string, data []byte) error {
	for _, layer := range r.layers {
		if err := layer.Write(ctx, path, data); err != nil {
			return err
		}
	}
	return nil
}

func (r layeredStorageResource) Read(ctx context.Context, path string) ([]byte, error) {
	var firstErr error
	for index, layer := range r.layers {
		data, err := layer.Read(ctx, path)
		if err == nil {
			if index > 0 {
				for warmIndex := 0; warmIndex < index; warmIndex++ {
					_ = r.layers[warmIndex].Write(ctx, path, data)
				}
			}
			return data, nil
		}
		if IsNotFoundError(err) || IsCode(err, CodeResourceNotFound) {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		return nil, err
	}

	if firstErr != nil {
		return nil, firstErr
	}
	return nil, newKernelError(CodeResourceNotFound, "layered storage read failed", nil)
}

func (r layeredStorageResource) Delete(ctx context.Context, path string) error {
	var firstErr error
	for _, layer := range r.layers {
		err := layer.Delete(ctx, path)
		if err != nil && !IsNotFoundError(err) && !IsCode(err, CodeResourceNotFound) {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

type tenantStorageResource struct {
	name          string
	defaultTenant StorageResource
	tenants       map[string]StorageResource
}

func (r tenantStorageResource) Write(ctx context.Context, path string, data []byte) error {
	resource, err := r.forTenant(ctx)
	if err != nil {
		return err
	}
	return resource.Write(ctx, path, data)
}

func (r tenantStorageResource) Read(ctx context.Context, path string) ([]byte, error) {
	resource, err := r.forTenant(ctx)
	if err != nil {
		return nil, err
	}
	return resource.Read(ctx, path)
}

func (r tenantStorageResource) Delete(ctx context.Context, path string) error {
	resource, err := r.forTenant(ctx)
	if err != nil {
		return err
	}
	return resource.Delete(ctx, path)
}

func (r tenantStorageResource) forTenant(ctx context.Context) (StorageResource, error) {
	tenantID := TenantIDFromContext(ctx)
	if tenantID == "" {
		if r.defaultTenant != nil {
			return r.defaultTenant, nil
		}
		return nil, newKernelError(CodeTenantBindingMissing, fmt.Sprintf("storage resource %q requires tenant binding", r.name), nil)
	}

	resource, ok := r.tenants[tenantID]
	if ok {
		return resource, nil
	}
	if r.defaultTenant != nil {
		return r.defaultTenant, nil
	}

	return nil, newKernelError(
		CodeTenantBindingMissing,
		fmt.Sprintf("storage resource %q has no binding for tenant %q", r.name, tenantID),
		nil,
	)
}

func cloneStorageBindingInfo(in StorageBindingInfo) StorageBindingInfo {
	out := StorageBindingInfo{
		Name:         in.Name,
		Driver:       in.Driver,
		Provider:     in.Provider,
		Layered:      in.Layered,
		MultiTenant:  in.MultiTenant,
		HotSwappable: in.HotSwappable,
		Tenants:      cloneStringSlice(in.Tenants),
	}
	if len(in.Providers) > 0 {
		out.Providers = make([]StorageProviderInfo, len(in.Providers))
		for index, provider := range in.Providers {
			out.Providers[index] = StorageProviderInfo{
				Name:   provider.Name,
				Driver: provider.Driver,
				Config: cloneMap(provider.Config),
				Stats:  provider.Stats,
			}
		}
	}
	return out
}

func mergeStorageProviderInfo(sets ...[]StorageProviderInfo) []StorageProviderInfo {
	out := make([]StorageProviderInfo, 0)
	for _, set := range sets {
		for _, provider := range set {
			out = append(out, StorageProviderInfo{
				Name:   provider.Name,
				Driver: provider.Driver,
				Config: cloneMap(provider.Config),
				Stats:  provider.Stats,
			})
		}
	}
	return out
}

func newResourceNotImplementedError(kind, name string) error {
	return newKernelError(
		CodeResourceNotImplemented,
		fmt.Sprintf("%s resource %q is not implemented yet", kind, name),
		nil,
	)
}

type notImplementedCacheResource struct {
	name string
}

func (r notImplementedCacheResource) Get(ctx context.Context, key string) ([]byte, error) {
	return nil, newResourceNotImplementedError("cache", r.name)
}

func (r notImplementedCacheResource) Set(ctx context.Context, key string, value []byte) error {
	return newResourceNotImplementedError("cache", r.name)
}

func (r notImplementedCacheResource) Delete(ctx context.Context, key string) error {
	return newResourceNotImplementedError("cache", r.name)
}

type notImplementedSQLResource struct {
	name string
}

func (r notImplementedSQLResource) Query(ctx context.Context, query string, args ...any) (Rows, error) {
	return nil, newResourceNotImplementedError("sql", r.name)
}

func (r notImplementedSQLResource) Exec(ctx context.Context, query string, args ...any) (Result, error) {
	return nil, newResourceNotImplementedError("sql", r.name)
}

type notImplementedHTTPResource struct {
	name string
}

func (r notImplementedHTTPResource) Do(req *http.Request) (*http.Response, error) {
	return nil, newResourceNotImplementedError("http", r.name)
}

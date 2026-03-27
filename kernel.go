package aegis

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

type Builder struct {
	config               Config
	modules              []Module
	drivers              *DriverRegistry
	policy               PolicyEngine
	policyRegistry       *PolicyRegistry
	globalPolicies       []Policy
	audit                AuditWriter
	observer             ObservabilityHooks
	effectStore          *MemoryEffectStore
	highAssuranceEffects bool
}

func NewBuilder(config Config) *Builder {
	return &Builder{
		config:         config,
		drivers:        NewDriverRegistry(),
		policyRegistry: NewPolicyRegistry(),
		audit:          NoopAuditWriter{},
		observer:       NoopObserver{},
		effectStore:    NewMemoryEffectStore(),
	}
}

func (b *Builder) WithModule(module Module) *Builder {
	b.modules = append(b.modules, module)
	return b
}

func (b *Builder) WithModules(modules ...Module) *Builder {
	b.modules = append(b.modules, modules...)
	return b
}

func (b *Builder) WithDriverRegistry(drivers *DriverRegistry) *Builder {
	if drivers != nil {
		b.drivers = drivers
	}
	return b
}

func (b *Builder) WithPolicyEngine(policy PolicyEngine) *Builder {
	b.policy = policy
	return b
}

func (b *Builder) WithPolicyRegistry(registry *PolicyRegistry) *Builder {
	if registry != nil {
		b.policyRegistry = registry
	}
	return b
}

func (b *Builder) WithPolicies(policies ...Policy) *Builder {
	b.globalPolicies = append(b.globalPolicies, policies...)
	return b
}

func (b *Builder) WithAuditWriter(writer AuditWriter) *Builder {
	if writer != nil {
		b.audit = writer
	}
	return b
}

func (b *Builder) WithObservability(hooks ObservabilityHooks) *Builder {
	if hooks != nil {
		b.observer = hooks
	}
	return b
}

func (b *Builder) WithEffectStore(store *MemoryEffectStore) *Builder {
	if store != nil {
		b.effectStore = store
	}
	return b
}

func (b *Builder) WithHighAssuranceEffects(enabled bool) *Builder {
	b.highAssuranceEffects = enabled
	return b
}

func (b *Builder) Build() (*Kernel, error) {
	registry := NewOperationRegistry()
	for _, module := range b.modules {
		if err := registry.RegisterModule(module); err != nil {
			return nil, err
		}
		for _, policy := range module.Policies() {
			if err := b.policyRegistry.Register(policy); err != nil {
				return nil, err
			}
		}
	}
	for _, policy := range b.globalPolicies {
		if err := b.policyRegistry.Register(policy); err != nil {
			return nil, err
		}
	}

	resources := NewResourceManager()
	if err := BindResources(b.config, b.drivers, resources); err != nil {
		return nil, err
	}

	policyEngine := b.policy
	if policyEngine == nil {
		policyEngine = NewPolicyEngine(b.policyRegistry)
	}

	return &Kernel{
		ops:                  registry,
		caps:                 NewCapabilityManager(b.observer),
		resources:            resources,
		drivers:              b.drivers,
		policy:               policyEngine,
		policyRegistry:       b.policyRegistry,
		audit:                b.audit,
		observer:             b.observer,
		effectStore:          b.effectStore,
		config:               b.config,
		highAssuranceEffects: b.highAssuranceEffects,
	}, nil
}

type Kernel struct {
	ops                  *OperationRegistry
	caps                 *CapabilityManager
	resources            *ResourceManager
	drivers              *DriverRegistry
	policy               PolicyEngine
	policyRegistry       *PolicyRegistry
	audit                AuditWriter
	observer             ObservabilityHooks
	effectStore          *MemoryEffectStore
	config               Config
	highAssuranceEffects bool
	activeCriticalOps    atomic.Int64
}

func (k *Kernel) Execute(ctx context.Context, opName string, input any) (_ any, err error) {
	if k == nil {
		return nil, newKernelError(CodeBootstrapFailed, "kernel is nil", nil)
	}

	entry, ok := k.ops.Lookup(opName)
	if !ok {
		return nil, newKernelError(
			CodeOperationNotFound,
			fmt.Sprintf("operation %q is not registered", opName),
			nil,
		)
	}

	audit := k.audit
	if audit == nil {
		audit = NoopAuditWriter{}
	}

	observer := k.observer
	if observer == nil {
		observer = NoopObserver{}
	}

	tracker := newExecutionEffectTracker(k.effectStore, k.policy, k.highAssuranceEffects)
	execResources := newExecutionResources(k.resources, k.caps, tracker, entry.descriptor)
	subject := SubjectFromContext(ctx)
	grantedCaps, hasGrantedCaps := grantedCapabilitiesStateFromContext(ctx)
	capabilityResolution := ResolveCapabilities(subject, grantedCaps, hasGrantedCaps)

	exec := ExecutionContext{
		Operation:            opName,
		Module:               entry.module.Name,
		Subject:              subject,
		Capabilities:         capabilityResolution.Effective,
		CapabilityResolution: capabilityResolution,
		Deadline:             deadlineFromContext(ctx),
		Metadata:             MetadataFromContext(ctx),
		Resources:            execResources,
		RequestID:            RequestIDFromContext(ctx),
		TraceID:              TraceIDFromContext(ctx),
		Environment:          EnvironmentFromContext(ctx),
		Transport:            TransportFromContext(ctx),
		TenantID:             TenantIDFromContext(ctx),
		DegradationMode:      DegradationModeFromContext(ctx),
	}
	if _, ok := exec.Metadata["invocation_class"]; !ok {
		exec.Metadata["invocation_class"] = invocationClass(entry.descriptor)
	}
	execResources.bind(&exec)
	for _, effect := range entry.descriptor.Effects {
		if planErr := tracker.Plan(exec, effect); planErr != nil {
			return nil, planErr
		}
	}
	if operationHasCriticalEffects(entry.descriptor) {
		k.activeCriticalOps.Add(1)
		defer k.activeCriticalOps.Add(-1)
	}

	startedAt := time.Now().UTC()
	observer.OperationStarted(ctx, exec)

	var (
		out               any
		effects           []EffectRecord
		auditErr          error
		policyExplanation DecisionExplanation
	)

	defer func() {
		if flushErr := tracker.Flush(); flushErr != nil && err == nil {
			err = flushErr
		}
		effects = tracker.Records(exec)

		report := ExecutionReport{
			Execution:         exec,
			Decision:          exec.Decision,
			PolicyExplanation: policyExplanation,
			Effects:           effects,
			StartedAt:         startedAt,
			FinishedAt:        time.Now().UTC(),
			Err:               err,
			AuditErr:          auditErr,
		}
		if writeErr := audit.Write(ctx, report.AuditEvent()); writeErr != nil {
			auditErr = newKernelError(CodeAuditFailed, "failed to emit audit event", writeErr)
			if err == nil {
				err = auditErr
			}
			report.AuditErr = auditErr
		}
		observer.OperationFinished(ctx, report)
	}()

	for _, capability := range entry.descriptor.RequiredCapabilities {
		if err := k.caps.Check(exec, capability); err != nil {
			return nil, err
		}
	}

	policyRefs := append(clonePolicyRefs(entry.module.RequiredPolicies), entry.descriptor.RequiredPolicies...)
	policyExplanation, err = k.policy.Explain(ctx, policyContextFor(exec, input, ResourceRef{}), policyRefs)
	if err != nil {
		return nil, err
	}
	exec.Decision = policyExplanation.FinalDecision
	if !exec.Decision.Allowed {
		return nil, newKernelError(CodePolicyDenied, exec.Decision.Reason, nil)
	}
	if exec.Decision.Confirm && !ConfirmedFromContext(ctx) {
		return nil, newKernelError(CodeConfirmationNeeded, "operation requires explicit confirmation", nil)
	}

	ctx, cancel := applyDecision(ctx, &exec, exec.Decision)
	defer cancel()

	if err := entry.operation.Validate(input); err != nil {
		return nil, err
	}

	out, err = entry.operation.Execute(ctx, exec, input)
	if err != nil {
		return nil, err
	}

	if err := execResources.Commit(); err != nil {
		return nil, err
	}

	out, err = applyRedactions(out, exec.Decision.Redactions)
	if err != nil {
		return nil, err
	}

	return out, nil
}

func policyContextFor(exec ExecutionContext, input any, resource ResourceRef) PolicyContext {
	invocationClass, _ := exec.Metadata["invocation_class"].(string)
	return PolicyContext{
		RequestID:            exec.RequestID,
		TraceID:              exec.TraceID,
		Timestamp:            time.Now().UTC(),
		Environment:          exec.Environment,
		Module:               exec.Module,
		Operation:            exec.Operation,
		Subject:              exec.Subject,
		Resource:             resource,
		Input:                input,
		Metadata:             cloneMap(exec.Metadata),
		Capabilities:         exec.Capabilities.clone(),
		CapabilityResolution: exec.CapabilityResolution,
		DegradationMode:      exec.DegradationMode,
		InvocationClass:      invocationClass,
		Transport:            exec.Transport,
		TenantID:             exec.TenantID,
	}
}

func applyDecision(ctx context.Context, exec *ExecutionContext, decision Decision) (context.Context, context.CancelFunc) {
	metadata := cloneMap(exec.Metadata)
	if len(decision.Tags) > 0 {
		tags := make(map[string]any, len(decision.Tags))
		for key, value := range decision.Tags {
			tags[key] = value
		}
		metadata["policy_tags"] = tags
	}
	exec.Metadata = metadata

	var (
		deadline time.Time
		hasLimit bool
	)
	for _, obligation := range decision.Obligations {
		switch obligation.Type {
		case "limit_timeout_ms":
			value, ok := obligation.Params["ms"].(int)
			if !ok {
				if numeric, ok := obligation.Params["ms"].(float64); ok {
					value = int(numeric)
				}
			}
			if value <= 0 {
				continue
			}
			candidate := time.Now().Add(time.Duration(value) * time.Millisecond)
			if !hasLimit || candidate.Before(deadline) {
				deadline = candidate
				hasLimit = true
			}
		}
	}

	if hasLimit && (exec.Deadline.IsZero() || deadline.Before(exec.Deadline)) {
		exec.Deadline = deadline
		return context.WithDeadline(ctx, deadline)
	}

	return ctx, func() {}
}

func (k *Kernel) SwapStorageBinding(ctx context.Context, name string, binding StorageBinding) error {
	if k == nil {
		return newKernelError(CodeBootstrapFailed, "kernel is nil", nil)
	}
	if k.activeCriticalOps.Load() > 0 {
		return newKernelError(CodeHotSwapDenied, "cannot hot-swap while critical operations are active", nil)
	}
	if k.drivers == nil {
		return newKernelError(CodeBootstrapFailed, "driver registry is not available", nil)
	}

	current, ok := k.resources.StorageBindingInfo(name)
	if !ok {
		return newKernelError(CodeResourceNotFound, fmt.Sprintf("storage resource %q was not found", name), nil)
	}
	if !current.HotSwappable {
		return newKernelError(CodeHotSwapDenied, fmt.Sprintf("storage resource %q is not hot-swappable", name), nil)
	}

	resource, info, err := resolveStorageBinding(name, name, binding, k.drivers)
	if err != nil {
		return err
	}
	info.HotSwappable = true
	if err := k.resources.SwapStorage(name, resource, info); err != nil {
		return err
	}

	if k.config.Resources.Storage == nil {
		k.config.Resources.Storage = map[string]StorageBinding{}
	}
	binding.HotSwappable = true
	k.config.Resources.Storage[name] = binding.clone()
	return nil
}

func operationHasCriticalEffects(descriptor OperationDescriptor) bool {
	for _, effect := range descriptor.Effects {
		if effect.Critical {
			return true
		}
	}
	return false
}

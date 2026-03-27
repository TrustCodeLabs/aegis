package aegis

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testInput struct {
	Value string `json:"value"`
}

type schemaCarrier struct {
	ID       string            `json:"id"`
	Optional *int              `json:"optional,omitempty"`
	Raw      []byte            `json:"raw"`
	Scores   [2]int            `json:"scores"`
	Meta     map[string]string `json:"meta"`
	Skip     string            `json:"-"`
}

type memoryStorageResource struct {
	data      map[string][]byte
	writeErr  error
	readErr   error
	deleteErr error
	writeHits int
}

func newMemoryStorageResource() *memoryStorageResource {
	return &memoryStorageResource{data: map[string][]byte{}}
}

func (r *memoryStorageResource) Write(ctx context.Context, path string, data []byte) error {
	if r.writeErr != nil {
		return r.writeErr
	}
	r.writeHits++
	r.data[path] = append([]byte(nil), data...)
	return nil
}

func (r *memoryStorageResource) Read(ctx context.Context, path string) ([]byte, error) {
	if r.readErr != nil {
		return nil, r.readErr
	}
	data, ok := r.data[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

func (r *memoryStorageResource) Delete(ctx context.Context, path string) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	if _, ok := r.data[path]; !ok {
		return os.ErrNotExist
	}
	delete(r.data, path)
	return nil
}

type stubStorageDriver struct {
	files     map[string][]byte
	initErr   error
	healthErr error
	writeErr  error
	readErr   error
	deleteErr error
}

func (d *stubStorageDriver) Init(config map[string]any) error {
	return d.initErr
}

func (d *stubStorageDriver) Write(ctx context.Context, path string, data []byte) error {
	if d.writeErr != nil {
		return d.writeErr
	}
	if d.files == nil {
		d.files = map[string][]byte{}
	}
	d.files[path] = append([]byte(nil), data...)
	return nil
}

func (d *stubStorageDriver) Read(ctx context.Context, path string) ([]byte, error) {
	if d.readErr != nil {
		return nil, d.readErr
	}
	data, ok := d.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

func (d *stubStorageDriver) Delete(ctx context.Context, path string) error {
	if d.deleteErr != nil {
		return d.deleteErr
	}
	if _, ok := d.files[path]; !ok {
		return os.ErrNotExist
	}
	delete(d.files, path)
	return nil
}

func (d *stubStorageDriver) HealthCheck(ctx context.Context) error {
	return d.healthErr
}

type recordingObserver struct {
	started          int
	finished         int
	capabilityChecks int
	lastAllowed      bool
	lastReport       ExecutionReport
	lastExecution    ExecutionContext
}

func (o *recordingObserver) OperationStarted(ctx context.Context, exec ExecutionContext) {
	o.started++
	o.lastExecution = exec
}

func (o *recordingObserver) OperationFinished(ctx context.Context, report ExecutionReport) {
	o.finished++
	o.lastReport = report
}

func (o *recordingObserver) CapabilityChecked(exec ExecutionContext, capability CapabilityRef, allowed bool) {
	o.capabilityChecks++
	o.lastAllowed = allowed
}

type recordingAuditWriter struct {
	events []AuditEvent
}

func (w *recordingAuditWriter) Write(ctx context.Context, event AuditEvent) error {
	w.events = append(w.events, event)
	return nil
}

type staticPolicyEngine struct {
	explanation DecisionExplanation
}

func (e *staticPolicyEngine) Evaluate(ctx context.Context, in PolicyContext, refs []PolicyRef) (Decision, error) {
	return e.explanation.FinalDecision, nil
}

func (e *staticPolicyEngine) Explain(ctx context.Context, in PolicyContext, refs []PolicyRef) (DecisionExplanation, error) {
	return e.explanation, nil
}

func TestContextAndErrorHelpers(t *testing.T) {
	subject := Subject{
		ID:           "user-1",
		Type:         "user",
		Roles:        []string{"editor"},
		Capabilities: []CapabilityRef{"storage.read:notes"},
		Attributes:   map[string]any{"team": "core"},
	}
	metadata := map[string]any{"feature": "notes"}

	ctx := context.Background()
	ctx = WithSubject(ctx, subject)
	ctx = WithCapabilities(ctx, NewGrantedCapabilities("storage.write:notes"))
	ctx = WithMetadata(ctx, metadata)
	ctx = WithEnvironment(ctx, "test")
	ctx = WithTransport(ctx, "http")
	ctx = WithTenantID(ctx, "tenant-a")
	ctx = WithRequestID(ctx, "req-1")
	ctx = WithTraceID(ctx, "trace-1")
	ctx = WithDegradationMode(ctx, DegradationMode("readonly"))

	subject.Roles[0] = "mutated"
	subject.Capabilities[0] = "mutated"
	subject.Attributes["team"] = "changed"
	metadata["feature"] = "changed"

	storedSubject := SubjectFromContext(ctx)
	if storedSubject.Roles[0] != "editor" {
		t.Fatalf("expected cloned subject roles, got %#v", storedSubject.Roles)
	}
	if storedSubject.Capabilities[0] != CapabilityRef("storage.read:notes") {
		t.Fatalf("expected cloned subject capabilities, got %#v", storedSubject.Capabilities)
	}
	if storedSubject.Attributes["team"] != "core" {
		t.Fatalf("expected cloned subject attributes, got %#v", storedSubject.Attributes)
	}

	granted := GrantedCapabilitiesFromContext(ctx)
	if !granted.Has("storage.write:notes") {
		t.Fatalf("expected granted capabilities from context")
	}

	if MetadataFromContext(ctx)["feature"] != "notes" {
		t.Fatalf("expected cloned metadata map")
	}
	if EnvironmentFromContext(ctx) != "test" {
		t.Fatalf("unexpected environment")
	}
	if TransportFromContext(ctx) != "http" {
		t.Fatalf("unexpected transport")
	}
	if TenantIDFromContext(ctx) != "tenant-a" {
		t.Fatalf("unexpected tenant id")
	}
	if RequestIDFromContext(ctx) != "req-1" || TraceIDFromContext(ctx) != "trace-1" {
		t.Fatalf("unexpected tracing context")
	}
	if DegradationModeFromContext(ctx) != DegradationMode("readonly") {
		t.Fatalf("unexpected degradation mode")
	}

	err := NewKernelError(CodeInvalidConfig, "invalid test config", os.ErrPermission)
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected wrapped cause")
	}
	if !IsCode(err, CodeInvalidConfig) {
		t.Fatalf("expected kernel error code")
	}
	if !strings.Contains(err.Error(), "invalid test config") {
		t.Fatalf("unexpected kernel error string: %v", err)
	}

	binding := StorageBinding{
		Provider: "files",
		Config: map[string]any{
			"root":  "/tmp/runtime",
			"count": 3,
		},
	}
	if root, ok := binding.LookupString("root"); !ok || root != "/tmp/runtime" {
		t.Fatalf("unexpected lookup result: %q %v", root, ok)
	}
	if _, ok := binding.LookupString("count"); ok {
		t.Fatalf("expected non-string config lookup to fail")
	}
	if provider := binding.ProviderName("fallback"); provider != "files" {
		t.Fatalf("unexpected provider name: %q", provider)
	}
}

func TestDriverPolicyAndDecisionHelpers(t *testing.T) {
	driver := &stubStorageDriver{
		files: map[string][]byte{"note.txt": []byte("hello")},
	}
	observed := newObservedStorageDriver("provider-a", "stub", driver)

	if err := observed.Delete(context.Background(), "note.txt"); err != nil {
		t.Fatalf("delete through observed driver: %v", err)
	}
	stats := observed.Stats()
	if stats.OperationCount == 0 {
		t.Fatalf("expected driver stats to count delete operation")
	}

	driver.deleteErr = os.ErrNotExist
	err := observed.Delete(context.Background(), "missing.txt")
	if !IsDriverKind(err, DriverErrorKindNotFound) {
		t.Fatalf("expected not_found driver error, got %v", err)
	}
	if !IsNotFoundError(err) {
		t.Fatalf("expected not found helper to detect driver error")
	}

	canceled := wrapDriverError("provider-a", "stub", "delete", context.Canceled)
	if !IsDriverKind(canceled, DriverErrorKindUnavailable) {
		t.Fatalf("expected unavailable driver error, got %v", canceled)
	}

	typed := wrapDriverError("provider-a", "stub", "write", &DriverError{
		Kind:  DriverErrorKindInvalidInput,
		Cause: errors.New("bad payload"),
	})
	if !IsDriverKind(typed, DriverErrorKindInvalidInput) {
		t.Fatalf("expected invalid_input driver error, got %v", typed)
	}
	if !strings.Contains(typed.Error(), "bad payload") {
		t.Fatalf("unexpected driver error string: %v", typed)
	}

	registry := NewPolicyRegistry()
	allow := DefinePolicy(PolicySpec{
		ID:          "ops.allow",
		Category:    "access",
		Module:      "ops",
		Description: "allows operations with audit obligation",
		AppliesTo:   []string{"ops.inspect"},
		Severity:    "low",
		Handler: func(ctx context.Context, in PolicyContext) (Decision, error) {
			return Decision{
				Allowed: true,
				Reason:  "allowed",
				Obligations: []Obligation{
					{Type: "emit_audit", Params: map[string]any{"stream": "ops"}},
				},
				Tags: map[string]string{"team": "core"},
			}, nil
		},
	})
	deny := DefinePolicy(PolicySpec{
		ID:       "ops.deny",
		Module:   "ops",
		Severity: "high",
		Handler: func(ctx context.Context, in PolicyContext) (Decision, error) {
			return Decision{Allowed: false, Reason: "denied", Tags: map[string]string{"team": "core"}}, nil
		},
	})

	if err := registry.Register(deny); err != nil {
		t.Fatalf("register deny policy: %v", err)
	}
	if err := registry.Register(allow); err != nil {
		t.Fatalf("register allow policy: %v", err)
	}

	all := registry.All()
	if len(all) != 2 || all[0].ID() != "ops.allow" || all[1].ID() != "ops.deny" {
		t.Fatalf("expected sorted policy registry output, got %#v", all)
	}
	if allow.Metadata().Description == "" {
		t.Fatalf("expected policy metadata to be preserved")
	}

	engine := NewPolicyEngine(registry)
	decision, err := engine.Evaluate(context.Background(), PolicyContext{}, []PolicyRef{
		PolicyID("ops.allow"),
		PolicyID("ops.deny"),
	})
	if err != nil {
		t.Fatalf("evaluate policy engine: %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected merged decision to deny")
	}
	if len(decision.Obligations) != 1 {
		t.Fatalf("expected obligations to survive deny merge, got %#v", decision.Obligations)
	}

	_, err = mergeDecision(
		Decision{Allowed: true, Tags: map[string]string{"team": "core"}},
		Decision{Allowed: true, Tags: map[string]string{"team": "other"}},
	)
	if !IsCode(err, CodePolicyDenied) {
		t.Fatalf("expected conflicting tags to fail merge, got %v", err)
	}

	exec := ExecutionContext{
		Metadata: map[string]any{"existing": true},
	}
	ctx, cancel := applyDecision(context.Background(), &exec, Decision{
		Tags: map[string]string{"risk": "low"},
		Obligations: []Obligation{
			{Type: "limit_timeout_ms", Params: map[string]any{"ms": float64(25)}},
		},
	})
	defer cancel()

	if exec.Metadata["existing"] != true {
		t.Fatalf("expected metadata to be preserved")
	}
	policyTags, ok := exec.Metadata["policy_tags"].(map[string]any)
	if !ok || policyTags["risk"] != "low" {
		t.Fatalf("expected policy tags in execution metadata, got %#v", exec.Metadata)
	}
	if exec.Deadline.IsZero() {
		t.Fatalf("expected deadline to be constrained by obligation")
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		t.Fatalf("expected context deadline after applying decision")
	}

	NoopObserver{}.OperationStarted(context.Background(), ExecutionContext{})
	NoopObserver{}.OperationFinished(context.Background(), ExecutionReport{})
	NoopObserver{}.CapabilityChecked(ExecutionContext{}, "ops.inspect", true)
}

func TestResourceManagerAndBindingsHelpers(t *testing.T) {
	manager := NewResourceManager()
	storage := newMemoryStorageResource()

	if err := manager.RegisterStorage("mem", storage); err != nil {
		t.Fatalf("register storage: %v", err)
	}
	if err := manager.RegisterStorage("mem", storage); !IsCode(err, CodeDuplicateResource) {
		t.Fatalf("expected duplicate resource error, got %v", err)
	}
	if _, ok := manager.LookupStorage("mem"); !ok {
		t.Fatalf("expected lookup to find registered storage")
	}
	info, ok := manager.StorageBindingInfo("mem")
	if !ok || info.Driver != "direct" {
		t.Fatalf("unexpected storage binding info: %#v", info)
	}

	missing := missingStorageResource{name: "ghost"}
	if err := missing.Write(context.Background(), "a.txt", []byte("x")); !IsCode(err, CodeResourceNotFound) {
		t.Fatalf("expected missing write to fail, got %v", err)
	}
	if _, err := missing.Read(context.Background(), "a.txt"); !IsCode(err, CodeResourceNotFound) {
		t.Fatalf("expected missing read to fail, got %v", err)
	}
	if err := missing.Delete(context.Background(), "a.txt"); !IsCode(err, CodeResourceNotFound) {
		t.Fatalf("expected missing delete to fail, got %v", err)
	}

	cacheLayer := newMemoryStorageResource()
	durableLayer := newMemoryStorageResource()
	durableLayer.data["doc.txt"] = []byte("payload")
	layered := layeredStorageResource{layers: []StorageResource{cacheLayer, durableLayer}}

	data, err := layered.Read(context.Background(), "doc.txt")
	if err != nil {
		t.Fatalf("layered read: %v", err)
	}
	if string(data) != "payload" {
		t.Fatalf("unexpected layered read payload: %q", string(data))
	}
	if cacheLayer.writeHits == 0 {
		t.Fatalf("expected layered read to warm earlier layers")
	}
	if err := layered.Delete(context.Background(), "doc.txt"); err != nil {
		t.Fatalf("layered delete: %v", err)
	}

	defaultTenant := newMemoryStorageResource()
	tenantSpecific := newMemoryStorageResource()
	tenanted := tenantStorageResource{
		name:          "notes",
		defaultTenant: defaultTenant,
		tenants: map[string]StorageResource{
			"tenant-a": tenantSpecific,
		},
	}

	ctx := WithTenantID(context.Background(), "tenant-a")
	if err := tenanted.Write(ctx, "note.txt", []byte("tenant")); err != nil {
		t.Fatalf("tenant-specific write: %v", err)
	}
	if _, err := tenanted.Read(ctx, "note.txt"); err != nil {
		t.Fatalf("tenant-specific read: %v", err)
	}
	if err := tenanted.Delete(ctx, "note.txt"); err != nil {
		t.Fatalf("tenant-specific delete: %v", err)
	}

	fallbackCtx := WithTenantID(context.Background(), "tenant-b")
	if err := tenanted.Write(fallbackCtx, "note.txt", []byte("fallback")); err != nil {
		t.Fatalf("default tenant write: %v", err)
	}

	tenantless := tenantStorageResource{name: "notes", tenants: map[string]StorageResource{}}
	if _, err := tenantless.Read(WithTenantID(context.Background(), "tenant-z"), "note.txt"); !IsCode(err, CodeTenantBindingMissing) {
		t.Fatalf("expected tenant binding error, got %v", err)
	}

	cache := notImplementedCacheResource{name: "cache"}
	if err := cache.Set(context.Background(), "k", []byte("v")); !IsCode(err, CodeResourceNotImplemented) {
		t.Fatalf("expected cache set to be not implemented, got %v", err)
	}
	if err := cache.Delete(context.Background(), "k"); !IsCode(err, CodeResourceNotImplemented) {
		t.Fatalf("expected cache delete to be not implemented, got %v", err)
	}

	sql := notImplementedSQLResource{name: "sql"}
	if _, err := sql.Exec(context.Background(), "select 1"); !IsCode(err, CodeResourceNotImplemented) {
		t.Fatalf("expected sql exec to be not implemented, got %v", err)
	}

	external := notImplementedHTTPResource{name: "http"}
	if _, err := external.Do(&http.Request{}); !IsCode(err, CodeResourceNotImplemented) {
		t.Fatalf("expected external call to be not implemented, got %v", err)
	}
}

func TestKernelBuilderHooksIntrospectionAndSkills(t *testing.T) {
	observer := &recordingObserver{}
	audit := &recordingAuditWriter{}
	store := NewMemoryEffectStore()
	registry := NewPolicyRegistry()
	engine := &staticPolicyEngine{
		explanation: DecisionExplanation{
			FinalDecision: Decision{
				Allowed:   true,
				Reason:    "allowed by stub engine",
				PolicyIDs: []string{"ops.guard"},
				Tags:      map[string]string{"team": "core"},
				Obligations: []Obligation{
					{Type: "limit_timeout_ms", Params: map[string]any{"ms": 15}},
				},
			},
			Evaluations: []PolicyEvaluationRecord{
				{PolicyID: "ops.guard", Allowed: true, Reason: "allowed"},
			},
		},
	}

	module := NewModuleWithManifest(
		Manifest{
			Name:         "ops",
			Version:      "v1",
			Description:  "Ops module",
			Dependencies: []string{"infra"},
			AI: &ModuleAISpec{
				Title:   "Ops tools",
				Summary: "Read-only ops tooling.",
				Skills: []SkillSpec{
					{
						Name:        "ops.inspectors",
						Title:       "Ops inspectors",
						Description: "Inspect runtime state through AI-safe operations.",
						Operations:  []string{"ops.inspect"},
					},
				},
			},
		},
		[]Operation{
			DefineOperation[testInput, map[string]any](OperationSpec[testInput, map[string]any]{
				Name: "ops.inspect",
				RequiredCapabilities: []CapabilityRef{
					"ops.inspect",
				},
				RequiredPolicies: []PolicyRef{
					PolicyID("ops.guard"),
				},
				Effects: []EffectSpec{
					{
						Name:       "ops.inspect.request",
						Kind:       "observe.read",
						Idempotent: true,
					},
				},
				AI: &AIExposureSpec{
					Exposed:         true,
					Exposure:        "internal",
					Title:           "Inspect ops state",
					Summary:         "Returns runtime state for operators.",
					Description:     "Use this when an internal caller needs safe read-only runtime state.",
					InvocationClass: "read",
					UseCases:        []string{"an operator needs to inspect runtime metadata"},
					Tags:            []string{"ops", "runtime"},
				},
				Handler: func(ctx context.Context, exec ExecutionContext, input testInput) (map[string]any, error) {
					return map[string]any{
						"value":        input.Value,
						"request_id":   exec.RequestID,
						"trace_id":     exec.TraceID,
						"deadline_set": !exec.Deadline.IsZero(),
						"policy_tags":  exec.Metadata["policy_tags"],
					}, nil
				},
			}),
			DefineOperation[struct{}, map[string]any](OperationSpec[struct{}, map[string]any]{
				Name: "ops.status",
				RequiredCapabilities: []CapabilityRef{
					"ops.status",
				},
				AI: &AIExposureSpec{
					Exposed:         true,
					Exposure:        "internal",
					Title:           "Inspect ops status",
					Summary:         "Returns coarse runtime health.",
					InvocationClass: "read",
				},
				Handler: func(ctx context.Context, exec ExecutionContext, input struct{}) (map[string]any, error) {
					return map[string]any{"ok": true}, nil
				},
			}),
		},
		DefinePolicy(PolicySpec{
			ID:          "ops.guard",
			Category:    "access",
			Module:      "ops",
			Description: "guards ops.inspect",
			AppliesTo:   []string{"ops.inspect"},
			Severity:    "medium",
			Handler: func(ctx context.Context, in PolicyContext) (Decision, error) {
				return Decision{Allowed: true, Reason: "allowed"}, nil
			},
		}),
	)

	kernel, err := NewBuilder(Config{}).
		WithModules(module).
		WithPolicyRegistry(registry).
		WithPolicies(DefinePolicy(PolicySpec{
			ID:          "global.audit",
			Category:    "audit",
			Module:      "ops",
			Description: "global audit placeholder",
			Severity:    "low",
			Handler: func(ctx context.Context, in PolicyContext) (Decision, error) {
				return Decision{Allowed: true, Reason: "ok"}, nil
			},
		})).
		WithAuditWriter(audit).
		WithObservability(observer).
		WithEffectStore(store).
		WithPolicyEngine(engine).
		Build()
	if err != nil {
		t.Fatalf("build kernel: %v", err)
	}

	ctx := context.Background()
	ctx = WithSubject(ctx, Subject{ID: "user-1", Roles: []string{"operator"}})
	ctx = WithCapabilities(ctx, NewGrantedCapabilities("ops.inspect"))
	ctx = WithRequestID(ctx, "req-ops")
	ctx = WithTraceID(ctx, "trace-ops")

	rawOutput, err := kernel.InvokeTool(ctx, "ops.inspect", testInput{Value: "hello"})
	if err != nil {
		t.Fatalf("invoke tool: %v", err)
	}

	output, ok := rawOutput.(map[string]any)
	if !ok {
		t.Fatalf("unexpected output type: %T", rawOutput)
	}
	if output["request_id"] != "req-ops" || output["trace_id"] != "trace-ops" {
		t.Fatalf("unexpected operation output: %#v", output)
	}
	if output["deadline_set"] != true {
		t.Fatalf("expected policy obligation to constrain deadline")
	}
	if observer.started != 1 || observer.finished != 1 || observer.capabilityChecks != 1 || !observer.lastAllowed {
		t.Fatalf("expected observability hooks to be exercised, got %+v", observer)
	}
	if len(audit.events) != 1 || !audit.events[0].Success {
		t.Fatalf("expected audit event for successful execution, got %#v", audit.events)
	}

	modules, err := kernel.Modules(context.Background(), IntrospectionFilter{
		AIOnly:         true,
		VisibilityTier: "internal",
	})
	if err != nil {
		t.Fatalf("modules introspection: %v", err)
	}
	if len(modules) != 1 || modules[0].Name != "ops" {
		t.Fatalf("unexpected modules introspection: %#v", modules)
	}

	ops, err := kernel.Operations(context.Background(), IntrospectionFilter{
		AIOnly:         true,
		VisibilityTier: "internal",
	})
	if err != nil {
		t.Fatalf("operations introspection: %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("unexpected operations introspection: %#v", ops)
	}

	policies, err := kernel.Policies(context.Background(), IntrospectionFilter{})
	if err != nil {
		t.Fatalf("policies introspection: %v", err)
	}
	if len(policies) != 2 {
		t.Fatalf("expected both module and global policies, got %#v", policies)
	}

	capabilities, err := kernel.Capabilities(context.Background(), IntrospectionFilter{
		GrantedCapabilities:    NewGrantedCapabilities("ops.inspect", "ops.status"),
		UseGrantedCapabilities: true,
	})
	if err != nil {
		t.Fatalf("capabilities introspection: %v", err)
	}
	if len(capabilities) != 2 || !capabilities[0].Granted || !capabilities[1].Granted {
		t.Fatalf("unexpected capabilities introspection: %#v", capabilities)
	}

	topology, err := kernel.Topology(context.Background(), IntrospectionFilter{})
	if err != nil {
		t.Fatalf("topology introspection: %v", err)
	}
	if len(topology.Nodes) == 0 || len(topology.Edges) == 0 {
		t.Fatalf("expected topology graph to contain nodes and edges")
	}

	tools, err := kernel.MCPTools(context.Background(), IntrospectionFilter{
		VisibilityTier: "internal",
	})
	if err != nil {
		t.Fatalf("mcp tools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("unexpected MCP tools: %#v", tools)
	}

	effects, err := kernel.Effects(context.Background(), EffectQuery{Operation: "ops.inspect"})
	if err != nil {
		t.Fatalf("effects query: %v", err)
	}
	if len(effects) != 1 || effects[0].Status != "planned" {
		t.Fatalf("expected planned effect record, got %#v", effects)
	}

	skills, err := kernel.GenerateSkillsMarkdown(context.Background(), IntrospectionFilter{
		VisibilityTier: "internal",
	})
	if err != nil {
		t.Fatalf("generate skills markdown: %v", err)
	}
	if !strings.Contains(skills, "ops.inspectors") || !strings.Contains(skills, "`ops.inspect`") || !strings.Contains(skills, "ops.default") {
		t.Fatalf("unexpected generated skills markdown: %s", skills)
	}

	outputPath := filepath.Join(t.TempDir(), "generated", "SKILLS.md")
	if err := kernel.WriteSkillsMarkdown(context.Background(), IntrospectionFilter{
		VisibilityTier: "internal",
	}, outputPath); err != nil {
		t.Fatalf("write skills markdown: %v", err)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read generated skills file: %v", err)
	}
	if !strings.Contains(string(content), "Inspect ops state") {
		t.Fatalf("unexpected skills file content: %s", string(content))
	}
}

func TestEffectTrackerSchemaAndOperationHelpers(t *testing.T) {
	exec := ExecutionContext{
		Operation:    "ops.delete",
		Module:       "ops",
		Subject:      Subject{ID: "user-1"},
		TenantID:     "tenant-a",
		Capabilities: NewGrantedCapabilities("storage.write:files"),
	}

	capDenied := newExecutionEffectTracker(NewMemoryEffectStore(), nil, false)
	_, err := capDenied.Before(exec, EffectSpec{
		Name:        "effect.capability",
		Kind:        "storage.write",
		RequiredCap: "storage.write:missing",
	}, ResourceRef{Kind: "storage", ID: "files"})
	if !IsCode(err, CodeCapabilityDenied) {
		t.Fatalf("expected capability denial, got %v", err)
	}

	highAssurance := newExecutionEffectTracker(NewMemoryEffectStore(), nil, true)
	_, err = highAssurance.Before(exec, EffectSpec{
		Name:        "effect.undeclared",
		Kind:        "storage.write",
		RequiredCap: "storage.write:files",
		Critical:    true,
	}, ResourceRef{Kind: "storage", ID: "files"})
	if !IsCode(err, CodeEffectViolation) {
		t.Fatalf("expected effect violation, got %v", err)
	}

	policyDenied := newExecutionEffectTracker(NewMemoryEffectStore(), &staticPolicyEngine{
		explanation: DecisionExplanation{
			FinalDecision: Decision{Allowed: false, Reason: "policy denied"},
		},
	}, false)
	_, err = policyDenied.Before(exec, EffectSpec{
		Name:        "effect.policy",
		Kind:        "storage.write",
		RequiredCap: "storage.write:files",
		Policies:    []PolicyRef{PolicyID("deny")},
	}, ResourceRef{Kind: "storage", ID: "files"})
	if !IsCode(err, CodeEffectDenied) {
		t.Fatalf("expected effect policy denial, got %v", err)
	}

	store := NewMemoryEffectStore()
	tracker := newExecutionEffectTracker(store, nil, false)
	descriptor := OperationDescriptor{
		Effects: []EffectSpec{
			{
				Name:        "storage.delete.files",
				Kind:        "storage.write",
				RequiredCap: "storage.write:files",
				Metadata: map[string]any{
					"resource": "files",
				},
			},
		},
	}
	manager := NewResourceManager()
	backend := newMemoryStorageResource()
	backend.data["doc.txt"] = []byte("payload")
	if err := manager.RegisterStorage("files", backend); err != nil {
		t.Fatalf("register storage for delete test: %v", err)
	}

	execResources := newExecutionResources(manager, NewCapabilityManager(nil), tracker, descriptor)
	execResources.bind(&exec)
	if err := tracker.Plan(exec, descriptor.Effects[0]); err != nil {
		t.Fatalf("plan effect: %v", err)
	}
	if err := execResources.Storage("files").Delete(context.Background(), "doc.txt"); err != nil {
		t.Fatalf("guarded delete: %v", err)
	}
	if err := tracker.Flush(); err != nil {
		t.Fatalf("flush effect tracker: %v", err)
	}

	effects, err := store.Query(EffectQuery{Operation: "ops.delete"})
	if err != nil {
		t.Fatalf("query stored effects: %v", err)
	}
	if len(effects) != 2 {
		t.Fatalf("expected planned and completed effect records, got %#v", effects)
	}

	schema := SchemaOf[schemaCarrier]()
	if schema.Type != "object" || schema.Properties["raw"].Description != "binary" || schema.Properties["scores"].Type != "array" {
		t.Fatalf("unexpected schema output: %#v", schema)
	}
	if _, ok := schema.Properties["skip"]; ok {
		t.Fatalf("expected skipped json field to be omitted from schema")
	}

	op := DefineOperation[testInput, string](OperationSpec[testInput, string]{
		Name: "ops.cast",
		Validate: func(input testInput) error {
			if input.Value == "" {
				return errors.New("value is required")
			}
			return nil
		},
	})
	if err := op.Validate("wrong-type"); !IsCode(err, CodeInvalidInput) {
		t.Fatalf("expected cast validation error, got %v", err)
	}
	if err := op.Validate(testInput{}); !IsCode(err, CodeInvalidInput) {
		t.Fatalf("expected validator error to be wrapped, got %v", err)
	}
	if _, err := op.Execute(context.Background(), ExecutionContext{}, "wrong-type"); !IsCode(err, CodeInvalidInput) {
		t.Fatalf("expected cast execution error, got %v", err)
	}
	if _, err := op.Execute(context.Background(), ExecutionContext{}, testInput{Value: "ok"}); !IsCode(err, CodeBootstrapFailed) {
		t.Fatalf("expected missing handler bootstrap error, got %v", err)
	}
}

func TestGenerateSkillsMarkdownWithoutAIExposure(t *testing.T) {
	kernel, err := NewBuilder(Config{}).
		WithModule(NewModule(
			"plain",
			DefineOperation[struct{}, struct{}](OperationSpec[struct{}, struct{}]{
				Name: "plain.noop",
				Handler: func(ctx context.Context, exec ExecutionContext, input struct{}) (struct{}, error) {
					return struct{}{}, nil
				},
			}),
		)).
		Build()
	if err != nil {
		t.Fatalf("build plain kernel: %v", err)
	}

	content, err := kernel.GenerateSkillsMarkdown(context.Background(), IntrospectionFilter{})
	if err != nil {
		t.Fatalf("generate empty skills markdown: %v", err)
	}
	if !strings.Contains(content, "_No AI-exposed operations available._") {
		t.Fatalf("expected empty skills placeholder, got %s", content)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWD)
	}()

	if err := kernel.WriteSkillsMarkdown(context.Background(), IntrospectionFilter{}, ""); err != nil {
		t.Fatalf("write default skills markdown: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "generated", "SKILLS.md")); err != nil {
		t.Fatalf("expected default skills file to be created: %v", err)
	}
}

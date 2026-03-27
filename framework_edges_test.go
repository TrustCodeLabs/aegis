package aegis_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"aegis"
	"aegis/drivers/localstorage"
)

func TestCapabilitiesFallbackToSubjectWhenNoGrantedContext(t *testing.T) {
	module := aegis.NewModule(
		"identity",
		aegis.DefineOperation[struct{}, map[string]any](aegis.OperationSpec[struct{}, map[string]any]{
			Name: "identity.capabilities.inspect",
			Handler: func(ctx context.Context, exec aegis.ExecutionContext, input struct{}) (map[string]any, error) {
				return map[string]any{
					"source": string(exec.CapabilityResolution.Source),
					"caps":   exec.Capabilities.Slice(),
				}, nil
			},
		}),
	)

	kernel, err := aegis.NewBuilder(aegis.Config{}).
		WithModule(module).
		Build()
	if err != nil {
		t.Fatalf("build kernel: %v", err)
	}

	ctx := aegis.WithSubject(context.Background(), aegis.Subject{
		ID:           "user-1",
		Type:         "user",
		Capabilities: []aegis.CapabilityRef{"storage.read:user-uploads"},
	})

	rawOutput, err := kernel.Execute(ctx, "identity.capabilities.inspect", struct{}{})
	if err != nil {
		t.Fatalf("execute kernel: %v", err)
	}

	output, ok := rawOutput.(map[string]any)
	if !ok {
		t.Fatalf("unexpected output type: %T", rawOutput)
	}
	if output["source"] != string(aegis.CapabilitySourceSubject) {
		t.Fatalf("expected subject fallback source, got %#v", output["source"])
	}
}

func TestExplicitGrantedCapabilitiesOverrideSubjectCapabilities(t *testing.T) {
	module := aegis.NewModule(
		"uploads",
		aegis.DefineOperation[struct{}, bool](aegis.OperationSpec[struct{}, bool]{
			Name: "upload.read",
			RequiredCapabilities: []aegis.CapabilityRef{
				"storage.read:user-uploads",
			},
			Handler: func(ctx context.Context, exec aegis.ExecutionContext, input struct{}) (bool, error) {
				return true, nil
			},
		}),
	)

	kernel, err := aegis.NewBuilder(aegis.Config{}).
		WithModule(module).
		Build()
	if err != nil {
		t.Fatalf("build kernel: %v", err)
	}

	ctx := aegis.WithSubject(context.Background(), aegis.Subject{
		ID:           "user-1",
		Type:         "user",
		Capabilities: []aegis.CapabilityRef{"storage.read:user-uploads"},
	})
	ctx = aegis.WithCapabilities(ctx, aegis.NewGrantedCapabilities())

	_, err = kernel.Execute(ctx, "upload.read", struct{}{})
	if err == nil {
		t.Fatalf("expected explicit granted capabilities to override subject capabilities")
	}
	if !aegis.IsCode(err, aegis.CodeCapabilityDenied) {
		t.Fatalf("expected capability denial, got %v", err)
	}
}

func TestPolicyMergePreservesObligationsOnDeny(t *testing.T) {
	registry := aegis.NewPolicyRegistry()
	for _, policy := range []aegis.Policy{
		aegis.DefinePolicy(aegis.PolicySpec{
			ID:       "allow.with.audit",
			Module:   "identity",
			Severity: "low",
			Handler: func(ctx context.Context, in aegis.PolicyContext) (aegis.Decision, error) {
				return aegis.Decision{
					Allowed: true,
					Reason:  "allowed with audit trail",
					Obligations: []aegis.Obligation{
						{Type: "emit_audit", Params: map[string]any{"stream": "security"}},
					},
				}, nil
			},
		}),
		aegis.DefinePolicy(aegis.PolicySpec{
			ID:       "deny.by.default",
			Module:   "identity",
			Severity: "high",
			Handler: func(ctx context.Context, in aegis.PolicyContext) (aegis.Decision, error) {
				return aegis.Decision{
					Allowed: false,
					Reason:  "subject blocked by default",
				}, nil
			},
		}),
	} {
		if err := registry.Register(policy); err != nil {
			t.Fatalf("register policy %q: %v", policy.ID(), err)
		}
	}

	engine := aegis.NewPolicyEngine(registry)
	explanation, err := engine.Explain(context.Background(), aegis.PolicyContext{}, []aegis.PolicyRef{
		aegis.PolicyID("allow.with.audit"),
		aegis.PolicyID("deny.by.default"),
	})
	if err != nil {
		t.Fatalf("explain policies: %v", err)
	}

	if explanation.FinalDecision.Allowed {
		t.Fatalf("expected final decision to deny")
	}
	if explanation.FinalDecision.Reason != "subject blocked by default" {
		t.Fatalf("unexpected final reason: %q", explanation.FinalDecision.Reason)
	}
	if len(explanation.FinalDecision.Obligations) != 1 {
		t.Fatalf("expected deny merge to preserve obligations, got %d", len(explanation.FinalDecision.Obligations))
	}
	if explanation.FinalDecision.Obligations[0].Type != "emit_audit" {
		t.Fatalf("unexpected obligation: %#v", explanation.FinalDecision.Obligations[0])
	}
}

func TestPlaceholderResourcesReturnNotImplemented(t *testing.T) {
	module := aegis.NewModule(
		"infra",
		aegis.DefineOperation[string, bool](aegis.OperationSpec[string, bool]{
			Name: "infra.cache.get",
			Handler: func(ctx context.Context, exec aegis.ExecutionContext, input string) (bool, error) {
				_, err := exec.Resources.Cache("tenant-cache").Get(ctx, input)
				return err == nil, err
			},
		}),
		aegis.DefineOperation[string, bool](aegis.OperationSpec[string, bool]{
			Name: "infra.sql.query",
			Handler: func(ctx context.Context, exec aegis.ExecutionContext, input string) (bool, error) {
				_, err := exec.Resources.SQL("primary").Query(ctx, input)
				return err == nil, err
			},
		}),
		aegis.DefineOperation[string, bool](aegis.OperationSpec[string, bool]{
			Name: "infra.http.call",
			Handler: func(ctx context.Context, exec aegis.ExecutionContext, input string) (bool, error) {
				_, err := exec.Resources.External("crm").Do(nil)
				return err == nil, err
			},
		}),
	)

	kernel, err := aegis.NewBuilder(aegis.Config{}).
		WithModule(module).
		Build()
	if err != nil {
		t.Fatalf("build kernel: %v", err)
	}

	for _, operation := range []string{"infra.cache.get", "infra.sql.query", "infra.http.call"} {
		_, err := kernel.Execute(context.Background(), operation, "select 1")
		if err == nil {
			t.Fatalf("expected %s to return not implemented", operation)
		}
		if !aegis.IsCode(err, aegis.CodeResourceNotImplemented) {
			t.Fatalf("expected not implemented error for %s, got %v", operation, err)
		}
	}
}

func TestHotSwapDeniedWhileCriticalOperationActive(t *testing.T) {
	initialRoot := t.TempDir()
	swappedRoot := t.TempDir()

	registry := aegis.NewDriverRegistry()
	if err := localstorage.Register(registry); err != nil {
		t.Fatalf("register local driver: %v", err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)

	module := aegis.NewModule(
		"uploads",
		aegis.DefineOperation[struct{}, bool](aegis.OperationSpec[struct{}, bool]{
			Name: "upload.blocking.write",
			RequiredCapabilities: []aegis.CapabilityRef{
				"storage.write:user-uploads",
			},
			Effects: []aegis.EffectSpec{
				{
					Name:        "storage.write.user-uploads",
					Kind:        "storage.write",
					RequiredCap: "storage.write:user-uploads",
					Critical:    true,
				},
			},
			Handler: func(ctx context.Context, exec aegis.ExecutionContext, input struct{}) (bool, error) {
				close(started)
				<-release
				return true, nil
			},
		}),
	)

	kernel, err := aegis.NewBuilder(aegis.Config{
		Resources: aegis.ResourcesConfig{
			Storage: map[string]aegis.StorageBinding{
				"user-uploads": {
					Driver:       "local",
					HotSwappable: true,
					Config:       map[string]any{"root": initialRoot},
				},
			},
		},
	}).
		WithDriverRegistry(registry).
		WithModule(module).
		Build()
	if err != nil {
		t.Fatalf("build kernel: %v", err)
	}

	ctx := aegis.WithCapabilityRefs(context.Background(), "storage.write:user-uploads")

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, execErr := kernel.Execute(ctx, "upload.blocking.write", struct{}{})
		done <- execErr
	}()

	<-started

	err = kernel.SwapStorageBinding(context.Background(), "user-uploads", aegis.StorageBinding{
		Driver:       "local",
		HotSwappable: true,
		Config:       map[string]any{"root": swappedRoot},
	})
	if err == nil {
		t.Fatalf("expected hot swap to be denied while critical operation is active")
	}
	if !aegis.IsCode(err, aegis.CodeHotSwapDenied) {
		t.Fatalf("expected hot swap denial, got %v", err)
	}

	close(release)
	wg.Wait()
	if execErr := <-done; execErr != nil {
		t.Fatalf("expected blocking operation to finish cleanly, got %v", execErr)
	}
}

func TestIntrospectionVisibilityFilteringAndExposureAwareArtifacts(t *testing.T) {
	module := aegis.NewModuleWithManifest(
		aegis.Manifest{
			Name:        "assistant",
			Description: "Visibility-aware AI operations",
			AI: &aegis.ModuleAISpec{
				Title:   "Assistant operations",
				Summary: "Expose only the operations visible to a caller tier.",
				Skills: []aegis.SkillSpec{
					{
						Name:        "assistant.tools",
						Title:       "Assistant tools",
						Description: "Operations grouped for assistants.",
						Operations: []string{
							"assistant.public",
							"assistant.internal",
							"assistant.restricted",
						},
					},
				},
			},
		},
		[]aegis.Operation{
			aegis.DefineOperation[struct{}, string](aegis.OperationSpec[struct{}, string]{
				Name: "assistant.public",
				AI: &aegis.AIExposureSpec{
					Exposed:         true,
					Exposure:        "public",
					Title:           "Public tool",
					Summary:         "Visible everywhere.",
					InvocationClass: "read",
				},
				Handler: func(ctx context.Context, exec aegis.ExecutionContext, input struct{}) (string, error) {
					return "public", nil
				},
			}),
			aegis.DefineOperation[struct{}, string](aegis.OperationSpec[struct{}, string]{
				Name: "assistant.internal",
				AI: &aegis.AIExposureSpec{
					Exposed:         true,
					Exposure:        "internal",
					Title:           "Internal tool",
					Summary:         "Visible to internal callers.",
					InvocationClass: "read",
				},
				Handler: func(ctx context.Context, exec aegis.ExecutionContext, input struct{}) (string, error) {
					return "internal", nil
				},
			}),
			aegis.DefineOperation[struct{}, string](aegis.OperationSpec[struct{}, string]{
				Name: "assistant.restricted",
				AI: &aegis.AIExposureSpec{
					Exposed:         true,
					Exposure:        "restricted",
					Title:           "Restricted tool",
					Summary:         "Visible only to restricted callers.",
					InvocationClass: "read",
				},
				Handler: func(ctx context.Context, exec aegis.ExecutionContext, input struct{}) (string, error) {
					return "restricted", nil
				},
			}),
			aegis.DefineOperation[struct{}, string](aegis.OperationSpec[struct{}, string]{
				Name: "assistant.hidden",
				AI: &aegis.AIExposureSpec{
					Exposed:         false,
					Exposure:        "internal",
					Title:           "Hidden tool",
					Summary:         "Should never be visible.",
					InvocationClass: "read",
				},
				Handler: func(ctx context.Context, exec aegis.ExecutionContext, input struct{}) (string, error) {
					return "hidden", nil
				},
			}),
		},
	)

	kernel, err := aegis.NewBuilder(aegis.Config{}).
		WithModule(module).
		Build()
	if err != nil {
		t.Fatalf("build kernel: %v", err)
	}

	publicOps, err := kernel.Operations(context.Background(), aegis.IntrospectionFilter{
		AIOnly:         true,
		VisibilityTier: "public",
	})
	if err != nil {
		t.Fatalf("public operations: %v", err)
	}
	if len(publicOps) != 1 || publicOps[0].Name != "assistant.public" {
		t.Fatalf("expected only public operation, got %#v", publicOps)
	}

	internalOps, err := kernel.Operations(context.Background(), aegis.IntrospectionFilter{
		AIOnly:         true,
		VisibilityTier: "internal",
	})
	if err != nil {
		t.Fatalf("internal operations: %v", err)
	}
	if len(internalOps) != 2 {
		t.Fatalf("expected public + internal operations, got %d", len(internalOps))
	}

	publicTools, err := kernel.MCPTools(context.Background(), aegis.IntrospectionFilter{
		VisibilityTier: "public",
	})
	if err != nil {
		t.Fatalf("public MCP tools: %v", err)
	}
	if len(publicTools) != 1 || publicTools[0].Name != "assistant.public" {
		t.Fatalf("expected only public MCP tool, got %#v", publicTools)
	}

	skills, err := kernel.GenerateSkillsMarkdown(context.Background(), aegis.IntrospectionFilter{
		VisibilityTier: "public",
	})
	if err != nil {
		t.Fatalf("public skills markdown: %v", err)
	}
	if !strings.Contains(skills, "assistant.public") {
		t.Fatalf("expected public skill markdown to contain assistant.public")
	}
	if strings.Contains(skills, "assistant.internal") || strings.Contains(skills, "assistant.restricted") {
		t.Fatalf("expected skills markdown to hide non-public operations, got:\n%s", skills)
	}
}

package aegis_test

import (
	"context"
	"strings"
	"testing"

	"aegis"
	"aegis/drivers/localstorage"
)

type inspectInput struct {
	ID string `json:"id"`
}

type writeFileInput struct {
	Path string `json:"path"`
	Data []byte `json:"data"`
}

func TestPolicyConfirmationAndOutputRedaction(t *testing.T) {
	policy := aegis.DefinePolicy(aegis.PolicySpec{
		ID:          "identity.inspect.guard",
		Category:    "access",
		Module:      "identity",
		Description: "requires confirmation and redacts secret output",
		Handler: func(ctx context.Context, in aegis.PolicyContext) (aegis.Decision, error) {
			return aegis.Decision{
				Allowed:   true,
				Reason:    "allowed with guardrails",
				PolicyIDs: []string{"identity.inspect.guard"},
				Confirm:   true,
				Redactions: []aegis.RedactionRule{
					{Path: "$.secret", Mode: "mask"},
				},
			}, nil
		},
	})

	module := aegis.NewModuleWithManifest(
		aegis.Manifest{Name: "identity"},
		[]aegis.Operation{
			aegis.DefineOperation[inspectInput, map[string]any](aegis.OperationSpec[inspectInput, map[string]any]{
				Name: "identity.inspect",
				RequiredPolicies: []aegis.PolicyRef{
					aegis.PolicyID("identity.inspect.guard"),
				},
				Handler: func(ctx context.Context, exec aegis.ExecutionContext, input inspectInput) (map[string]any, error) {
					return map[string]any{
						"id":     input.ID,
						"secret": "top-secret",
					}, nil
				},
			}),
		},
		policy,
	)

	kernel, err := aegis.NewBuilder(aegis.Config{}).
		WithModule(module).
		Build()
	if err != nil {
		t.Fatalf("build kernel: %v", err)
	}

	_, err = kernel.Execute(context.Background(), "identity.inspect", inspectInput{ID: "u-1"})
	if err == nil {
		t.Fatalf("expected confirmation error")
	}
	if !aegis.IsCode(err, aegis.CodeConfirmationNeeded) {
		t.Fatalf("expected confirmation error, got: %v", err)
	}

	ctx := aegis.WithConfirmed(context.Background(), true)
	output, err := kernel.Execute(ctx, "identity.inspect", inspectInput{ID: "u-1"})
	if err != nil {
		t.Fatalf("execute confirmed operation: %v", err)
	}

	decoded, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("expected redacted map output, got %T", output)
	}
	if decoded["secret"] != "[redacted]" {
		t.Fatalf("expected secret to be redacted, got %#v", decoded["secret"])
	}
}

func TestHighAssuranceDeniesUndeclaredMutatingEffect(t *testing.T) {
	tmpDir := t.TempDir()

	registry := aegis.NewDriverRegistry()
	if err := localstorage.Register(registry); err != nil {
		t.Fatalf("register local driver: %v", err)
	}

	module := aegis.NewModule(
		"storage",
		aegis.DefineOperation[writeFileInput, bool](aegis.OperationSpec[writeFileInput, bool]{
			Name: "storage.file.save",
			RequiredCapabilities: []aegis.CapabilityRef{
				"storage.write:user-uploads",
			},
			Handler: func(ctx context.Context, exec aegis.ExecutionContext, input writeFileInput) (bool, error) {
				err := exec.Resources.Storage("user-uploads").Write(ctx, input.Path, input.Data)
				return err == nil, err
			},
		}),
	)

	kernel, err := aegis.NewBuilder(aegis.Config{
		Resources: aegis.ResourcesConfig{
			Storage: map[string]aegis.StorageBinding{
				"user-uploads": {
					Driver: "local",
					Config: map[string]any{"root": tmpDir},
				},
			},
		},
	}).
		WithDriverRegistry(registry).
		WithModule(module).
		WithHighAssuranceEffects(true).
		Build()
	if err != nil {
		t.Fatalf("build kernel: %v", err)
	}

	ctx := aegis.WithCapabilityRefs(context.Background(), "storage.write:user-uploads")
	_, err = kernel.Execute(ctx, "storage.file.save", writeFileInput{
		Path: "hello.txt",
		Data: []byte("world"),
	})
	if err == nil {
		t.Fatalf("expected undeclared effect to be denied")
	}
	if !aegis.IsCode(err, aegis.CodeEffectViolation) {
		t.Fatalf("expected effect violation, got %v", err)
	}
}

func TestIntrospectionSkillsAndMCPTools(t *testing.T) {
	tmpDir := t.TempDir()

	registry := aegis.NewDriverRegistry()
	if err := localstorage.Register(registry); err != nil {
		t.Fatalf("register local driver: %v", err)
	}

	module := aegis.NewModuleWithManifest(
		aegis.Manifest{
			Name:        "storage",
			Version:     "v1",
			Description: "Storage operations",
			AI: &aegis.ModuleAISpec{
				Title:   "Storage operations",
				Summary: "Manage files safely through named resources.",
				Skills: []aegis.SkillSpec{
					{
						Name:        "storage.file_management",
						Title:       "File management",
						Description: "Manage file save and retrieval operations.",
						Operations: []string{
							"storage.file.save",
							"storage.file.get",
						},
					},
				},
			},
		},
		[]aegis.Operation{
			aegis.DefineOperation[writeFileInput, map[string]any](aegis.OperationSpec[writeFileInput, map[string]any]{
				Name: "storage.file.save",
				RequiredCapabilities: []aegis.CapabilityRef{
					"storage.write:user-uploads",
				},
				Effects: []aegis.EffectSpec{
					{
						Name:        "storage.write.user-uploads",
						Kind:        "storage.write",
						RequiredCap: "storage.write:user-uploads",
						Critical:    true,
						Metadata: map[string]any{
							"resource": "user-uploads",
						},
					},
				},
				AI: &aegis.AIExposureSpec{
					Exposed:         true,
					Exposure:        "internal",
					Title:           "Save file",
					Summary:         "Stores a file in the configured uploads resource.",
					InvocationClass: "mutate",
					UseCases:        []string{"a caller needs to save a file"},
					AvoidWhen:       []string{"the caller needs raw driver access"},
					SideEffects:     []string{"writes to the configured storage backend"},
					Tags:            []string{"storage", "files"},
				},
				Idempotent: true,
				Handler: func(ctx context.Context, exec aegis.ExecutionContext, input writeFileInput) (map[string]any, error) {
					err := exec.Resources.Storage("user-uploads").Write(ctx, input.Path, input.Data)
					if err != nil {
						return nil, err
					}
					return map[string]any{"path": input.Path}, nil
				},
			}),
			aegis.DefineOperation[inspectInput, map[string]any](aegis.OperationSpec[inspectInput, map[string]any]{
				Name: "storage.file.get",
				RequiredCapabilities: []aegis.CapabilityRef{
					"storage.read:user-uploads",
				},
				Effects: []aegis.EffectSpec{
					{
						Name:        "storage.read.user-uploads",
						Kind:        "storage.read",
						RequiredCap: "storage.read:user-uploads",
						Metadata: map[string]any{
							"resource": "user-uploads",
						},
					},
				},
				AI: &aegis.AIExposureSpec{
					Exposed:         true,
					Exposure:        "internal",
					Title:           "Get file metadata",
					Summary:         "Returns file metadata from the uploads resource.",
					InvocationClass: "read",
					UseCases:        []string{"a caller needs to fetch stored file metadata"},
				},
				Handler: func(ctx context.Context, exec aegis.ExecutionContext, input inspectInput) (map[string]any, error) {
					return map[string]any{"id": input.ID}, nil
				},
			}),
		},
	)

	kernel, err := aegis.NewBuilder(aegis.Config{
		Resources: aegis.ResourcesConfig{
			Storage: map[string]aegis.StorageBinding{
				"user-uploads": {
					Driver: "local",
					Config: map[string]any{"root": tmpDir},
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

	ctx := aegis.WithCapabilityRefs(context.Background(),
		"storage.write:user-uploads",
		"storage.read:user-uploads",
	)
	ctx = aegis.WithRequestID(ctx, "req-1")
	ctx = aegis.WithTraceID(ctx, "trace-1")

	_, err = kernel.Execute(ctx, "storage.file.save", writeFileInput{
		Path: "hello.txt",
		Data: []byte("world"),
	})
	if err != nil {
		t.Fatalf("execute storage save: %v", err)
	}

	ops, err := kernel.Operations(context.Background(), aegis.IntrospectionFilter{
		AIOnly:         true,
		VisibilityTier: "internal",
	})
	if err != nil {
		t.Fatalf("list operations: %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("expected 2 AI operations, got %d", len(ops))
	}

	tools, err := kernel.MCPTools(context.Background(), aegis.IntrospectionFilter{
		VisibilityTier: "internal",
	})
	if err != nil {
		t.Fatalf("list MCP tools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 MCP tools, got %d", len(tools))
	}

	skills, err := kernel.GenerateSkillsMarkdown(context.Background(), aegis.IntrospectionFilter{
		VisibilityTier: "internal",
	})
	if err != nil {
		t.Fatalf("generate skills markdown: %v", err)
	}
	if !strings.Contains(skills, "storage.file_management") {
		t.Fatalf("expected named skill in output: %s", skills)
	}
	if !strings.Contains(skills, "`storage.file.save`") {
		t.Fatalf("expected save operation in skills output: %s", skills)
	}

	effects, err := kernel.Effects(context.Background(), aegis.EffectQuery{
		Operation: "storage.file.save",
		TraceID:   "trace-1",
	})
	if err != nil {
		t.Fatalf("query effects: %v", err)
	}
	if len(effects) == 0 {
		t.Fatalf("expected recorded effects")
	}

	foundCompleted := false
	for _, effect := range effects {
		if effect.EffectName == "storage.write.user-uploads" && effect.Status == "completed" {
			foundCompleted = true
			break
		}
	}
	if !foundCompleted {
		t.Fatalf("expected completed storage write effect, got %#v", effects)
	}
}

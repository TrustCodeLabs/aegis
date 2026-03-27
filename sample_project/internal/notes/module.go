package notes

import (
	"context"
	"strings"

	"aegis"
)

func BuildModule() aegis.Module {
	readAccessPolicy := aegis.DefinePolicy(aegis.PolicySpec{
		ID:          "notes.read.access",
		Category:    "access",
		Module:      ModuleName,
		Description: "permite leitura para perfis viewer, editor e admin com tenant definido",
		AppliesTo:   []string{OperationList, OperationGet},
		Severity:    "medium",
		Handler: func(ctx context.Context, in aegis.PolicyContext) (aegis.Decision, error) {
			if in.TenantID == "" {
				return aegis.Decision{
					Allowed:   false,
					Reason:    "tenant is required",
					PolicyIDs: []string{"notes.read.access"},
					Severity:  "high",
				}, nil
			}
			if hasAnyRole(in.Subject, "viewer", "editor", "admin") {
				return aegis.Decision{
					Allowed:   true,
					Reason:    "subject may read notes",
					PolicyIDs: []string{"notes.read.access"},
				}, nil
			}
			return aegis.Decision{
				Allowed:   false,
				Reason:    "subject may not read notes",
				PolicyIDs: []string{"notes.read.access"},
				Severity:  "high",
			}, nil
		},
	})

	writeAccessPolicy := aegis.DefinePolicy(aegis.PolicySpec{
		ID:          "notes.write.access",
		Category:    "access",
		Module:      ModuleName,
		Description: "permite escrita para perfis editor e admin com tenant definido",
		AppliesTo:   []string{OperationCreate, OperationUpdate, OperationDelete},
		Severity:    "high",
		Handler: func(ctx context.Context, in aegis.PolicyContext) (aegis.Decision, error) {
			if in.TenantID == "" {
				return aegis.Decision{
					Allowed:   false,
					Reason:    "tenant is required",
					PolicyIDs: []string{"notes.write.access"},
					Severity:  "high",
				}, nil
			}
			if hasAnyRole(in.Subject, "editor", "admin") {
				return aegis.Decision{
					Allowed:   true,
					Reason:    "subject may mutate notes",
					PolicyIDs: []string{"notes.write.access"},
				}, nil
			}
			return aegis.Decision{
				Allowed:   false,
				Reason:    "subject may not mutate notes",
				PolicyIDs: []string{"notes.write.access"},
				Severity:  "high",
			}, nil
		},
	})

	redactionPolicy := aegis.DefinePolicy(aegis.PolicySpec{
		ID:          "notes.read.redaction",
		Category:    "field_visibility",
		Module:      ModuleName,
		Description: "oculta o campo internal para perfis não-admin",
		AppliesTo:   []string{OperationList, OperationGet, OperationCreate, OperationUpdate},
		Severity:    "low",
		Handler: func(ctx context.Context, in aegis.PolicyContext) (aegis.Decision, error) {
			if hasAnyRole(in.Subject, "admin") {
				return aegis.Decision{
					Allowed:   true,
					Reason:    "admins may see internal note fields",
					PolicyIDs: []string{"notes.read.redaction"},
				}, nil
			}

			rules := make([]aegis.RedactionRule, 0, 1)
			switch in.Operation {
			case OperationList:
				rules = append(rules, aegis.RedactionRule{Path: "$.notes.internal", Mode: "mask"})
			case OperationGet, OperationCreate, OperationUpdate:
				rules = append(rules, aegis.RedactionRule{Path: "$.note.internal", Mode: "mask"})
			}

			return aegis.Decision{
				Allowed:    true,
				Reason:     "internal note fields are masked for non-admin subjects",
				PolicyIDs:  []string{"notes.read.redaction"},
				Redactions: rules,
			}, nil
		},
	})

	deleteConfirmationPolicy := aegis.DefinePolicy(aegis.PolicySpec{
		ID:          "notes.delete.confirmation",
		Category:    "effect_permission",
		Module:      ModuleName,
		Description: "exige confirmação explícita para deletar uma nota",
		AppliesTo:   []string{OperationDelete},
		Severity:    "medium",
		Handler: func(ctx context.Context, in aegis.PolicyContext) (aegis.Decision, error) {
			return aegis.Decision{
				Allowed:   true,
				Reason:    "delete requires explicit confirmation",
				PolicyIDs: []string{"notes.delete.confirmation"},
				Confirm:   true,
				Obligations: []aegis.Obligation{
					{
						Type: "require_confirmation",
						Params: map[string]any{
							"source": "http_header",
						},
					},
				},
			}, nil
		},
	})

	manifest := aegis.Manifest{
		Name:        ModuleName,
		Version:     "v1",
		Status:      "active",
		Description: "Notes API backed by Aegis storage resources, policies and introspection.",
		AI: &aegis.ModuleAISpec{
			Title:       "Notes operations",
			Summary:     "Create, list, inspect, update and delete notes for workspace tenants.",
			IntendedFor: []string{"internal support agents", "workspace assistants", "operators"},
			Skills: []aegis.SkillSpec{
				{
					Name:        "notes_management",
					Title:       "Notes management",
					Description: "Manage note creation, lookup, update and deletion for a tenant.",
					Operations: []string{
						OperationCreate,
						OperationList,
						OperationGet,
						OperationUpdate,
						OperationDelete,
					},
				},
			},
		},
	}

	operations := []aegis.Operation{
		aegis.DefineOperation[CreateInput, Output](aegis.OperationSpec[CreateInput, Output]{
			Name: OperationCreate,
			RequiredCapabilities: []aegis.CapabilityRef{
				"storage.read:" + ResourceName,
				"storage.write:" + ResourceName,
			},
			RequiredPolicies: []aegis.PolicyRef{
				aegis.PolicyID("notes.write.access"),
				aegis.PolicyID("notes.read.redaction"),
			},
			Effects:    readWriteEffects(),
			AI:         createAI(),
			Idempotent: false,
			Validate:   ValidateCreateInput,
			Handler: func(ctx context.Context, exec aegis.ExecutionContext, input CreateInput) (Output, error) {
				repo := NewRepository(exec.Resources.Storage(ResourceName), exec.TenantID)
				return repo.Create(ctx, input)
			},
		}),
		aegis.DefineOperation[ListInput, ListOutput](aegis.OperationSpec[ListInput, ListOutput]{
			Name: OperationList,
			RequiredCapabilities: []aegis.CapabilityRef{
				"storage.read:" + ResourceName,
			},
			RequiredPolicies: []aegis.PolicyRef{
				aegis.PolicyID("notes.read.access"),
				aegis.PolicyID("notes.read.redaction"),
			},
			Effects:       readEffects(),
			AI:            listAI(),
			Idempotent:    true,
			Deterministic: true,
			Handler: func(ctx context.Context, exec aegis.ExecutionContext, input ListInput) (ListOutput, error) {
				repo := NewRepository(exec.Resources.Storage(ResourceName), exec.TenantID)
				return repo.List(ctx)
			},
		}),
		aegis.DefineOperation[LookupInput, Output](aegis.OperationSpec[LookupInput, Output]{
			Name: OperationGet,
			RequiredCapabilities: []aegis.CapabilityRef{
				"storage.read:" + ResourceName,
			},
			RequiredPolicies: []aegis.PolicyRef{
				aegis.PolicyID("notes.read.access"),
				aegis.PolicyID("notes.read.redaction"),
			},
			Effects:       readEffects(),
			AI:            getAI(),
			Idempotent:    true,
			Deterministic: true,
			Validate:      ValidateLookupInput,
			Handler: func(ctx context.Context, exec aegis.ExecutionContext, input LookupInput) (Output, error) {
				repo := NewRepository(exec.Resources.Storage(ResourceName), exec.TenantID)
				return repo.Get(ctx, input.ID)
			},
		}),
		aegis.DefineOperation[UpdateInput, Output](aegis.OperationSpec[UpdateInput, Output]{
			Name: OperationUpdate,
			RequiredCapabilities: []aegis.CapabilityRef{
				"storage.read:" + ResourceName,
				"storage.write:" + ResourceName,
			},
			RequiredPolicies: []aegis.PolicyRef{
				aegis.PolicyID("notes.write.access"),
				aegis.PolicyID("notes.read.redaction"),
			},
			Effects:  readWriteEffects(),
			AI:       updateAI(),
			Validate: ValidateUpdateInput,
			Handler: func(ctx context.Context, exec aegis.ExecutionContext, input UpdateInput) (Output, error) {
				repo := NewRepository(exec.Resources.Storage(ResourceName), exec.TenantID)
				return repo.Update(ctx, input)
			},
		}),
		aegis.DefineOperation[LookupInput, DeleteOutput](aegis.OperationSpec[LookupInput, DeleteOutput]{
			Name: OperationDelete,
			RequiredCapabilities: []aegis.CapabilityRef{
				"storage.read:" + ResourceName,
				"storage.write:" + ResourceName,
			},
			RequiredPolicies: []aegis.PolicyRef{
				aegis.PolicyID("notes.write.access"),
				aegis.PolicyID("notes.delete.confirmation"),
			},
			Effects:  deleteEffects(),
			AI:       deleteAI(),
			Validate: ValidateLookupInput,
			Handler: func(ctx context.Context, exec aegis.ExecutionContext, input LookupInput) (DeleteOutput, error) {
				repo := NewRepository(exec.Resources.Storage(ResourceName), exec.TenantID)
				return repo.Delete(ctx, input.ID)
			},
		}),
	}

	return aegis.NewModuleWithManifest(
		manifest,
		operations,
		readAccessPolicy,
		writeAccessPolicy,
		redactionPolicy,
		deleteConfirmationPolicy,
	)
}

func CapabilitiesForRole(role string) []aegis.CapabilityRef {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "admin", "editor":
		return []aegis.CapabilityRef{
			"storage.read:" + ResourceName,
			"storage.write:" + ResourceName,
		}
	case "viewer":
		return []aegis.CapabilityRef{
			"storage.read:" + ResourceName,
		}
	default:
		return []aegis.CapabilityRef{
			"storage.read:" + ResourceName,
		}
	}
}

func hasAnyRole(subject aegis.Subject, expected ...string) bool {
	for _, role := range subject.Roles {
		for _, candidate := range expected {
			if strings.EqualFold(role, candidate) {
				return true
			}
		}
	}
	return false
}

func createAI() *aegis.AIExposureSpec {
	return &aegis.AIExposureSpec{
		Exposed:              true,
		Exposure:             "internal",
		Title:                "Create note",
		Summary:              "Creates a note for the current tenant and updates the note index.",
		Description:          "Use this when a workspace actor needs to create a new persistent note. This writes the note payload and updates the tenant-scoped index.",
		UseCases:             []string{"a caller needs to register a new note", "a workspace flow needs tenant-scoped persistent memory"},
		AvoidWhen:            []string{"the caller only needs to inspect an existing note", "the action should not mutate tenant data"},
		InvocationClass:      "mutate",
		RequiresConfirmation: false,
		SideEffects:          []string{"writes note data to storage", "updates the note index"},
		Tags:                 []string{"notes", "storage", "tenant"},
	}
}

func listAI() *aegis.AIExposureSpec {
	return &aegis.AIExposureSpec{
		Exposed:         true,
		Exposure:        "internal",
		Title:           "List notes",
		Summary:         "Lists tenant-scoped notes ordered by most recent update.",
		Description:     "Use this when a caller needs a quick view of the notes available in the current tenant workspace.",
		UseCases:        []string{"a caller needs to browse tenant notes", "an assistant needs note identifiers before fetching details"},
		AvoidWhen:       []string{"the caller already knows the target note id", "the action should create or mutate state"},
		InvocationClass: "read",
		SideEffects:     []string{"reads the tenant note index from storage"},
		Tags:            []string{"notes", "browse", "tenant"},
	}
}

func getAI() *aegis.AIExposureSpec {
	return &aegis.AIExposureSpec{
		Exposed:         true,
		Exposure:        "internal",
		Title:           "Get note",
		Summary:         "Fetches a single note by id from the current tenant.",
		Description:     "Use this when a caller already knows the note identifier and needs the full content.",
		UseCases:        []string{"a caller needs one specific note", "a follow-up step needs note content after listing ids"},
		AvoidWhen:       []string{"the note id is unknown", "the action should browse across multiple notes"},
		InvocationClass: "read",
		SideEffects:     []string{"reads note data from storage"},
		Tags:            []string{"notes", "lookup", "tenant"},
	}
}

func updateAI() *aegis.AIExposureSpec {
	return &aegis.AIExposureSpec{
		Exposed:         true,
		Exposure:        "internal",
		Title:           "Update note",
		Summary:         "Updates an existing tenant note and refreshes the note index.",
		Description:     "Use this when a caller needs to change the content or metadata of an existing note.",
		UseCases:        []string{"a caller needs to edit a note", "a workflow must correct or enrich note content"},
		AvoidWhen:       []string{"the note does not exist", "the action should be append-only"},
		InvocationClass: "mutate",
		SideEffects:     []string{"reads note data from storage", "writes updated note data", "updates the note index"},
		Tags:            []string{"notes", "edit", "tenant"},
	}
}

func deleteAI() *aegis.AIExposureSpec {
	return &aegis.AIExposureSpec{
		Exposed:              true,
		Exposure:             "internal",
		Title:                "Delete note",
		Summary:              "Deletes a note for the current tenant after explicit confirmation.",
		Description:          "Use this when a caller intentionally wants to remove a note. Confirmation is required because the action is destructive.",
		UseCases:             []string{"a caller wants to remove an obsolete note", "a workspace clean-up flow must delete known data"},
		AvoidWhen:            []string{"the caller is unsure which note should be deleted", "the action should preserve historical content"},
		InvocationClass:      "mutate",
		RequiresConfirmation: true,
		SideEffects:          []string{"reads note data", "deletes note storage", "updates the note index"},
		Tags:                 []string{"notes", "delete", "dangerous"},
	}
}

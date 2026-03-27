package notes

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aegis"
	"aegis/drivers/localstorage"
)

func TestRepositoryCRUDAndModuleShape(t *testing.T) {
	driver, err := localstorage.New(t.TempDir())
	if err != nil {
		t.Fatalf("create driver: %v", err)
	}

	repo := NewRepository(driver, "team-a")

	created, err := repo.Create(context.Background(), CreateInput{
		ID:       "note-1",
		Title:    "First note",
		Content:  "hello world",
		Internal: "internal field",
	})
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if created.Note.TenantID != "team-a" {
		t.Fatalf("expected tenant team-a, got %q", created.Note.TenantID)
	}

	listed, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("list notes: %v", err)
	}
	if len(listed.Notes) != 1 || listed.Notes[0].ID != "note-1" {
		t.Fatalf("unexpected list output: %#v", listed.Notes)
	}

	fetched, err := repo.Get(context.Background(), "note-1")
	if err != nil {
		t.Fatalf("get note: %v", err)
	}
	if fetched.Note.Title != "First note" {
		t.Fatalf("unexpected note title: %q", fetched.Note.Title)
	}

	updated, err := repo.Update(context.Background(), UpdateInput{
		ID:       "note-1",
		Title:    "Updated note",
		Content:  "updated body",
		Internal: "changed",
	})
	if err != nil {
		t.Fatalf("update note: %v", err)
	}
	if updated.Note.Title != "Updated note" {
		t.Fatalf("unexpected updated title: %q", updated.Note.Title)
	}

	deleted, err := repo.Delete(context.Background(), "note-1")
	if err != nil {
		t.Fatalf("delete note: %v", err)
	}
	if !deleted.Deleted {
		t.Fatalf("expected deleted flag")
	}

	afterDelete, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(afterDelete.Notes) != 0 {
		t.Fatalf("expected empty note index after delete, got %#v", afterDelete.Notes)
	}

	module := BuildModule()
	if module.Manifest().Name != ModuleName {
		t.Fatalf("unexpected module name: %q", module.Manifest().Name)
	}
	if len(module.Operations()) != 5 {
		t.Fatalf("expected 5 operations, got %d", len(module.Operations()))
	}
	if len(module.Policies()) != 4 {
		t.Fatalf("expected 4 policies, got %d", len(module.Policies()))
	}

	if len(readEffects()) != 1 || len(readWriteEffects()) != 2 || len(deleteEffects()) != 3 {
		t.Fatalf("unexpected effects declaration sizes")
	}
}

func TestValidationAndCapabilitiesForRole(t *testing.T) {
	cases := []struct {
		name    string
		check   func() error
		wantErr bool
	}{
		{
			name: "valid create",
			check: func() error {
				return ValidateCreateInput(CreateInput{Title: "ok", Content: "ok"})
			},
		},
		{
			name: "invalid create title",
			check: func() error {
				return ValidateCreateInput(CreateInput{Content: "ok"})
			},
			wantErr: true,
		},
		{
			name: "invalid update id",
			check: func() error {
				return ValidateUpdateInput(UpdateInput{ID: "../bad", Title: "ok", Content: "ok"})
			},
			wantErr: true,
		},
		{
			name: "invalid update title",
			check: func() error {
				return ValidateUpdateInput(UpdateInput{ID: "safe-id", Content: "ok"})
			},
			wantErr: true,
		},
		{
			name: "invalid update content",
			check: func() error {
				return ValidateUpdateInput(UpdateInput{ID: "safe-id", Title: "ok"})
			},
			wantErr: true,
		},
		{
			name: "valid lookup",
			check: func() error {
				return ValidateLookupInput(LookupInput{ID: "safe-id"})
			},
		},
		{
			name: "invalid lookup",
			check: func() error {
				return ValidateLookupInput(LookupInput{ID: "unsafe/id"})
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.check()
			if tc.wantErr && err == nil {
				t.Fatalf("expected validation error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}

	editorCaps := CapabilitiesForRole("editor")
	if len(editorCaps) != 2 {
		t.Fatalf("expected editor to have read/write capabilities, got %#v", editorCaps)
	}

	viewerCaps := CapabilitiesForRole("viewer")
	if len(viewerCaps) != 1 || viewerCaps[0] != aegis.CapabilityRef("storage.read:"+ResourceName) {
		t.Fatalf("unexpected viewer capabilities: %#v", viewerCaps)
	}

	if !hasAnyRole(aegis.Subject{Roles: []string{"Admin"}}, "admin") {
		t.Fatalf("expected role matching to be case-insensitive")
	}
	if hasAnyRole(aegis.Subject{Roles: []string{"viewer"}}, "admin") {
		t.Fatalf("expected unrelated roles not to match")
	}

	generated := generateID("note")
	if !strings.HasPrefix(generated, "note-") {
		t.Fatalf("unexpected generated id: %q", generated)
	}
	if len(CapabilitiesForRole("unknown")) != 1 {
		t.Fatalf("expected default capabilities for unknown role")
	}
}

func TestIndexHelpers(t *testing.T) {
	now := time.Now().UTC()
	index := Index{
		Notes: []Summary{
			{ID: "older", UpdatedAt: now.Add(-time.Minute)},
			{ID: "newer", UpdatedAt: now},
		},
	}

	if !noteExists(index, "older") {
		t.Fatalf("expected noteExists to find existing id")
	}
	if noteExists(index, "missing") {
		t.Fatalf("expected noteExists to return false for missing id")
	}

	updated := upsertSummary(index.Notes, Summary{ID: "older", Title: "updated", UpdatedAt: now.Add(time.Minute)})
	sortIndex(updated)
	if updated[0].ID != "older" {
		t.Fatalf("expected updated summary to move to the front, got %#v", updated)
	}

	removed := removeSummary(updated, "newer")
	if len(removed) != 1 || removed[0].ID != "older" {
		t.Fatalf("unexpected removeSummary output: %#v", removed)
	}
}

func TestModuleOperationsPoliciesAndRedactions(t *testing.T) {
	registry := aegis.NewDriverRegistry()
	if err := localstorage.Register(registry); err != nil {
		t.Fatalf("register local driver: %v", err)
	}

	kernel, err := aegis.NewBuilder(aegis.Config{
		Resources: aegis.ResourcesConfig{
			Storage: map[string]aegis.StorageBinding{
				ResourceName: {
					Tenant: map[string]aegis.StorageBinding{
						"team-a": {
							Driver: "local",
							Config: map[string]any{"root": t.TempDir()},
						},
						"team-b": {
							Driver: "local",
							Config: map[string]any{"root": t.TempDir()},
						},
					},
				},
			},
		},
	}).
		WithDriverRegistry(registry).
		WithModule(BuildModule()).
		WithHighAssuranceEffects(true).
		Build()
	if err != nil {
		t.Fatalf("build notes kernel: %v", err)
	}

	execCtx := func(role, tenant string, confirmed bool) context.Context {
		caps := CapabilitiesForRole(role)
		ctx := aegis.WithSubject(context.Background(), aegis.Subject{
			ID:           "user-" + role,
			Type:         "user",
			Roles:        []string{role},
			Capabilities: caps,
		})
		ctx = aegis.WithCapabilities(ctx, aegis.NewGrantedCapabilities(caps...))
		if tenant != "" {
			ctx = aegis.WithTenantID(ctx, tenant)
		}
		if confirmed {
			ctx = aegis.WithConfirmed(ctx, true)
		}
		return ctx
	}

	createRaw, err := kernel.Execute(execCtx("editor", "team-a", false), OperationCreate, CreateInput{
		ID:       "module-note-1",
		Title:    "Created by module",
		Content:  "body",
		Internal: "editor secret",
	})
	if err != nil {
		t.Fatalf("create through module: %v", err)
	}
	createOut := mustDecodeMap(t, createRaw)
	if note, ok := createOut["note"].(map[string]any); !ok || note["internal"] != "[redacted]" {
		t.Fatalf("expected non-admin create output to be redacted, got %#v", createOut)
	}

	adminGetRaw, err := kernel.Execute(execCtx("admin", "team-a", false), OperationGet, LookupInput{ID: "module-note-1"})
	if err != nil {
		t.Fatalf("admin get through module: %v", err)
	}
	adminGet := adminGetRaw.(Output)
	if adminGet.Note.Internal != "editor secret" {
		t.Fatalf("expected admin to see unredacted field, got %#v", adminGet.Note)
	}

	listRaw, err := kernel.Execute(execCtx("viewer", "team-a", false), OperationList, ListInput{})
	if err != nil {
		t.Fatalf("viewer list through module: %v", err)
	}
	listOut := mustDecodeMap(t, listRaw)
	notes, ok := listOut["notes"].([]any)
	if !ok || len(notes) != 1 {
		t.Fatalf("expected viewer list to include one note, got %#v", listOut)
	}
	firstNote, ok := notes[0].(map[string]any)
	if !ok || firstNote["internal"] != "[redacted]" {
		t.Fatalf("expected viewer list to redact internal field, got %#v", listOut)
	}

	_, err = kernel.Execute(execCtx("viewer", "team-a", false), OperationUpdate, UpdateInput{
		ID:       "module-note-1",
		Title:    "viewer update",
		Content:  "should fail",
		Internal: "blocked",
	})
	if !aegis.IsCode(err, aegis.CodeCapabilityDenied) {
		t.Fatalf("expected viewer update to be denied by capability, got %v", err)
	}

	policyDenyCtx := aegis.WithSubject(context.Background(), aegis.Subject{
		ID:           "user-guest",
		Type:         "user",
		Roles:        []string{"guest"},
		Capabilities: []aegis.CapabilityRef{"storage.read:" + ResourceName, "storage.write:" + ResourceName},
	})
	policyDenyCtx = aegis.WithCapabilities(policyDenyCtx, aegis.NewGrantedCapabilities(
		"storage.read:"+ResourceName,
		"storage.write:"+ResourceName,
	))
	policyDenyCtx = aegis.WithTenantID(policyDenyCtx, "team-a")

	_, err = kernel.Execute(policyDenyCtx, OperationUpdate, UpdateInput{
		ID:       "module-note-1",
		Title:    "guest update",
		Content:  "should still fail",
		Internal: "blocked",
	})
	if !aegis.IsCode(err, aegis.CodePolicyDenied) {
		t.Fatalf("expected guest update to be denied by policy, got %v", err)
	}

	updateRaw, err := kernel.Execute(execCtx("editor", "team-a", false), OperationUpdate, UpdateInput{
		ID:       "module-note-1",
		Title:    "Updated by editor",
		Content:  "updated body",
		Internal: "updated secret",
	})
	if err != nil {
		t.Fatalf("editor update through module: %v", err)
	}
	updateOut := mustDecodeMap(t, updateRaw)
	if note, ok := updateOut["note"].(map[string]any); !ok || note["internal"] != "[redacted]" {
		t.Fatalf("expected non-admin update output to be redacted, got %#v", updateOut)
	}

	_, err = kernel.Execute(execCtx("editor", "team-a", false), OperationDelete, LookupInput{ID: "module-note-1"})
	if !aegis.IsCode(err, aegis.CodeConfirmationNeeded) {
		t.Fatalf("expected delete without confirmation to fail, got %v", err)
	}

	deleteRaw, err := kernel.Execute(execCtx("editor", "team-a", true), OperationDelete, LookupInput{ID: "module-note-1"})
	if err != nil {
		t.Fatalf("delete through module: %v", err)
	}
	deleteOut := deleteRaw.(DeleteOutput)
	if !deleteOut.Deleted {
		t.Fatalf("expected delete operation to succeed")
	}

	if _, err := kernel.Execute(execCtx("viewer", "", false), OperationList, ListInput{}); !aegis.IsCode(err, aegis.CodePolicyDenied) {
		t.Fatalf("expected tenant-less list to be denied, got %v", err)
	}

	if _, err := kernel.Execute(execCtx("editor", "team-b", false), OperationCreate, CreateInput{
		ID:      "tenant-b-note",
		Title:   "Tenant B",
		Content: "isolated",
	}); err != nil {
		t.Fatalf("create tenant-b note: %v", err)
	}

	isolatedRaw, err := kernel.Execute(execCtx("viewer", "team-a", false), OperationList, ListInput{})
	if err != nil {
		t.Fatalf("list team-a after tenant-b create: %v", err)
	}
	isolated := mustDecodeMap(t, isolatedRaw)
	isolatedNotes, ok := isolated["notes"].([]any)
	if !ok {
		t.Fatalf("expected notes list in response, got %#v", isolated)
	}
	if len(isolatedNotes) != 0 {
		t.Fatalf("expected tenant isolation between team-a and team-b, got %#v", isolatedNotes)
	}
}

func mustDecodeMap(t *testing.T, value any) map[string]any {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal value: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode map value: %v", err)
	}
	return decoded
}

package notes

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"aegis"
)

type Repository struct {
	storage  aegis.StorageResource
	tenantID string
}

func NewRepository(storage aegis.StorageResource, tenantID string) Repository {
	return Repository{
		storage:  storage,
		tenantID: tenantID,
	}
}

func (r Repository) Create(ctx context.Context, input CreateInput) (Output, error) {
	index, err := loadIndex(ctx, r.storage)
	if err != nil {
		return Output{}, err
	}

	noteID := strings.TrimSpace(input.ID)
	if noteID == "" {
		noteID = generateID("note")
	}
	if noteExists(index, noteID) {
		return Output{}, fmt.Errorf("note %q already exists", noteID)
	}

	now := time.Now().UTC()
	created := Note{
		ID:        noteID,
		TenantID:  r.tenantID,
		Title:     strings.TrimSpace(input.Title),
		Content:   strings.TrimSpace(input.Content),
		Internal:  strings.TrimSpace(input.Internal),
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := writeJSON(ctx, r.storage, noteFilePath(noteID), created); err != nil {
		return Output{}, err
	}

	index.Notes = append(index.Notes, summarize(created))
	sortIndex(index.Notes)
	if err := writeJSON(ctx, r.storage, noteIndexPath(), index); err != nil {
		return Output{}, err
	}

	return Output{Note: created}, nil
}

func (r Repository) List(ctx context.Context) (ListOutput, error) {
	index, err := loadIndex(ctx, r.storage)
	if err != nil {
		return ListOutput{}, err
	}
	sortIndex(index.Notes)
	return ListOutput{Notes: index.Notes}, nil
}

func (r Repository) Get(ctx context.Context, id string) (Output, error) {
	stored, err := loadNote(ctx, r.storage, id)
	if err != nil {
		return Output{}, err
	}
	return Output{Note: stored}, nil
}

func (r Repository) Update(ctx context.Context, input UpdateInput) (Output, error) {
	existing, err := loadNote(ctx, r.storage, input.ID)
	if err != nil {
		return Output{}, err
	}

	existing.Title = strings.TrimSpace(input.Title)
	existing.Content = strings.TrimSpace(input.Content)
	existing.Internal = strings.TrimSpace(input.Internal)
	existing.UpdatedAt = time.Now().UTC()

	if err := writeJSON(ctx, r.storage, noteFilePath(existing.ID), existing); err != nil {
		return Output{}, err
	}

	index, err := loadIndex(ctx, r.storage)
	if err != nil {
		return Output{}, err
	}
	index.Notes = upsertSummary(index.Notes, summarize(existing))
	sortIndex(index.Notes)
	if err := writeJSON(ctx, r.storage, noteIndexPath(), index); err != nil {
		return Output{}, err
	}

	return Output{Note: existing}, nil
}

func (r Repository) Delete(ctx context.Context, id string) (DeleteOutput, error) {
	if _, err := loadNote(ctx, r.storage, id); err != nil {
		return DeleteOutput{}, err
	}
	if err := r.storage.Delete(ctx, noteFilePath(id)); err != nil {
		return DeleteOutput{}, err
	}

	index, err := loadIndex(ctx, r.storage)
	if err != nil {
		return DeleteOutput{}, err
	}
	index.Notes = removeSummary(index.Notes, id)
	if err := writeJSON(ctx, r.storage, noteIndexPath(), index); err != nil {
		return DeleteOutput{}, err
	}

	return DeleteOutput{Deleted: true, ID: id}, nil
}

func ValidateCreateInput(input CreateInput) error {
	if strings.TrimSpace(input.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if strings.TrimSpace(input.Content) == "" {
		return fmt.Errorf("content is required")
	}
	if input.ID != "" && !isSafeNoteID(input.ID) {
		return fmt.Errorf("id contains unsupported characters")
	}
	return nil
}

func ValidateUpdateInput(input UpdateInput) error {
	if !isSafeNoteID(input.ID) {
		return fmt.Errorf("id contains unsupported characters")
	}
	if strings.TrimSpace(input.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if strings.TrimSpace(input.Content) == "" {
		return fmt.Errorf("content is required")
	}
	return nil
}

func ValidateLookupInput(input LookupInput) error {
	if !isSafeNoteID(input.ID) {
		return fmt.Errorf("id contains unsupported characters")
	}
	return nil
}

func loadIndex(ctx context.Context, storage aegis.StorageResource) (Index, error) {
	var index Index
	err := readJSON(ctx, storage, noteIndexPath(), &index)
	if err != nil {
		if aegis.IsNotFoundError(err) || aegis.IsCode(err, aegis.CodeResourceNotFound) {
			return Index{Notes: []Summary{}}, nil
		}
		return Index{}, err
	}
	if index.Notes == nil {
		index.Notes = []Summary{}
	}
	return index, nil
}

func loadNote(ctx context.Context, storage aegis.StorageResource, id string) (Note, error) {
	var out Note
	if err := readJSON(ctx, storage, noteFilePath(id), &out); err != nil {
		return Note{}, err
	}
	return out, nil
}

func readJSON(ctx context.Context, storage aegis.StorageResource, path string, out any) error {
	data, err := storage.Read(ctx, path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func writeJSON(ctx context.Context, storage aegis.StorageResource, path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return storage.Write(ctx, path, data)
}

func noteExists(index Index, id string) bool {
	for _, item := range index.Notes {
		if item.ID == id {
			return true
		}
	}
	return false
}

func upsertSummary(summaries []Summary, summary Summary) []Summary {
	for index, item := range summaries {
		if item.ID == summary.ID {
			summaries[index] = summary
			return summaries
		}
	}
	return append(summaries, summary)
}

func removeSummary(summaries []Summary, id string) []Summary {
	filtered := make([]Summary, 0, len(summaries))
	for _, item := range summaries {
		if item.ID == id {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func summarize(n Note) Summary {
	return Summary{
		ID:        n.ID,
		Title:     n.Title,
		Internal:  n.Internal,
		CreatedAt: n.CreatedAt,
		UpdatedAt: n.UpdatedAt,
	}
}

func sortIndex(notes []Summary) {
	sort.Slice(notes, func(i, j int) bool {
		return notes[i].UpdatedAt.After(notes[j].UpdatedAt)
	})
}

func noteIndexPath() string {
	return "notes/index.json"
}

func noteFilePath(id string) string {
	return filepath.ToSlash(filepath.Join("notes", id+".json"))
}

func isSafeNoteID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, char := range id {
		switch {
		case char >= 'a' && char <= 'z':
		case char >= 'A' && char <= 'Z':
		case char >= '0' && char <= '9':
		case char == '-', char == '_':
		default:
			return false
		}
	}
	return true
}

func generateID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
}

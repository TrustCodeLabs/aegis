package aegis

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type EffectSpec struct {
	Name        string
	Kind        string
	RequiredCap CapabilityRef
	Critical    bool
	Idempotent  bool
	Optional    bool
	Metadata    map[string]any
	Policies    []PolicyRef
	Consistency string
	Declared    bool
}

type EffectSpecInfo struct {
	Name       string
	Kind       string
	Critical   bool
	Idempotent bool
	Optional   bool
}

type EffectRecord struct {
	ID         string
	Operation  string
	Module     string
	EffectName string
	EffectKind string
	SubjectID  string
	TenantID   string
	Resource   ResourceRef
	Capability string
	Declared   bool
	Allowed    bool
	Critical   bool
	Idempotent bool
	Status     string
	ErrorCode  string
	StartedAt  time.Time
	FinishedAt time.Time
	TraceID    string
	RequestID  string
	Metadata   map[string]any
}

type EffectQuery struct {
	Module    string
	Operation string
	TenantID  string
	TraceID   string
	Status    string
	Since     *time.Time
	Until     *time.Time
}

type EffectTracker interface {
	Plan(exec ExecutionContext, spec EffectSpec) error
	Before(exec ExecutionContext, spec EffectSpec, resource ResourceRef) (EffectRecord, error)
	After(rec EffectRecord, err error) error
	Records(exec ExecutionContext) []EffectRecord
}

type MemoryEffectStore struct {
	mu      sync.RWMutex
	records []EffectRecord
}

func NewMemoryEffectStore() *MemoryEffectStore {
	return &MemoryEffectStore{}
}

func (s *MemoryEffectStore) Append(records []EffectRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, record := range records {
		s.records = append(s.records, cloneEffectRecord(record))
	}
	return nil
}

func (s *MemoryEffectStore) Query(filter EffectQuery) ([]EffectRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]EffectRecord, 0, len(s.records))
	for _, record := range s.records {
		if filter.Module != "" && record.Module != filter.Module {
			continue
		}
		if filter.Operation != "" && record.Operation != filter.Operation {
			continue
		}
		if filter.TenantID != "" && record.TenantID != filter.TenantID {
			continue
		}
		if filter.TraceID != "" && record.TraceID != filter.TraceID {
			continue
		}
		if filter.Status != "" && record.Status != filter.Status {
			continue
		}
		if filter.Since != nil && record.StartedAt.Before(*filter.Since) {
			continue
		}
		if filter.Until != nil && record.StartedAt.After(*filter.Until) {
			continue
		}
		out = append(out, cloneEffectRecord(record))
	}
	return out, nil
}

type executionEffectTracker struct {
	store          *MemoryEffectStore
	policy         PolicyEngine
	highAssurance  bool
	records        []EffectRecord
	recordIndex    map[string]int
	recordCounter  atomic.Uint64
	plannedEffects map[string]EffectSpec
}

func newExecutionEffectTracker(store *MemoryEffectStore, policy PolicyEngine, highAssurance bool) *executionEffectTracker {
	if store == nil {
		store = NewMemoryEffectStore()
	}
	return &executionEffectTracker{
		store:          store,
		policy:         policy,
		highAssurance:  highAssurance,
		recordIndex:    map[string]int{},
		plannedEffects: map[string]EffectSpec{},
	}
}

func (t *executionEffectTracker) Plan(exec ExecutionContext, spec EffectSpec) error {
	spec = normalizedEffectSpec(spec)
	record := t.newRecord(exec, spec, ResourceRef{}, "planned")
	t.append(record)
	t.plannedEffects[spec.Name] = spec
	return nil
}

func (t *executionEffectTracker) Before(exec ExecutionContext, spec EffectSpec, resource ResourceRef) (EffectRecord, error) {
	spec = normalizedEffectSpec(spec)
	if _, ok := t.plannedEffects[spec.Name]; ok {
		spec.Declared = true
	}

	record := t.newRecord(exec, spec, resource, "attempted")

	if spec.RequiredCap != "" && !exec.Capabilities.Has(spec.RequiredCap) {
		record.Allowed = false
		record.Status = "denied"
		record.ErrorCode = CodeCapabilityDenied
		record.FinishedAt = time.Now().UTC()
		t.append(record)
		return record, newKernelError(
			CodeCapabilityDenied,
			fmt.Sprintf("effect %q requires capability %q", spec.Name, spec.RequiredCap),
			nil,
		)
	}

	if !spec.Declared && t.highAssurance && (spec.Critical || strings.HasSuffix(spec.Kind, ".write") || strings.HasPrefix(spec.Kind, "storage.write")) {
		record.Allowed = false
		record.Status = "denied"
		record.ErrorCode = CodeEffectViolation
		record.FinishedAt = time.Now().UTC()
		t.append(record)
		return record, newKernelError(
			CodeEffectViolation,
			fmt.Sprintf("undeclared effect %q denied in high-assurance mode", spec.Name),
			nil,
		)
	}

	if len(spec.Policies) > 0 && t.policy != nil {
		decision, err := t.policy.Evaluate(context.Background(), policyContextFor(exec, nil, resource), spec.Policies)
		if err != nil {
			return record, err
		}
		if !decision.Allowed {
			record.Allowed = false
			record.Status = "denied"
			record.ErrorCode = CodeEffectDenied
			record.FinishedAt = time.Now().UTC()
			t.append(record)
			return record, newKernelError(CodeEffectDenied, decision.Reason, nil)
		}
	}

	record.Allowed = true
	record.Status = "allowed"
	t.append(record)
	return record, nil
}

func (t *executionEffectTracker) After(rec EffectRecord, err error) error {
	index, ok := t.recordIndex[rec.ID]
	if !ok {
		return nil
	}

	current := t.records[index]
	current.FinishedAt = time.Now().UTC()
	if err != nil {
		current.Status = "failed"
		if kernelErr, ok := err.(*KernelError); ok {
			current.ErrorCode = kernelErr.Code
		}
		t.records[index] = current
		return nil
	}

	current.Status = "completed"
	t.records[index] = current
	return nil
}

func (t *executionEffectTracker) Records(exec ExecutionContext) []EffectRecord {
	out := make([]EffectRecord, len(t.records))
	for index, record := range t.records {
		out[index] = cloneEffectRecord(record)
	}
	return out
}

func (t *executionEffectTracker) Flush() error {
	return t.store.Append(t.records)
}

func (t *executionEffectTracker) append(record EffectRecord) {
	t.recordIndex[record.ID] = len(t.records)
	t.records = append(t.records, cloneEffectRecord(record))
}

func (t *executionEffectTracker) newRecord(exec ExecutionContext, spec EffectSpec, resource ResourceRef, status string) EffectRecord {
	id := fmt.Sprintf("%s:%d", spec.Name, t.recordCounter.Add(1))
	now := time.Now().UTC()
	return EffectRecord{
		ID:         id,
		Operation:  exec.Operation,
		Module:     exec.Module,
		EffectName: spec.Name,
		EffectKind: spec.Kind,
		SubjectID:  exec.Subject.ID,
		TenantID:   exec.TenantID,
		Resource: ResourceRef{
			Kind:       resource.Kind,
			ID:         resource.ID,
			Module:     resource.Module,
			TenantID:   resource.TenantID,
			Attributes: cloneMap(resource.Attributes),
		},
		Capability: string(spec.RequiredCap),
		Declared:   spec.Declared,
		Allowed:    false,
		Critical:   spec.Critical,
		Idempotent: spec.Idempotent,
		Status:     status,
		StartedAt:  now,
		FinishedAt: now,
		TraceID:    exec.TraceID,
		RequestID:  exec.RequestID,
		Metadata:   cloneMap(spec.Metadata),
	}
}

func normalizedEffectSpec(spec EffectSpec) EffectSpec {
	spec.Metadata = cloneMap(spec.Metadata)
	spec.Policies = slices.Clone(spec.Policies)
	return spec
}

func cloneEffectRecord(in EffectRecord) EffectRecord {
	out := in
	out.Resource.Attributes = cloneMap(in.Resource.Attributes)
	out.Metadata = cloneMap(in.Metadata)
	return out
}

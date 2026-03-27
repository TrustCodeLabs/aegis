package aegis

import (
	"fmt"
	"sort"
)

type CapabilityRef string

func (c CapabilityRef) String() string {
	return string(c)
}

type GrantedCapabilities struct {
	values map[CapabilityRef]struct{}
}

type CapabilitySource string

const (
	CapabilitySourceSubject        CapabilitySource = "subject"
	CapabilitySourceGrantedContext CapabilitySource = "granted_context"
)

type CapabilityResolution struct {
	Source    CapabilitySource
	Subject   GrantedCapabilities
	Granted   GrantedCapabilities
	Effective GrantedCapabilities
}

func NewGrantedCapabilities(refs ...CapabilityRef) GrantedCapabilities {
	values := make(map[CapabilityRef]struct{}, len(refs))
	for _, ref := range refs {
		if ref == "" {
			continue
		}
		values[ref] = struct{}{}
	}
	return GrantedCapabilities{values: values}
}

func (g GrantedCapabilities) Has(ref CapabilityRef) bool {
	_, ok := g.values[ref]
	return ok
}

func (g GrantedCapabilities) Slice() []CapabilityRef {
	out := make([]CapabilityRef, 0, len(g.values))
	for ref := range g.values {
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i] < out[j]
	})
	return out
}

func (g GrantedCapabilities) clone() GrantedCapabilities {
	if len(g.values) == 0 {
		return GrantedCapabilities{values: map[CapabilityRef]struct{}{}}
	}

	values := make(map[CapabilityRef]struct{}, len(g.values))
	for ref := range g.values {
		values[ref] = struct{}{}
	}
	return GrantedCapabilities{values: values}
}

func ResolveCapabilities(subject Subject, granted GrantedCapabilities, hasGrantedContext bool) CapabilityResolution {
	subjectCaps := NewGrantedCapabilities(subject.Capabilities...)
	resolution := CapabilityResolution{
		Source:    CapabilitySourceSubject,
		Subject:   subjectCaps,
		Granted:   granted.clone(),
		Effective: subjectCaps.clone(),
	}
	if hasGrantedContext {
		resolution.Source = CapabilitySourceGrantedContext
		resolution.Effective = granted.clone()
	}
	return resolution
}

type CapabilityManager struct {
	hooks ObservabilityHooks
}

func NewCapabilityManager(hooks ObservabilityHooks) *CapabilityManager {
	if hooks == nil {
		hooks = NoopObserver{}
	}
	return &CapabilityManager{hooks: hooks}
}

func (m *CapabilityManager) Check(exec ExecutionContext, ref CapabilityRef) error {
	allowed := exec.Capabilities.Has(ref)
	if m.hooks != nil {
		m.hooks.CapabilityChecked(exec, ref, allowed)
	}
	if allowed {
		return nil
	}

	return newKernelError(
		CodeCapabilityDenied,
		fmt.Sprintf("capability %q denied for operation %q", ref, exec.Operation),
		nil,
	)
}

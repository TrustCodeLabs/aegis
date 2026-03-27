package aegis

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

type PolicyRef struct {
	ID string
}

func PolicyID(id string) PolicyRef {
	return PolicyRef{ID: id}
}

type DegradationMode string

type ResourceRef struct {
	Kind       string
	ID         string
	Module     string
	TenantID   string
	Attributes map[string]any
}

type PolicyContext struct {
	RequestID            string
	TraceID              string
	Timestamp            time.Time
	Environment          string
	Module               string
	Operation            string
	Subject              Subject
	Resource             ResourceRef
	Input                any
	Metadata             map[string]any
	Capabilities         GrantedCapabilities
	CapabilityResolution CapabilityResolution
	DegradationMode      DegradationMode
	InvocationClass      string
	Transport            string
	TenantID             string
}

type RedactionRule struct {
	Path string
	Mode string
}

type Obligation struct {
	Type   string
	Params map[string]any
}

type Decision struct {
	Allowed     bool
	Reason      string
	PolicyIDs   []string
	Obligations []Obligation
	Tags        map[string]string
	Redactions  []RedactionRule
	Confirm     bool
	Severity    string
}

type DecisionExplanation struct {
	FinalDecision Decision
	Evaluations   []PolicyEvaluationRecord
}

type PolicyEvaluationRecord struct {
	PolicyID    string
	Allowed     bool
	Reason      string
	Obligations []Obligation
	Duration    time.Duration
}

type PolicyMetadata struct {
	Category    string
	Module      string
	Description string
	AppliesTo   []string
	Severity    string
}

type Policy interface {
	ID() string
	Metadata() PolicyMetadata
	Evaluate(ctx context.Context, in PolicyContext) (Decision, error)
}

type PolicyEvaluator func(ctx context.Context, in PolicyContext) (Decision, error)

type PolicySpec struct {
	ID          string
	Category    string
	Module      string
	Description string
	AppliesTo   []string
	Severity    string
	Handler     PolicyEvaluator
}

func DefinePolicy(spec PolicySpec) Policy {
	return definedPolicy{spec: spec}
}

type definedPolicy struct {
	spec PolicySpec
}

func (p definedPolicy) ID() string {
	return p.spec.ID
}

func (p definedPolicy) Metadata() PolicyMetadata {
	return PolicyMetadata{
		Category:    p.spec.Category,
		Module:      p.spec.Module,
		Description: p.spec.Description,
		AppliesTo:   cloneStringSlice(p.spec.AppliesTo),
		Severity:    p.spec.Severity,
	}
}

func (p definedPolicy) Evaluate(ctx context.Context, in PolicyContext) (Decision, error) {
	if p.spec.Handler == nil {
		return Decision{}, newKernelError(
			CodeBootstrapFailed,
			fmt.Sprintf("policy %q has no evaluator", p.spec.ID),
			nil,
		)
	}

	decision, err := p.spec.Handler(ctx, in)
	if err != nil {
		return Decision{}, err
	}
	if len(decision.PolicyIDs) == 0 {
		decision.PolicyIDs = []string{p.spec.ID}
	}
	if decision.Severity == "" {
		decision.Severity = p.spec.Severity
	}
	return decision, nil
}

type PolicyRegistry struct {
	policies map[string]Policy
}

func NewPolicyRegistry() *PolicyRegistry {
	return &PolicyRegistry{
		policies: make(map[string]Policy),
	}
}

func (r *PolicyRegistry) Register(policy Policy) error {
	if policy == nil {
		return newKernelError(CodeInvalidConfig, "policy cannot be nil", nil)
	}
	if policy.ID() == "" {
		return newKernelError(CodeInvalidConfig, "policy id cannot be empty", nil)
	}
	if _, exists := r.policies[policy.ID()]; exists {
		return newKernelError(CodeDuplicatePolicy, fmt.Sprintf("policy %q is already registered", policy.ID()), nil)
	}
	r.policies[policy.ID()] = policy
	return nil
}

func (r *PolicyRegistry) Lookup(id string) (Policy, bool) {
	policy, ok := r.policies[id]
	return policy, ok
}

func (r *PolicyRegistry) All() []Policy {
	out := make([]Policy, 0, len(r.policies))
	for _, policy := range r.policies {
		out = append(out, policy)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID() < out[j].ID()
	})
	return out
}

type PolicyEngine interface {
	Evaluate(ctx context.Context, in PolicyContext, refs []PolicyRef) (Decision, error)
	Explain(ctx context.Context, in PolicyContext, refs []PolicyRef) (DecisionExplanation, error)
}

type RegistryPolicyEngine struct {
	registry *PolicyRegistry
}

func NewPolicyEngine(registry *PolicyRegistry) *RegistryPolicyEngine {
	if registry == nil {
		registry = NewPolicyRegistry()
	}
	return &RegistryPolicyEngine{registry: registry}
}

func (e *RegistryPolicyEngine) Evaluate(ctx context.Context, in PolicyContext, refs []PolicyRef) (Decision, error) {
	explanation, err := e.Explain(ctx, in, refs)
	if err != nil {
		return Decision{}, err
	}
	return explanation.FinalDecision, nil
}

func (e *RegistryPolicyEngine) Explain(ctx context.Context, in PolicyContext, refs []PolicyRef) (DecisionExplanation, error) {
	if len(refs) == 0 {
		return DecisionExplanation{
			FinalDecision: Decision{
				Allowed: true,
				Reason:  "no policies required",
			},
		}, nil
	}

	final := Decision{
		Allowed: true,
		Tags:    map[string]string{},
	}

	records := make([]PolicyEvaluationRecord, 0, len(refs))
	reasons := make([]string, 0, len(refs))

	for _, ref := range refs {
		if ref.ID == "" {
			continue
		}

		policy, ok := e.registry.Lookup(ref.ID)
		if !ok {
			return DecisionExplanation{}, newKernelError(
				CodePolicyNotFound,
				fmt.Sprintf("policy %q is not registered", ref.ID),
				nil,
			)
		}

		started := time.Now()
		decision, err := policy.Evaluate(ctx, in)
		duration := time.Since(started)
		if err != nil {
			return DecisionExplanation{}, err
		}

		records = append(records, PolicyEvaluationRecord{
			PolicyID:    policy.ID(),
			Allowed:     decision.Allowed,
			Reason:      decision.Reason,
			Obligations: cloneObligations(decision.Obligations),
			Duration:    duration,
		})
		if decision.Reason != "" {
			reasons = append(reasons, decision.Reason)
		}

		next, err := mergeDecision(final, decision)
		if err != nil {
			return DecisionExplanation{}, err
		}
		final = next
	}

	if final.Reason == "" {
		final.Reason = strings.Join(reasons, "; ")
	}

	return DecisionExplanation{
		FinalDecision: final,
		Evaluations:   records,
	}, nil
}

func mergeDecision(current, next Decision) (Decision, error) {
	out := Decision{
		Allowed:     current.Allowed && next.Allowed,
		PolicyIDs:   append(append([]string{}, current.PolicyIDs...), next.PolicyIDs...),
		Obligations: append(cloneObligations(current.Obligations), cloneObligations(next.Obligations)...),
		Tags:        cloneStringMap(current.Tags),
		Redactions:  append(cloneRedactions(current.Redactions), cloneRedactions(next.Redactions)...),
		Confirm:     current.Confirm || next.Confirm,
		Severity:    stricterSeverity(current.Severity, next.Severity),
		Reason:      current.Reason,
	}

	if out.Reason == "" {
		out.Reason = next.Reason
	}
	if !next.Allowed {
		out.Allowed = false
		if next.Reason != "" {
			out.Reason = next.Reason
		}
	}

	for key, value := range next.Tags {
		if currentValue, exists := out.Tags[key]; exists && currentValue != value {
			return Decision{}, newKernelError(
				CodePolicyDenied,
				fmt.Sprintf("conflicting policy tag for %q", key),
				nil,
			)
		}
		out.Tags[key] = value
	}

	return out, nil
}

func cloneObligations(in []Obligation) []Obligation {
	if len(in) == 0 {
		return nil
	}
	out := make([]Obligation, len(in))
	for index, obligation := range in {
		out[index] = Obligation{
			Type:   obligation.Type,
			Params: cloneMap(obligation.Params),
		}
	}
	return out
}

func cloneRedactions(in []RedactionRule) []RedactionRule {
	if len(in) == 0 {
		return nil
	}
	out := make([]RedactionRule, len(in))
	copy(out, in)
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func stricterSeverity(left, right string) string {
	if severityRank(right) > severityRank(left) {
		return right
	}
	return left
}

func severityRank(in string) int {
	switch strings.ToLower(in) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

# RFC 0003 — Policy Engine, Effect System, and Introspection API
**Status:** Draft  
**Depends on:** RFC 0001, RFC 0002, RFC 0002-A  
**Last updated:** 2026-03-26  
**Target version:** `0.1.0-alpha`

---

# TL;DR

This RFC defines the **decision brain** of Aegis Core:

- a **Policy Engine** that centralizes authorization and execution constraints
- an **Effect System** that makes side effects explicit, enforceable, and auditable
- an **Introspection API** that exposes runtime topology and metadata for humans, operators, and AI agents

Together, these subsystems turn Aegis from a modular runtime into a **governed execution platform**.

This RFC is implementation-oriented and is intended to be detailed enough for an AI agent to build the first working version of these subsystems.

---

# Table of Contents

1. [Problem Statement](#1-problem-statement)
2. [Goals](#2-goals)
3. [Non-Goals](#3-non-goals)
4. [Core Concepts](#4-core-concepts)
5. [Policy Engine Overview](#5-policy-engine-overview)
6. [Policy Model](#6-policy-model)
7. [Policy Context and Inputs](#7-policy-context-and-inputs)
8. [Policy Decisions and Obligations](#8-policy-decisions-and-obligations)
9. [Policy Evaluation Lifecycle](#9-policy-evaluation-lifecycle)
10. [Policy Composition and Precedence](#10-policy-composition-and-precedence)
11. [Effect System Overview](#11-effect-system-overview)
12. [Effect Declarations](#12-effect-declarations)
13. [Effect Capture and Recording](#13-effect-capture-and-recording)
14. [Effect Enforcement](#14-effect-enforcement)
15. [Idempotency and Effect Safety](#15-idempotency-and-effect-safety)
16. [Transactional Boundaries](#16-transactional-boundaries)
17. [Introspection API Overview](#17-introspection-api-overview)
18. [Introspection Data Model](#18-introspection-data-model)
19. [AI-Facing Introspection](#19-ai-facing-introspection)
20. [Runtime Security and Redaction Rules](#20-runtime-security-and-redaction-rules)
21. [Reference Interfaces in Go](#21-reference-interfaces-in-go)
22. [Execution Flow Examples](#22-execution-flow-examples)
23. [Example Policies](#23-example-policies)
24. [Example Effect Graph](#24-example-effect-graph)
25. [Implementation Plan](#25-implementation-plan)
26. [Open Questions](#26-open-questions)
27. [Conclusion](#27-conclusion)

---

# 1. Problem Statement

A modular runtime is not enough by itself.

Without a centralized decision system, application behavior degenerates into:

- authorization checks scattered across handlers
- inconsistent safety rules
- invisible side effects
- weak auditability
- unclear runtime topology
- opaque coupling
- impossible-to-trust AI integration

In most real systems, the hardest parts are not:

- routing
- parsing JSON
- opening DB connections

The hardest parts are:

- deciding whether something is allowed
- deciding what should happen if it is allowed
- recording what actually happened
- explaining the system to humans and machines

Aegis needs first-class answers to those problems.

This RFC introduces three core subsystems:

1. **Policy Engine**
   - decides whether an action is allowed, denied, constrained, or conditionally allowed

2. **Effect System**
   - captures and governs side effects as explicit runtime behavior

3. **Introspection API**
   - exposes machine-readable knowledge of modules, operations, policies, capabilities, and effects

---

# 2. Goals

## 2.1 Primary goals

Aegis MUST provide:

1. A centralized **Policy Engine**
2. A structured **Decision Model**
3. A runtime **Effect System** with explicit declarations and capture
4. A runtime **Effect Log / Effect Graph**
5. An **Introspection API** for operators, tooling, and AI agents
6. A mechanism to connect:
   - policies
   - effects
   - audit ledger
   - execution context
7. Safe redaction and visibility boundaries for introspection
8. Enough structure that an AI agent can:
   - inspect runtime topology
   - reason about operation safety
   - understand side effects
   - detect missing or weak constraints

## 2.2 Secondary goals

Aegis SHOULD also provide:

- field-level policy hooks
- obligation-based policy responses
- effect classification
- deterministic operation policy hooks
- compatibility-ready introspection schemas
- graph export for tooling
- policy simulation mode

---

# 3. Non-Goals

This RFC does **not** define:

- a full user-facing auth product
- a specific UI for runtime exploration
- a specific storage engine for every effect record
- a full workflow language
- a general theorem prover for policy correctness
- automated planning behavior for AI agents

This RFC defines the primitives that make those things possible.

---

# 4. Core Concepts

## 4.1 Policy
A rule or set of rules evaluated against an execution request.

## 4.2 Decision
The result of policy evaluation.

## 4.3 Obligation
A constraint or required action attached to a decision.

## 4.4 Effect
A side effect caused by operation execution.

## 4.5 Effect declaration
The expected effect contract declared by an operation.

## 4.6 Effect record
A captured runtime record of an actual effect.

## 4.7 Introspection
Machine-readable access to runtime topology and metadata.

## 4.8 Topology
The graph of:
- modules
- operations
- policies
- capabilities
- bindings
- effects
- dependencies

---

# 5. Policy Engine Overview

The Policy Engine is the system that decides:

- can this subject invoke this operation?
- under what constraints?
- with what obligations?
- against which resource scope?
- in which environment?
- with which safety classification?

The Policy Engine MUST be transport-agnostic.  
It should not matter whether the request comes from:

- REST
- GraphQL
- WebSocket
- Jobs
- Internal RPC
- MCP / AI tooling

## 5.1 Design intent

The Policy Engine must replace ad hoc authorization checks with a canonical runtime mechanism.

It should support:

- operation access control
- resource access control
- capability use control
- environment constraints
- tenant isolation
- field-level visibility decisions
- confirmation requirements
- degraded-mode restrictions

---

# 6. Policy Model

## 6.1 Core interface

A policy evaluates a `PolicyContext` and yields a `Decision`.

```go
type Policy interface {
    ID() string
    Evaluate(ctx context.Context, in PolicyContext) (Decision, error)
}
```

## 6.2 Policy granularity

Policies may apply at different levels:

- runtime-global
- module-level
- operation-level
- resource-level
- effect-level
- field-level

## 6.3 Policy categories

The initial policy categories are:

- `access`
- `capability_use`
- `field_visibility`
- `effect_permission`
- `degradation`
- `rate_control`
- `tenant_isolation`
- `ai_exposure`

## 6.4 Policy registration

Policies must be registered during module or runtime initialization.

Example:

```go
reg.RegisterPolicy("billing.invoice.refund", RefundPolicy{})
reg.RegisterPolicy("storage.file.delete.confirmation", DeleteFileConfirmationPolicy{})
```

## 6.5 Policy references

Operations should reference policies by stable IDs, not by inlined lambdas in business handlers.

Example:

```go
RequiredPolicies: []PolicyRef{
    {ID: "billing.invoice.refund"},
    {ID: "tenant.isolation"},
}
```

---

# 7. Policy Context and Inputs

## 7.1 PolicyContext

The Policy Engine must evaluate against a rich context.

```go
type PolicyContext struct {
    RequestID        string
    TraceID          string
    Timestamp        time.Time
    Environment      string
    Module           string
    Operation        string
    Subject          Subject
    Resource         ResourceRef
    Input            any
    Metadata         map[string]any
    Capabilities     GrantedCapabilities
    DegradationMode  DegradationMode
    InvocationClass  string
    Transport        string
    TenantID         string
}
```

## 7.2 Subject

Policies are evaluated against normalized subjects:

- user
- service
- system
- job
- internal agent
- external AI agent

## 7.3 ResourceRef

A resource must be structured.

```go
type ResourceRef struct {
    Kind       string
    ID         string
    Module     string
    TenantID   string
    Attributes map[string]any
}
```

## 7.4 Input awareness

Policies MAY inspect input values when necessary.  
However:

- policy evaluation should avoid becoming arbitrary business logic
- heavy domain decisions should stay in domain/application services
- input-derived policy should remain explainable and observable

---

# 8. Policy Decisions and Obligations

## 8.1 Decision model

A decision MUST be more expressive than simple allow/deny.

```go
type Decision struct {
    Allowed      bool
    Reason       string
    PolicyIDs    []string
    Obligations  []Obligation
    Tags         map[string]string
    Redactions   []RedactionRule
    Confirm      bool
    Severity     string
}
```

## 8.2 Obligations

Obligations tell the execution engine what must happen if the action proceeds.

Examples:

- require audit tag
- force confirmation
- redact fields
- limit timeout
- require idempotency key
- limit effect scope
- deny in degraded mode
- force readonly path
- require dual-approval token

```go
type Obligation struct {
    Type    string
    Params  map[string]any
}
```

## 8.3 Decision semantics

Possible effective outcomes:

- allow
- deny
- allow with obligations
- deny with explanation
- conditional allow pending confirmation
- allow with field redactions
- allow in readonly mode only

## 8.4 Decision explainability

Every decision SHOULD be explainable.  
At minimum, runtime should retain:

- which policy participated
- why it denied or allowed
- which obligations were returned

This is critical for operators and AI systems.

---

# 9. Policy Evaluation Lifecycle

## 9.1 Evaluation phases

Policies can be evaluated at several points:

1. **operation preflight**
2. **resource access**
3. **effect execution**
4. **field serialization**
5. **AI exposure / introspection exposure**

## 9.2 Phase 1: Operation preflight

Before handler execution:
- resolve operation
- construct policy context
- evaluate required policies
- merge decisions
- apply obligations

## 9.3 Phase 2: Resource access

When code uses a resource via the Resource Layer:
- the runtime may enforce additional policy checks
- especially for sensitive reads/writes, tenant boundaries, or destructive actions

## 9.4 Phase 3: Effect execution

If an operation tries to emit an effect:
- verify effect is declared
- verify effect is allowed by policy
- verify capability is present

## 9.5 Phase 4: Output filtering

Before returning output:
- field-level redaction policies may apply
- visibility rules may be enforced

---

# 10. Policy Composition and Precedence

## 10.1 Why this matters

Multiple policies may apply simultaneously.  
Aegis needs deterministic merge semantics.

## 10.2 Composition strategy

For v1, policy merge SHOULD follow conservative rules:

1. any explicit deny overrides allow
2. obligations accumulate unless conflicting
3. confirmation requirements accumulate
4. field redactions accumulate
5. timeout reductions choose the strictest
6. readonly constraints override mutating allowances

## 10.3 Precedence levels

Suggested order:

1. runtime-global
2. environment policy
3. module policy
4. operation policy
5. resource/effect policy

## 10.4 Conflict rules

If policy obligations conflict in an unsafe or ambiguous way, the runtime SHOULD fail closed.

Example:
- policy A says timeout 5s
- policy B says timeout 30s
- choose 5s

Example:
- policy A says allow effect X
- policy B says deny effect X
- deny effect X

---

# 11. Effect System Overview

The Effect System exists to make side effects:

- explicit
- declared
- enforceable
- observable
- auditable
- explainable

Without an Effect System, the runtime cannot honestly answer:
- what did this operation do?
- what did it try to do?
- what was denied?
- what changed state?
- what external consequences occurred?

## 11.1 Core principle

Handlers do not get to create invisible side effects.

All significant side effects should pass through runtime-managed resource and effect paths.

## 11.2 Effect categories

Initial categories:

- `db.read`
- `db.write`
- `cache.read`
- `cache.write`
- `event.publish`
- `job.dispatch`
- `http.outbound`
- `storage.read`
- `storage.write`
- `secret.read`
- `audit.write`
- `metrics.write`
- `ws.emit`

---

# 12. Effect Declarations

## 12.1 Operation-declared effects

Operations SHOULD declare expected effects.

```go
type EffectSpec struct {
    Name         string
    Kind         string
    RequiredCap  CapabilityRef
    Critical     bool
    Idempotent   bool
    Optional     bool
    Metadata     map[string]any
}
```

Example:

```go
Effects: []EffectSpec{
    {
        Name:        "db.update.invoice",
        Kind:        "db.write",
        RequiredCap: Cap("sql.write:billing"),
        Critical:    true,
        Idempotent:  true,
    },
    {
        Name:        "event.publish.invoice.refunded",
        Kind:        "event.publish",
        RequiredCap: Cap("event.publish:invoice.refunded"),
        Critical:    false,
        Idempotent:  true,
    },
}
```

## 12.2 Required declaration policy

V1 rule:

- mutating operations SHOULD declare all nontrivial effects
- high-assurance modules MUST declare all critical effects
- undeclared critical effects SHOULD be denied or logged as violations

## 12.3 Optional vs critical effects

- **critical** effects are required for operation correctness
- **optional** effects may fail without invalidating core mutation

Example:
- updating billing record = critical
- sending analytics event = optional

---

# 13. Effect Capture and Recording

## 13.1 Effect record

The runtime SHOULD record every effect that is attempted or completed.

```go
type EffectRecord struct {
    ID             string
    Operation      string
    Module         string
    EffectName     string
    EffectKind     string
    SubjectID      string
    TenantID       string
    Resource       ResourceRef
    Capability     string
    Declared       bool
    Allowed        bool
    Critical       bool
    Idempotent     bool
    Status         string
    ErrorCode      string
    StartedAt      time.Time
    FinishedAt     time.Time
    TraceID        string
    RequestID      string
    Metadata       map[string]any
}
```

## 13.2 Effect statuses

Recommended statuses:

- planned
- attempted
- allowed
- denied
- completed
- failed
- skipped
- compensated

## 13.3 Capture timing

At minimum, runtime should capture:

- planned effect from declaration
- attempted effect at invocation time
- final result after execution

## 13.4 Effect graph

The runtime SHOULD be able to reconstruct the effect graph of an operation execution.

Example:

```text
billing.invoice.refund
├── db.write.invoice_state      [completed]
├── event.publish.refund_event  [completed]
└── audit.write                 [completed]
```

---

# 14. Effect Enforcement

## 14.1 Effect validity checks

Before an effect is executed, the runtime SHOULD verify:

1. the effect is declared or explicitly permitted by runtime policy
2. the operation has the required capability
3. the relevant policy allows the effect
4. the degradation mode permits it
5. required obligations are satisfied

## 14.2 Denying undeclared effects

For v1:
- undeclared mutating effects SHOULD at least emit a violation
- high-assurance mode SHOULD deny undeclared critical effects

## 14.3 Effect wrappers

The Resource Layer should act as the main effect enforcement point.

Example flow:

```text
handler
→ exec.Resources.Storage("user-uploads").Write(...)
→ capability check
→ effect declaration check
→ effect policy check
→ driver call
→ effect record write
```

## 14.4 Direct-driver bypass is forbidden

Code MUST NOT access drivers directly outside authorized resource wrappers.

This is necessary to preserve effect visibility and enforcement.

---

# 15. Idempotency and Effect Safety

## 15.1 Why it matters

Mutating systems need predictable retries.

If an operation is retried:
- some effects must not duplicate
- some effects can safely repeat
- some effects need dedupe keys

## 15.2 Operation-level idempotency

Operations MAY declare:

```go
Idempotent: true
```

## 15.3 Effect-level idempotency

Even in a non-idempotent operation, some effects may be idempotent.

Example:
- `audit.write` can be deduped by request ID
- `event.publish` may require dedupe key
- `db.write` may rely on version check

## 15.4 Dedupe strategy hooks

The effect system SHOULD allow idempotency handlers per effect type.

Example:
- based on request ID
- based on operation key
- based on semantic resource key

---

# 16. Transactional Boundaries

## 16.1 Core principle

Not every effect can share the same transaction boundary.

Examples:
- DB writes can often be transactional
- event publication may require outbox patterns
- HTTP calls are external and non-transactional
- audit writes may be separate but must be linked

## 16.2 Effect consistency classes

Suggested classes:

- `in_tx`
- `after_commit`
- `best_effort`
- `compensatable`
- `external_nonatomic`

## 16.3 Outbox compatibility

Aegis SHOULD be compatible with outbox-style patterns for:
- event publication
- job dispatch
- integration notifications

## 16.4 Compensation

Future effect models MAY support compensation handlers for:
- partial failures
- long-running sagas
- external rollback semantics

---

# 17. Introspection API Overview

The Introspection API exposes runtime metadata to:

- operators
- admin tooling
- documentation generators
- IDE plugins
- AI agents
- MCP adapters

It is one of the most important parts of the Aegis runtime.

## 17.1 Required properties

The Introspection API MUST be:

- machine-readable
- structured
- versioned
- filterable
- policy-aware
- redactable

## 17.2 What it should expose

At minimum:
- modules
- operations
- bindings
- capabilities
- policies
- effect declarations
- effect history summary
- topology relationships
- AI exposure metadata
- health state

---

# 18. Introspection Data Model

## 18.1 ModuleInfo

```go
type ModuleInfo struct {
    Name          string
    Version       string
    Status        string
    Description   string
    Dependencies  []string
    Operations    []string
    Policies      []string
    Capabilities  []string
    AI            *ModuleAISpec
}
```

## 18.2 OperationInfo

```go
type OperationInfo struct {
    Name               string
    Module             string
    Version            string
    InvocationClass    string
    Idempotent         bool
    Deterministic      bool
    RequiredPolicies   []string
    RequiredCapabilities []string
    Effects            []EffectSpecInfo
    Bindings           []BindingInfo
    AI                 *AIExposureSpec
}
```

## 18.3 PolicyInfo

```go
type PolicyInfo struct {
    ID            string
    Category      string
    Module        string
    Description   string
    AppliesTo     []string
    Severity      string
}
```

## 18.4 CapabilityGrantInfo

```go
type CapabilityGrantInfo struct {
    Module       string
    Capability   string
    Granted      bool
    Source       string
    Conditions   map[string]any
}
```

## 18.5 TopologyGraph

```go
type TopologyGraph struct {
    Nodes []TopologyNode
    Edges []TopologyEdge
}
```

## 18.6 Topology edge examples

- module → operation
- operation → policy
- operation → capability
- operation → effect
- operation → binding
- module → dependency
- effect → resource

---

# 19. AI-Facing Introspection

Because Aegis is designed to be MCP-friendly and SKILLS-aware, the Introspection API must support AI-safe views.

## 19.1 AI view requirements

The AI-facing introspection view SHOULD expose:

- operation purpose
- safety class
- side effects
- required confirmation hint
- input and output schema
- visibility level
- tags
- anti-use-cases

## 19.2 AI-facing operation record example

```json
{
  "operation": "billing.invoice.refund",
  "module": "billing",
  "invocation_class": "mutate",
  "ai": {
    "exposed": true,
    "exposure": "internal",
    "title": "Refund invoice",
    "summary": "Refunds a previously charged invoice.",
    "requires_confirmation": true,
    "side_effects": [
      "updates invoice state",
      "publishes invoice.refunded",
      "writes audit record"
    ]
  },
  "effects": [
    {
      "name": "db.update.invoice",
      "kind": "db.write",
      "critical": true
    },
    {
      "name": "event.publish.invoice.refunded",
      "kind": "event.publish",
      "critical": false
    }
  ]
}
```

## 19.3 AI-safe filtering

AI-facing introspection MUST respect:
- operation exposure settings
- policy visibility
- redaction rules
- environment restrictions

---

# 20. Runtime Security and Redaction Rules

## 20.1 Principle

Introspection is powerful and dangerous.  
It must reveal enough to be useful without leaking secrets or internal-only details.

## 20.2 Never expose directly

The Introspection API MUST NOT expose:
- secret values
- raw credentials
- unrestricted driver internals
- private keys
- sensitive env vars
- hidden internal operations unless authorized

## 20.3 Redactable fields

Examples of redactable data:
- outbound endpoint identities
- tenant-specific resource IDs
- effect metadata containing PII
- internal-only policies

## 20.4 Visibility tiers

Recommended tiers:

- `public`
- `workspace`
- `internal`
- `restricted`

Introspection consumers should receive filtered output based on tier and subject.

---

# 21. Reference Interfaces in Go

This section is intended to be close to implementation-ready.

## 21.1 PolicyEngine

```go
type PolicyEngine interface {
    Evaluate(ctx context.Context, in PolicyContext, refs []PolicyRef) (Decision, error)
    Explain(ctx context.Context, in PolicyContext, refs []PolicyRef) (DecisionExplanation, error)
}
```

## 21.2 DecisionExplanation

```go
type DecisionExplanation struct {
    FinalDecision Decision
    Evaluations   []PolicyEvaluationRecord
}
```

## 21.3 PolicyEvaluationRecord

```go
type PolicyEvaluationRecord struct {
    PolicyID    string
    Allowed     bool
    Reason      string
    Obligations []Obligation
    Duration    time.Duration
}
```

## 21.4 EffectTracker

```go
type EffectTracker interface {
    Plan(exec ExecutionContext, spec EffectSpec) error
    Before(exec ExecutionContext, spec EffectSpec, resource ResourceRef) (EffectRecord, error)
    After(rec EffectRecord, err error) error
    Records(exec ExecutionContext) []EffectRecord
}
```

## 21.5 IntrospectionService

```go
type IntrospectionService interface {
    Modules(ctx context.Context, filter IntrospectionFilter) ([]ModuleInfo, error)
    Operations(ctx context.Context, filter IntrospectionFilter) ([]OperationInfo, error)
    Policies(ctx context.Context, filter IntrospectionFilter) ([]PolicyInfo, error)
    Capabilities(ctx context.Context, filter IntrospectionFilter) ([]CapabilityGrantInfo, error)
    Topology(ctx context.Context, filter IntrospectionFilter) (TopologyGraph, error)
    Effects(ctx context.Context, filter EffectQuery) ([]EffectRecord, error)
}
```

## 21.6 IntrospectionFilter

```go
type IntrospectionFilter struct {
    Subject        Subject
    VisibilityTier string
    Module         string
    Operation      string
    AIOnly         bool
}
```

---

# 22. Execution Flow Examples

## 22.1 Example: refund invoice

Operation: `billing.invoice.refund`

### Declared metadata
- required policies:
  - `billing.invoice.refund`
  - `tenant.isolation`
- required capabilities:
  - `sql.write:billing`
  - `event.publish:invoice.refunded`
  - `audit.write`
- declared effects:
  - `db.update.invoice`
  - `event.publish.invoice.refunded`
  - `audit.write`

### Execution flow

```text
request enters
→ operation lookup
→ PolicyEngine.Evaluate(...)
→ decision = allow with obligation(require_audit_tag)
→ ExecutionContext updated
→ handler executes
→ resource write attempts db.update.invoice
→ EffectTracker.Before(...)
→ capability check ok
→ effect policy ok
→ DB update runs
→ EffectTracker.After(..., nil)
→ event publication runs
→ audit write runs
→ output returned
→ Introspection and audit state updated
```

## 22.2 Example: deny undeclared delete

Operation tries:
- `storage.delete:user-uploads`

But:
- operation did not declare delete effect
- capability is absent
- policy does not allow destructive delete

Flow:

```text
handler calls resource delete
→ resource wrapper creates attempted effect
→ effect declaration check fails
→ capability check fails
→ effect denied
→ violation logged
→ execution fails closed
```

---

# 23. Example Policies

## 23.1 Role-based refund policy

```go
type RefundPolicy struct{}

func (p RefundPolicy) ID() string { return "billing.invoice.refund" }

func (p RefundPolicy) Evaluate(ctx context.Context, in PolicyContext) (Decision, error) {
    for _, role := range in.Subject.Roles {
        if role == "finance_admin" || role == "support_manager" {
            return Decision{
                Allowed:   true,
                Reason:    "subject has refund role",
                PolicyIDs: []string{p.ID()},
            }, nil
        }
    }

    return Decision{
        Allowed:   false,
        Reason:    "subject lacks refund role",
        PolicyIDs: []string{p.ID()},
        Severity:  "high",
    }, nil
}
```

## 23.2 Confirmation policy for destructive delete

```go
type DeleteConfirmationPolicy struct{}

func (p DeleteConfirmationPolicy) ID() string { return "storage.file.delete.confirmation" }

func (p DeleteConfirmationPolicy) Evaluate(ctx context.Context, in PolicyContext) (Decision, error) {
    return Decision{
        Allowed:   true,
        Reason:    "delete allowed only with confirmation",
        PolicyIDs: []string{p.ID()},
        Confirm:   true,
        Obligations: []Obligation{
            {
                Type: "require_confirmation",
                Params: map[string]any{
                    "level": "explicit",
                },
            },
        },
    }, nil
}
```

## 23.3 Redaction policy

```go
type UserEmailRedactionPolicy struct{}

func (p UserEmailRedactionPolicy) ID() string { return "identity.user.email.redaction" }

func (p UserEmailRedactionPolicy) Evaluate(ctx context.Context, in PolicyContext) (Decision, error) {
    if in.Subject.Type == "service" {
        return Decision{
            Allowed:   true,
            Reason:    "service subject may see full email",
            PolicyIDs: []string{p.ID()},
        }, nil
    }

    return Decision{
        Allowed:   true,
        Reason:    "email redacted for non-service subjects",
        PolicyIDs: []string{p.ID()},
        Redactions: []RedactionRule{
            {Path: "$.email", Mode: "mask"},
        },
    }, nil
}
```

---

# 24. Example Effect Graph

Example effect graph for `storage.file.save`:

```text
storage.file.save
├── storage.write.user-uploads        [completed]
├── audit.write                       [completed]
└── metrics.write.storage_save        [completed]
```

Example effect graph for `billing.invoice.refund`:

```text
billing.invoice.refund
├── db.write.billing.invoice          [completed]
├── event.publish.invoice.refunded    [completed]
├── audit.write                       [completed]
└── ws.emit.billing_updates           [skipped]
```

Possible richer graph export:

```json
{
  "operation": "billing.invoice.refund",
  "effects": [
    {
      "name": "db.write.billing.invoice",
      "kind": "db.write",
      "status": "completed",
      "critical": true
    },
    {
      "name": "event.publish.invoice.refunded",
      "kind": "event.publish",
      "status": "completed",
      "critical": false
    },
    {
      "name": "ws.emit.billing_updates",
      "kind": "ws.emit",
      "status": "skipped",
      "critical": false
    }
  ]
}
```

---

# 25. Implementation Plan

## Phase 1 — minimal policy engine
- Policy interface
- PolicyContext
- Decision model
- conservative merge strategy
- operation preflight evaluation

## Phase 2 — minimal effect system
- EffectSpec
- EffectTracker
- effect record lifecycle
- resource-layer effect enforcement
- undeclared effect violation logging

## Phase 3 — introspection service MVP
- modules endpoint / method
- operations endpoint / method
- policies endpoint / method
- capabilities endpoint / method
- topology export
- AI metadata filter

## Phase 4 — output redaction and field policies
- redaction rules
- serializer hooks
- visibility filtering

## Phase 5 — advanced effect safety
- idempotency hooks
- effect consistency classes
- outbox-aligned metadata
- policy-aware effect constraints

## Phase 6 — explanation and simulation
- decision explanation records
- policy simulation mode
- AI-readable effect summaries

---

# 26. Open Questions

1. Should field-level redaction live in the policy engine or serializer subsystem?
2. How strict should undeclared effect denial be in v1?
3. Should effect history be persisted in the same store as audit records or separately?
4. What is the stable schema versioning strategy for introspection responses?
5. How much policy detail should internal agents be allowed to see?
6. Should policy simulation support hypothetical subjects and environments?
7. Should every resource-layer call create an effect record, or only mutating/sensitive ones?
8. How should effect grouping interact with nested operation calls?
9. Should effect graphs be exportable in DOT/Graphviz form?
10. What is the best ergonomics for declaring field-level visibility without making code ugly?

---

# 27. Conclusion

RFC 0003 defines the runtime systems that make Aegis trustworthy.

Without these pieces, Aegis would be:
- modular
- fast
- clean

But still not fully governable.

With these pieces, Aegis becomes:

- policy-aware
- effect-aware
- introspectable
- explainable
- auditable
- AI-operable

That is the difference between “framework” and “application runtime OS”.

---

# Appendix A — Additional Supporting Types

```go
type RedactionRule struct {
    Path string
    Mode string
}

type EffectSpecInfo struct {
    Name       string
    Kind       string
    Critical   bool
    Idempotent bool
    Optional   bool
}

type BindingInfo struct {
    AdapterKind string
    Metadata    map[string]any
}

type EffectQuery struct {
    Module     string
    Operation  string
    TenantID   string
    TraceID    string
    Status     string
    Since      *time.Time
    Until      *time.Time
}

type TopologyNode struct {
    ID    string
    Kind  string
    Label string
}

type TopologyEdge struct {
    From string
    To   string
    Kind string
}
```

---

# Appendix B — Suggested Positioning Sentence

**Aegis Policy Engine decides what may happen, the Effect System records what did happen, and the Introspection API explains how the system is wired.**

---

# Appendix C — Suggested Future Commands

```bash
aegis introspect topology
aegis introspect policies
aegis introspect operations --ai-only
aegis explain decision --operation billing.invoice.refund
aegis explain effects --trace-id abc123
```

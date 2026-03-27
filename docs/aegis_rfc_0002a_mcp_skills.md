# RFC 0002-A — MCP Compatibility and AI Skill Integration
**Status:** Draft  
**Depends on:** RFC 0002  
**Related to:** RFC 0001  
**Last updated:** 2026-03-26  
**Target version:** `0.1.0-alpha`

---

# TL;DR

Aegis MUST be designed as an **MCP-friendly application runtime** and MUST provide a **SKILLS.md generator** so that systems, tools, and services built with Aegis can be consumed safely and effectively by AI agents.

This addendum defines:

- MCP compatibility goals
- MCP exposure model for Aegis applications
- constraints for safe tool exposure
- machine-readable capability and operation metadata
- `SKILLS.md` generation requirements
- conventions for AI-facing tool descriptions
- runtime introspection primitives needed to support agentic use

This RFC does **not** modify RFC 0002.  
It extends it.

---

# 1. Motivation

Aegis is already being designed as:

- operation-first
- capability-driven
- policy-aware
- auditable
- introspectable

That makes it unusually well positioned to be **AI-native**.

The next logical step is to formalize two things:

1. **MCP friendliness**
2. **AI-consumable skill descriptions**

In practice, this means that an application built on Aegis should be able to expose a clean, governed, machine-readable interface for AI systems, without requiring teams to handcraft ad hoc agent integration layers every time.

---

# 2. Goals

This RFC establishes the following goals.

## 2.1 Primary goals

Aegis SHOULD enable applications to:

1. expose selected operations through an **MCP-compatible interface**
2. generate machine-readable descriptions of those operations
3. produce a `SKILLS.md` file from operation/module metadata
4. clearly communicate:
   - what the tool does
   - when it should be used
   - what inputs it expects
   - what outputs it returns
   - what side effects it may perform
   - what permissions/capabilities it requires
   - what constraints or safety rules apply
5. support AI tool use without bypassing:
   - policy engine
   - capability enforcement
   - audit logging
   - observability

## 2.2 Secondary goals

Aegis SHOULD also:

- support grouping operations into AI-facing “skills”
- support AI-facing descriptions per module
- support versioned skill metadata
- support generation of docs from runtime introspection
- allow agent-facing descriptions to differ from internal engineering docs

---

# 3. Non-Goals

This RFC does **not** require Aegis to:

- embed an LLM
- implement planning logic
- provide a generic chatbot framework
- bypass security for AI callers
- auto-expose every operation to AI tools
- let AI see raw private internals by default

The key principle is:

> AI access is explicit, governed, introspected, and policy-bound.

---

# 4. Terminology

## 4.1 MCP
For this RFC, MCP refers to a machine-consumable protocol/interface for exposing tools, resources, or operations to AI systems in a structured way.

## 4.2 AI-facing operation
An operation explicitly marked as safe and useful for agent/tool consumption.

## 4.3 Skill
A coherent grouping of AI-facing operations, plus descriptive metadata explaining when and how they should be used.

## 4.4 SKILLS.md
A generated markdown artifact describing agent-usable skills, their triggers, usage boundaries, and tool contracts.

---

# 5. Why Aegis is Naturally MCP-Friendly

Aegis already has the exact primitives an AI-facing runtime needs:

- **Operations** as canonical executable units
- **Module manifests** as bounded descriptions
- **Capabilities** as explicit side-effect permissions
- **Policies** as authorization and execution constraints
- **Effects** as declared side effects
- **Introspection** as machine-readable topology
- **Audit ledger** as verifiable action history

That means Aegis can expose tools to AI systems in a way that is:

- explicit
- typed
- auditable
- secure
- evolvable

Most frameworks do not have these primitives centralized.  
Aegis does.

---

# 6. MCP-Friendly Design Requirements

## 6.1 Explicit exposure only

An operation MUST NOT be considered AI-exposed unless it is explicitly marked as such.

Example metadata:

```yaml
ai:
  exposed: true
```

## 6.2 Structured operation metadata

Every AI-exposed operation SHOULD define:

- title
- short description
- long description
- intended use cases
- anti-use-cases
- input schema
- output schema
- side effects
- idempotency behavior
- required policies
- safety notes

## 6.3 Safe operation boundaries

AI-exposed operations SHOULD be:
- narrow
- explicit
- composable
- well-scoped
- easy to audit

AI-exposed operations SHOULD NOT:
- expose raw unrestricted infra access
- expose ambient execution primitives
- expose unsafe internal escape hatches
- depend on ambiguous side effects

## 6.4 Policy and capability enforcement remain mandatory

If an AI invokes an operation through MCP or another AI-facing interface, the runtime MUST still enforce:

- subject resolution
- policy checks
- capability checks
- rate limiting if configured
- audit writing
- telemetry emission

AI calls are not privileged by default.

---

# 7. AI Exposure Model

## 7.1 Exposure levels

An operation MAY declare one of the following AI exposure levels:

- `none` — not exposed to AI systems
- `internal` — exposed only to trusted internal agents
- `workspace` — exposed to workspace-level agents/tools
- `public` — safe for broader AI tool integration

Example:

```yaml
ai:
  exposure: internal
```

## 7.2 Invocation class

AI-facing operations SHOULD declare invocation class:

- `read`
- `mutate`
- `admin`
- `dangerous`

This helps agent runtimes and safety policies reason about behavior.

## 7.3 Confirmation metadata

Mutating operations MAY require confirmation hints:

```yaml
ai:
  requires_confirmation: true
```

This does not force UI behavior in the runtime itself, but provides machine-readable safety hints.

---

# 8. MCP Mapping Model

Aegis SHOULD provide a transport or adapter that maps AI-exposed operations into MCP-compatible tool definitions.

## 8.1 Conceptual mapping

| Aegis | MCP-facing concept |
|------|---------------------|
| Module | skill namespace / tool collection |
| Operation | tool |
| Input contract | tool input schema |
| Output contract | tool output schema |
| Capability metadata | side-effect / permission hints |
| Policy metadata | invocation constraints |
| Effects | action semantics |
| Introspection graph | tool registry/resource catalog |

## 8.2 Tool identity

Recommended format:

```text
<module>.<operation>
```

Examples:

- `billing.invoice.create`
- `billing.invoice.refund`
- `storage.file.upload`
- `identity.user.lookup`

## 8.3 Input schema generation

Input contracts MUST be convertible into a machine-readable schema representation.

Recommended targets:
- JSON Schema
- OpenAPI schema fragments
- internal canonical schema model

## 8.4 Output schema generation

Output contracts SHOULD also be convertible into a machine-readable schema representation.

This helps:
- agent validation
- result explanation
- client generation
- planning safety

---

# 9. AI-Facing Metadata Model

## 9.1 Operation-level metadata

Each operation MAY declare:

```yaml
ai:
  exposed: true
  exposure: internal
  title: Refund invoice
  summary: Refunds a previously charged invoice.
  description: >
    Use this when a valid refund must be issued for an existing invoice.
    This operation updates invoice state, records audit data, and may publish refund events.
  use_cases:
    - reverse an accidental charge
    - refund a canceled order
  avoid_when:
    - invoice has already been refunded
    - caller is unsure which invoice should be refunded
  invocation_class: mutate
  requires_confirmation: true
  idempotent: true
  side_effects:
    - updates billing records
    - publishes invoice.refunded event
    - writes audit ledger record
  tags:
    - billing
    - finance
    - refunds
```

## 9.2 Module-level metadata

Each module MAY declare:

```yaml
ai:
  title: Billing operations
  summary: Financial operations related to invoices, refunds, and payment state.
  intended_for:
    - internal support agents
    - finance tooling agents
```

## 9.3 Skill grouping metadata

Modules MAY declare skill groups:

```yaml
ai:
  skills:
    - name: invoice_management
      title: Invoice management
      operations:
        - billing.invoice.get
        - billing.invoice.create
        - billing.invoice.refund
```

---

# 10. SKILLS.md Generator

## 10.1 Requirement

Aegis SHOULD provide a generator that produces `SKILLS.md` from:

- module manifests
- operation specs
- AI metadata
- policy metadata
- capability/effect declarations

## 10.2 Why it matters

AI systems often perform much better when they receive:
- concise usage boundaries
- explicit trigger conditions
- clear side effects
- examples
- anti-pattern warnings

A generated `SKILLS.md` lets Aegis applications ship a consistent AI-facing operational manual.

## 10.3 Generator inputs

The generator SHOULD read:

1. module manifest
2. operation registry
3. AI exposure metadata
4. policy metadata
5. effect declarations
6. optional custom documentation overrides

## 10.4 Generator outputs

The generator MUST produce a markdown file containing, at minimum:

- skill name
- description
- trigger conditions
- allowed use cases
- disallowed use cases
- operation list
- expected inputs/outputs summary
- side effect summary
- safety/confirmation notes
- version metadata

## 10.5 Output location

Recommended default:

```text
./generated/SKILLS.md
```

Alternative output locations MAY be configured.

---

# 11. Proposed SKILLS.md Structure

The generated file SHOULD follow a stable structure.

Example:

```md
# Skills

## billing.invoice_management

**Description**  
Manage invoice lookup, creation, and refund operations.

**Use when**
- a caller needs to inspect an invoice
- a caller needs to create a new invoice
- a caller needs to refund a valid invoice

**Do not use when**
- the invoice identifier is unknown and cannot be resolved
- the caller is attempting unsupported financial actions
- the action would bypass required approval flow

**Operations**
- `billing.invoice.get`
- `billing.invoice.create`
- `billing.invoice.refund`

**Safety notes**
- refunding is a mutating action
- some actions may require confirmation
- all actions are policy-checked and audited
```

## 11.1 Suggested per-operation section

Each operation entry SHOULD include:

- title
- summary
- invocation class
- input summary
- output summary
- side effects
- idempotency
- confirmation requirement
- visibility level

---

# 12. Generator CLI

A future Aegis CLI SHOULD support:

```bash
aegis generate skills
```

Optional flags MAY include:

```bash
aegis generate skills --module billing
aegis generate skills --format markdown
aegis generate skills --output ./generated/SKILLS.md
```

---

# 13. Runtime Introspection Requirements

The runtime SHOULD expose enough structured metadata for generators and MCP adapters.

## 13.1 Required introspection fields

At minimum, introspection for AI-facing operations SHOULD include:

- operation name
- module
- version
- input schema
- output schema
- AI exposure metadata
- effect declarations
- required capabilities
- required policies
- idempotency
- determinism flag if relevant

## 13.2 Example introspection output

```json
{
  "operation": "billing.invoice.refund",
  "module": "billing",
  "version": "v1",
  "ai": {
    "exposed": true,
    "exposure": "internal",
    "title": "Refund invoice",
    "summary": "Refunds a previously charged invoice.",
    "invocation_class": "mutate",
    "requires_confirmation": true
  },
  "input_schema": {
    "type": "object",
    "properties": {
      "invoice_id": { "type": "string" },
      "reason": { "type": "string" }
    },
    "required": ["invoice_id"]
  },
  "effects": [
    "db.update.invoice",
    "event.publish.invoice.refunded",
    "audit.write"
  ],
  "capabilities": [
    "sql.write:billing",
    "event.publish:invoice.refunded",
    "audit.write"
  ],
  "policies": [
    "billing.invoice.refund"
  ]
}
```

---

# 14. MCP Adapter Requirements

Aegis SHOULD provide an MCP adapter that:

1. enumerates AI-exposed operations
2. converts operation schemas into MCP-friendly tool schemas
3. routes tool invocations to the execution engine
4. preserves:
   - subject
   - trace context
   - audit linkage
   - policy/capability enforcement

## 14.1 Adapter constraints

The adapter MUST NOT:
- expose hidden operations
- bypass policies
- fabricate capabilities
- hide side effects from metadata
- expose internal-only operations as public tools without explicit config

## 14.2 Tool invocation mapping

On invocation:

```text
MCP tool call
→ operation lookup
→ input normalization
→ execution context creation
→ policy check
→ capability check
→ handler execution
→ audit write
→ result normalization
```

---

# 15. Safety Model for AI Tooling

## 15.1 Classes of operations

Recommended safety classes:

- `read_safe`
- `read_sensitive`
- `mutate_safe`
- `mutate_sensitive`
- `admin_sensitive`

These MAY be derived from metadata or declared directly.

## 15.2 Side effect declaration is mandatory for mutating AI tools

An operation exposed to AI and classified as mutating SHOULD declare side effects.

## 15.3 Confirmation hints

For sensitive or irreversible actions, operation metadata SHOULD declare a confirmation requirement.

## 15.4 Human override support

Future implementations MAY support policies that require:
- a human approval token
- a second actor
- a just-in-time elevated grant

---

# 16. Resource Layer and AI Usability

The Resource Layer described in RFC 0002 makes Aegis especially suitable for AI use.

Because infrastructure access is abstracted as named resources, AI systems can reason at the right semantic level.

Example:

Instead of exposing raw S3 mechanics, an Aegis AI-facing operation can use:

```go
storage := exec.Resources.Storage("user-uploads")
```

The AI agent does not need to know:
- whether it is S3
- whether it is local disk
- whether it is encrypted
- whether a bind mount or virtual layer is in use

This reduces coupling and improves agent reasoning.

---

# 17. Recommended Metadata Additions to RFC 0002 Objects

This RFC recommends extending relevant structs with optional AI metadata.

## 17.1 OperationSpec extension

```go
type OperationSpec struct {
    // existing fields...
    AI *AIExposureSpec
}
```

## 17.2 AIExposureSpec

```go
type AIExposureSpec struct {
    Exposed              bool
    Exposure             string
    Title                string
    Summary              string
    Description          string
    UseCases             []string
    AvoidWhen            []string
    InvocationClass      string
    RequiresConfirmation bool
    SideEffects          []string
    Tags                 []string
}
```

## 17.3 ModuleManifest extension

```go
type ModuleManifest struct {
    // existing fields...
    AI *ModuleAISpec
}
```

---

# 18. Example Generated SKILLS.md

```md
# Skills

## storage.file_management

**Description**  
Manage file upload, retrieval, and deletion through the Aegis storage resource layer.

**Use when**
- a caller needs to save a file
- a caller needs to fetch a stored file
- a caller needs to remove an existing file

**Do not use when**
- the caller needs low-level storage driver operations
- the caller is attempting bucket-level administration
- the action requires infrastructure reconfiguration

**Operations**
- `storage.file.save`
- `storage.file.get`
- `storage.file.delete`

### storage.file.save
- Class: mutate
- Input: file path, file contents, metadata
- Output: stored file reference
- Side effects:
  - writes to configured storage backend
  - may emit audit records
- Confirmation required: no

### storage.file.delete
- Class: mutate_sensitive
- Input: file identifier
- Output: deletion result
- Side effects:
  - deletes stored object
  - writes audit record
- Confirmation required: yes
```

---

# 19. Minimal Implementation Plan

## Phase 1
- extend operation metadata with AI fields
- expose AI metadata in introspection output

## Phase 2
- build `SKILLS.md` generator
- support per-module skill grouping

## Phase 3
- build MCP adapter over AI-exposed operations

## Phase 4
- add safety/confirmation metadata and policy-aware tool exposure controls

---

# 20. Open Questions

1. Should Aegis generate only `SKILLS.md`, or also a machine-readable `skills.json`?
2. Should AI exposure metadata live in code, manifests, or both?
3. How opinionated should the generated markdown structure be?
4. Should one operation belong to multiple skills?
5. Should internal policy details be visible in generated AI docs?
6. Should confirmation requirements be enforced by runtime, client, or both?
7. How should public vs internal skill exposure be represented across environments?

---

# 21. Conclusion

Aegis should not merely support AI tooling by accident.  
It should support it **by design**.

By making Aegis:

- MCP-friendly
- introspectable
- operation-driven
- capability-aware
- documentation-generating

we get a runtime where applications can be safely and clearly consumed by AI systems.

That means Aegis-built services become easier to:

- expose as tools
- reason about
- audit
- govern
- evolve

The `SKILLS.md` generator is not a cosmetic addition.  
It is part of the interface contract between Aegis applications and AI agents.

---

# Appendix A — Suggested Positioning Sentence

**Aegis is an application runtime OS that is MCP-friendly by design, capable of generating AI-consumable skills metadata for every explicitly exposed operation.**

---

# Appendix B — Suggested Future Commands

```bash
aegis generate skills
aegis generate skills-json
aegis serve mcp
aegis introspect ai
```

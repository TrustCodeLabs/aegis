# RFC 0002 — Aegis Kernel, Capability System and Resource Layer
**Status:** Draft  
**Depends on:** RFC 0001  
**Last updated:** 2026-03-26

---

# TL;DR

This RFC defines the **core executable kernel** of Aegis, including:

- Kernel responsibilities and lifecycle
- Operation execution engine
- Capability system as syscall layer
- Resource Layer (virtual infrastructure)
- Driver model (pluggable infra backends)
- Binding system (config → runtime wiring)
- Enforcement rules (security + performance)

This document is **implementation-oriented** and can be directly used by an AI agent to build the first working version of Aegis Core.

---

# 1. Scope

This RFC covers ONLY:

- Kernel runtime
- Operation execution engine
- Capability system
- Resource abstraction layer
- Driver interfaces
- Binding system

It explicitly excludes:

- GraphQL adapter (future RFC)
- WebSocket adapter (future RFC)
- CLI (future RFC)

---

# 2. Kernel Responsibilities

The Aegis Kernel is responsible for:

1. Bootstrapping runtime
2. Loading modules
3. Resolving capabilities
4. Registering operations
5. Executing operations
6. Enforcing policies
7. Managing resources via drivers
8. Emitting audit + observability

---

# 3. Kernel Architecture

```text
Kernel
├── Module Loader
├── Operation Registry
├── Execution Engine
├── Capability Manager
├── Resource Manager
├── Policy Engine (interface only in v1)
├── Audit Writer (interface)
└── Observability Hooks
```

---

# 4. Kernel Lifecycle

## Boot Sequence

```text
Load config
→ Initialize core services
→ Load module manifests
→ Resolve capabilities
→ Register operations
→ Initialize resource drivers
→ Bind resources
→ Validate system
→ Start execution
```

---

# 5. Operation Execution Engine

## Core Function

```go
func (k *Kernel) Execute(ctx context.Context, opName string, input any) (any, error)
```

## Execution Steps

```text
1. Lookup operation
2. Build ExecutionContext
3. Validate capabilities
4. Evaluate policy
5. Validate input
6. Execute handler
7. Capture effects
8. Commit via Resource Layer
9. Emit audit
10. Emit telemetry
```

---

# 6. ExecutionContext (final form)

```go
type ExecutionContext struct {
    Operation   string
    Module      string
    Subject     Subject
    Capabilities GrantedCapabilities
    Resources   ResourceResolver
    Deadline    time.Time
    Metadata    map[string]any
}
```

---

# 7. Capability System (Syscall Layer)

## Definition

Capabilities are **explicit permissions** to perform actions.

They behave like **syscalls in an OS**.

---

## Capability Types (v1)

```text
storage.read:<bucket>
storage.write:<bucket>
cache.read:<ns>
cache.write:<ns>
sql.read:<boundary>
sql.write:<boundary>
event.publish:<topic>
secret.read:<key>
http.outbound:<service>
```

---

## Capability Enforcement

```go
func (m *CapabilityManager) Check(exec ExecutionContext, cap CapabilityRef) error
```

MUST:
- fail fast
- log denial
- emit metric

---

# 8. Resource Layer (Virtual Infrastructure)

## Concept

The Resource Layer abstracts infrastructure like an OS abstracts hardware.

Developers NEVER call concrete services directly.

---

## ResourceResolver

```go
type ResourceResolver interface {
    Storage(name string) StorageResource
    Cache(name string) CacheResource
    SQL(name string) SQLResource
    External(name string) HTTPResource
}
```

---

# 9. Storage Resource Interface

```go
type StorageResource interface {
    Write(ctx context.Context, path string, data []byte) error
    Read(ctx context.Context, path string) ([]byte, error)
    Delete(ctx context.Context, path string) error
}
```

---

# 10. Driver Model

Drivers implement actual infrastructure.

## Storage Driver

```go
type StorageDriver interface {
    Write(ctx context.Context, path string, data []byte) error
    Read(ctx context.Context, path string) ([]byte, error)
    Delete(ctx context.Context, path string) error
}
```

---

# 11. Driver Registry

```go
type DriverRegistry struct {
    storage map[string]StorageDriver
}
```

---

# 12. Binding System

## Example Config

```yaml
resources:
  storage:
    user-uploads:
      driver: s3
      bucket: my-bucket
```

---

## Binding Flow

```text
config → driver lookup → driver init → resource registration
```

---

# 13. Resource Manager

```go
type ResourceManager struct {
    storage map[string]StorageResource
}
```

---

# 14. Usage Example

```go
func (h Handler) Execute(ctx context.Context, exec ExecutionContext, input Input) (Output, error) {
    storage := exec.Resources.Storage("user-uploads")

    err := storage.Write(ctx, input.Path, input.Data)
    if err != nil {
        return Output{}, err
    }

    return Output{OK: true}, nil
}
```

---

# 15. Enforcement Rules

## MUST

- No direct infra access outside Resource Layer
- All resource usage MUST check capability
- All drivers MUST be registered

## MUST NOT

- Allow bypass via global variables
- Allow reflection-based escape hatches

---

# 16. Performance Constraints

- Resource lookup MUST be O(1)
- Capability check MUST be O(1)
- No reflection in hot path
- Zero allocation goal for execution pipeline

---

# 17. Error Model (Kernel)

```go
type KernelError struct {
    Code string
    Message string
}
```

---

# 18. Minimal Implementation Plan

## Step 1
- Kernel struct
- Operation registry

## Step 2
- Execution pipeline

## Step 3
- Capability manager

## Step 4
- Resource layer

## Step 5
- Storage driver (local)

## Step 6
- REST adapter (minimal)

---

# 19. Example Minimal Kernel

```go
type Kernel struct {
    ops map[string]Operation
    caps *CapabilityManager
    resources *ResourceManager
}
```

---

# 20. Conclusion

RFC 0002 defines the **real engine** of Aegis.

If implemented correctly, this creates:

- OS-like infra abstraction
- safe execution model
- pluggable infra
- enforceable security

This is the foundation for everything else.



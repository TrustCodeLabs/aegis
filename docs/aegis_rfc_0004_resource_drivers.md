# RFC 0004 — Resource Drivers, Bindings and Provider Model
**Status:** Draft  
**Depends on:** RFC 0001, 0002, 0002-A, 0003  
**Last updated:** 2026-03-26  
**Target version:** 0.1.0-alpha

---

# TL;DR

This RFC defines how Aegis virtualizes infrastructure:

- Resource Drivers (like OS drivers)
- Provider Model (pluggable infra backends)
- Binding System (config → runtime wiring)
- Layered resources (composition)
- Hot-swappable infrastructure (controlled)
- Multi-tenant resource isolation

This is what turns Aegis into a real **application runtime OS**.

---

# 1. Goals

Aegis MUST:

- abstract infrastructure behind resources
- support multiple providers per resource type
- allow runtime binding via config
- enforce capabilities at resource access
- support composition (layered resources)
- allow safe swapping of providers

---

# 2. Core Concepts

## Resource
Logical interface (storage, cache, sql, etc)

## Driver
Concrete implementation (S3, Redis, Postgres, local FS)

## Provider
Configured instance of a driver

## Binding
Mapping between logical resource and provider

---

# 3. Resource Types (v1)

- storage
- cache
- sql
- queue
- pubsub
- secrets
- http (outbound)

---

# 4. Driver Interface Example

## StorageDriver

```go
type StorageDriver interface {
    Init(config map[string]any) error
    Write(ctx context.Context, path string, data []byte) error
    Read(ctx context.Context, path string) ([]byte, error)
    Delete(ctx context.Context, path string) error
}
```

---

# 5. Driver Registry

```go
type DriverRegistry struct {
    storage map[string]StorageDriverFactory
}

type StorageDriverFactory func() StorageDriver
```

---

# 6. Binding System

## Example Config

```yaml
resources:
  storage:
    user-uploads:
      driver: s3
      config:
        bucket: my-bucket
```

---

# 7. Binding Resolution

```text
config → driver lookup → driver init → resource instance → registry
```

---

# 8. Resource Manager

```go
type ResourceManager struct {
    storage map[string]StorageResource
}
```

---

# 9. Layered Resources (Composition)

Example:

```yaml
storage:
  user-uploads:
    driver: layered
    layers:
      - driver: cache
      - driver: s3
      - driver: encryption
```

Execution order:

```text
write → encryption → cache → s3
read → cache → s3 → decrypt
```

---

# 10. Multi-Tenant Binding

```yaml
storage:
  user-uploads:
    tenant:
      A: s3-bucket-a
      B: s3-bucket-b
```

---

# 11. Capability Enforcement

Every resource access MUST:

- check capability
- register effect
- pass through policy

---

# 12. Hot Swapping

Drivers MAY be reloaded if:

- no active critical operations
- consistency guarantees satisfied

---

# 13. Failure Handling

Drivers MUST:

- return typed errors
- support retry classification
- expose health checks

---

# 14. Observability

Drivers MUST emit:

- latency
- errors
- operation count

---

# 15. Example Usage

```go
storage := exec.Resources.Storage("user-uploads")
err := storage.Write(ctx, "file.txt", data)
```

---

# 16. Implementation Plan

Phase 1:
- driver registry
- storage driver (local)

Phase 2:
- S3 driver
- cache driver

Phase 3:
- layered driver

Phase 4:
- multi-tenant bindings

---

# 17. Conclusion

RFC 0004 completes the OS model:

- RFC 0002 → kernel
- RFC 0003 → brain (policy/effects)
- RFC 0004 → infrastructure abstraction

Together, Aegis becomes:

**a true application runtime OS**

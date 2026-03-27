# Aegis Core

Aegis Core is the Go implementation of the runtime described in the RFCs under [`docs/`](docs/).

The repository already implements the core runtime model from RFC 0001, 0002, 0002-A, 0003 and the storage-focused parts of RFC 0004. It is not a rewrite target: the current shape is a reusable operations-first framework with policies, effects, introspection and a resource layer.

## Go Support

- `go.mod` targets `go 1.22`
- no `toolchain` directive is required
- the project is intended to compile and test without automatic toolchain download

## Implemented Today

- kernel builder and operation registry
- execution pipeline with `ExecutionContext`
- capability checks at operation entry and resource boundary
- policy engine with explainable decisions, confirmation and redaction
- effect planning, tracking and high-assurance denial for undeclared mutating effects
- module/operation/policy/capability/topology/effect introspection
- MCP tool generation and `SKILLS.md` generation for AI-exposed operations
- storage resource binding with provider model, layered storage, multi-tenant binding and guarded hot swap
- local storage driver with typed errors, health check and basic stats
- minimal reusable REST adapter in [`restadapter/`](restadapter/)
- runnable example API in [`sample_project/`](sample_project/)

## Explicit Placeholders

The following resource shapes exist for architectural completeness, but are not implemented as mature runtime resources yet:

- `Cache`
- `SQL`
- `External` HTTP resource

Current behavior:

- calls to these resources return `resource_not_implemented`
- there is no binding/driver stack for them yet
- storage is the only resource implemented end-to-end in v1

Also still out of scope in the current codebase:

- real cache/sql/queue/pubsub/secrets drivers
- transactional resource commit phase
- MCP transport server
- production-ready authn/authz adapters

## Capability Model

Aegis now keeps the two existing capability inputs, but with explicit precedence:

- `Subject.Capabilities`: identity claims or default capabilities carried with the subject
- `GrantedCapabilities` in context: explicit execution scope supplied by middleware, transport or caller

Resolution rule:

- if explicit `GrantedCapabilities` are present in context, they are the authoritative capability set
- otherwise Aegis falls back to `Subject.Capabilities`

Why this model:

- avoids ambiguous union semantics
- avoids privilege escalation from subject claims when a transport already scoped execution
- keeps capability checks O(1) through the effective map-backed set
- preserves visibility of both sources through `ExecutionContext.CapabilityResolution`

## Minimal Quick Start

```go
package main

import (
	"context"
	"fmt"

	"aegis"
	"aegis/drivers/localstorage"
)

type UploadInput struct {
	Path string
	Data []byte
}

type UploadOutput struct {
	OK bool
}

func main() {
	registry := aegis.NewDriverRegistry()
	_ = localstorage.Register(registry)

	module := aegis.NewModule(
		"uploads",
		aegis.DefineOperation[UploadInput, UploadOutput](aegis.OperationSpec[UploadInput, UploadOutput]{
			Name: "upload.write",
			RequiredCapabilities: []aegis.CapabilityRef{
				"storage.write:user-uploads",
			},
			Handler: func(ctx context.Context, exec aegis.ExecutionContext, input UploadInput) (UploadOutput, error) {
				if err := exec.Resources.Storage("user-uploads").Write(ctx, input.Path, input.Data); err != nil {
					return UploadOutput{}, err
				}
				return UploadOutput{OK: true}, nil
			},
		}),
	)

	kernel, err := aegis.NewBuilder(aegis.Config{
		Resources: aegis.ResourcesConfig{
			Storage: map[string]aegis.StorageBinding{
				"user-uploads": {
					Driver: "local",
					Config: map[string]any{
						"root": "./tmp/uploads",
					},
				},
			},
		},
	}).
		WithDriverRegistry(registry).
		WithModule(module).
		Build()
	if err != nil {
		panic(err)
	}

	ctx := aegis.WithSubject(context.Background(), aegis.Subject{
		ID:   "user-1",
		Type: "user",
	})
	ctx = aegis.WithCapabilityRefs(ctx, "storage.write:user-uploads")

	output, err := kernel.Execute(ctx, "upload.write", UploadInput{
		Path: "hello.txt",
		Data: []byte("world"),
	})
	if err != nil {
		panic(err)
	}

	fmt.Println(output.(UploadOutput).OK)
}
```

## REST Adapter

[`restadapter/`](restadapter/) is intentionally small. It gives you:

- generic JSON endpoint wrapping for small HTTP adapters
- request binding for typed JSON inputs
- context injection per request
- `kernel.Execute(...)` dispatch through `NewJSONHandler(...)`
- JSON response writing
- default HTTP error classification for kernel errors

It does not try to become a full HTTP framework.

## Sample Project

[`sample_project/`](sample_project/) is the reference composition:

- `main.go`: server entrypoint
- `internal/app`: application composition and kernel wiring
- `internal/httpapi`: demo-only HTTP adapters, request context mapping and route composition on top of [`restadapter/`](restadapter/)
- `internal/notes`: sample domain and module definition

The sample keeps only example wiring, demo endpoints and sample data. Reusable HTTP transport plumbing lives in the framework core under [`restadapter/`](restadapter/).

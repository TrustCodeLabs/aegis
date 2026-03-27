# Sample Project

Rode a API a partir desta pasta:

```bash
go run main.go
```

O servidor sobe por padrão em `http://localhost:8090`.

O exemplo demonstra:

- API HTTP com CRUD de notas
- policies de acesso e redaction
- effect tracking e introspection
- geração de `SKILLS.md`
- multi-tenant e hot swap de storage (`layered` -> `direct`)
- uso do adapter reutilizável [`aegis/restadapter`](../restadapter)

## Estrutura

- `main.go`: entrypoint e bootstrap do servidor HTTP
- `internal/app`: composição da aplicação, kernel Aegis e bindings de storage
- `internal/httpapi`: handlers de demonstração e mapeamento HTTP -> operação usando o adapter REST do core
- `internal/notes`: domínio de notas, módulo Aegis, effects e persistência em storage

## Headers úteis

- `X-Tenant-ID`: `team-a` ou `team-b` (`team-a` por padrão)
- `X-Role`: `viewer`, `editor` ou `admin` (`editor` por padrão)
- `X-Subject-ID`: identificador do ator da chamada
- `X-Confirm-Delete: true`: exigido no `DELETE /api/notes/{id}`

## Exemplos

Criar uma nota:

```bash
curl -s -X POST http://localhost:8090/api/notes \
  -H 'Content-Type: application/json' \
  -d '{"title":"Minha nota","content":"Olá Aegis","internal":"somente equipe"}'
```

Listar notas:

```bash
curl -s http://localhost:8090/api/notes
```

Buscar uma nota:

```bash
curl -s http://localhost:8090/api/notes/<id>
```

Atualizar uma nota:

```bash
curl -s -X PUT http://localhost:8090/api/notes/<id> \
  -H 'Content-Type: application/json' \
  -d '{"title":"Atualizada","content":"Novo conteúdo","internal":"novo detalhe"}'
```

Deletar uma nota com confirmação:

```bash
curl -s -X DELETE http://localhost:8090/api/notes/<id> \
  -H 'X-Confirm-Delete: true'
```

Trocar o modo de storage para `direct`:

```bash
curl -s -X POST http://localhost:8090/api/admin/storage/mode \
  -H 'X-Role: admin' \
  -H 'Content-Type: application/json' \
  -d '{"mode":"direct"}'
```

Ver o catálogo MCP-friendly:

```bash
curl -s http://localhost:8090/api/admin/mcp-tools -H 'X-Role: admin'
```

Ver o grafo/topologia introspectável do framework:

```bash
curl -s http://localhost:8090/api/admin/introspection/topology -H 'X-Role: admin'
```

Ver o `SKILLS.md` gerado pelo framework:

```bash
curl -s http://localhost:8090/api/admin/skills -H 'X-Role: admin'
```

# Universal Tenant Scope Design

- Date: 2026-07-27
- Project: `cool-admin-go-next`
- Status: Approved for implementation planning

## 1. Purpose

`cool-admin-go-next` is a general-purpose rapid-development admin framework. Tenant isolation therefore belongs in the framework CRUD and database access boundaries, not in individual business services.

The implementation must provide secure defaults without adding SQL parsing, regular-expression rewriting, per-request reflection, global locks, or extra database round trips. A newly registered tenant-aware CRUD resource must receive tenant isolation automatically.

## 2. Current State

The generic CRUD Runtime currently builds and executes raw SQL. GoFrame Model Hooks only apply to a specific `*gdb.Model`, so adding a Model Hook alone would not protect generic Add, Update, Delete, Info, List, or Page operations.

Base services currently add tenant conditions manually and inconsistently. Dict mostly falls back to the unscoped generic Runtime, and its custom reads and recursive deletes also lack a uniform tenant boundary.

The Midway reference implementation uses a customized TypeORM fork with global QueryBuilder callbacks. It has useful policy ideas but cannot be copied directly to GoFrame. In particular, its missing-context behavior is fail-open, raw SQL bypasses the subscriber, and its `noTenant` helper mutates shared request state.

## 3. Decisions

### 3.1 Canonical tenant values

Persisted tenant values have one canonical representation:

| Value | Meaning |
| --- | --- |
| `tenant_id IS NULL` | Platform-owned record or authenticated platform user |
| `tenant_id > 0` | Record or user belonging to a concrete tenant |
| `tenant_id = 0` | Legacy value only; normalized to `NULL` |

New writes must never persist `tenant_id = 0`. Existing zero values will be normalized by an explicit, repeatable migration step. During token migration, a legacy zero claim is accepted as platform scope and newly issued tokens use the nullable representation.

### 3.2 Scope states

The central tenant package distinguishes four states rather than overloading a numeric value:

| Scope | Behavior |
| --- | --- |
| Tenant | Restrict reads and mutations to one positive tenant ID |
| Platform | Authenticated platform user; no automatic tenant predicate, compatible with Midway behavior |
| Bypass | Explicit internal cross-tenant operation such as Seed, Job, login, migration, or controlled administration |
| Missing | No authenticated or internal scope; reject access to tenant-aware resources |

Platform and Bypass are deliberately separate even though neither normally adds a tenant predicate. This makes authorization and auditing explicit and prevents a missing request context from becoming an accidental bypass.

### 3.3 Public access

Public endpoints do not receive implicit Bypass. A public query must explicitly declare one of these policies:

- GlobalOnly: read only `tenant_id IS NULL` records.
- TenantFromTrustedInput: resolve a tenant through a trusted server-side mechanism, not an arbitrary request field.
- Bypass: reserved for reviewed infrastructure paths and never selected by request input.

### 3.4 Platform writes

An ordinary platform-scoped Add creates a platform record with `tenant_id=NULL`. Creating data for a concrete tenant requires an explicit `ForTenant(id)` scope. Client-supplied `tenantId` remains read-only and never selects the write scope.

## 4. Architecture

### 4.1 Central tenant package

`cool/tenant` owns:

- Tenant scope types and context propagation.
- Resolution from authenticated user context.
- Explicit `WithoutTenant` and `ForTenant` derived contexts.
- Scope validation for tenant-aware resources.
- Structured predicate application for a table or alias.
- Construction of scoped GoFrame Models for custom ORM paths.
- Insert, Update, and Delete Model Hooks used as mutation defense.

The package must not depend on `modules/base`; Base authentication may populate framework auth context, but Dict and future modules consume tenant policy through `cool/tenant`.

`WithoutTenant` creates a derived context. It must not mutate a user object or global state, and its effect ends with the derived context even when the operation fails.

### 4.2 Compiled resource metadata

CRUD Resource compilation derives and stores tenant metadata once during application startup:

- Whether the resource is tenant-aware.
- The JSON field and database column names.
- Whether the resource has an explicit non-tenant override.
- The allowed public policy, if any.

Metadata is derived from the registered model definition, not table naming or runtime reflection. A resource declared tenant-aware without a valid nullable tenant column makes startup fail with a precise configuration error.

The default is tenant-aware when a resource includes the framework `tenant_id` base field. A deliberate opt-out must be explicit in resource metadata.

### 4.3 Generic CRUD Runtime Hook

The generic Runtime keeps its validated, parameterized SQL path. A mandatory tenant planning Hook runs before SQL generation:

- Add and AddMany remove client tenant fields and inject the resolved write scope.
- Info, List, Page, and Count append a structured tenant predicate.
- Update, UpdateMany, and Delete append the same predicate to the mutation condition.
- Platform and explicit Bypass omit the predicate after their scope has been authenticated or explicitly selected.
- Missing scope fails before SQL execution.

Tenant values are always bound parameters. The implementation must not parse or rewrite generated SQL text.

### 4.4 Scoped GoFrame Models

Custom ORM services obtain Models from the tenant package rather than calling `DB.Model` or `TX.Model` directly for tenant-aware entities.

The Model factory applies Select scope structurally before query generation. Insert, Update, and Delete Hooks enforce the write scope as a second line of defense. Transactional Models must remain bound to the caller's transaction.

The Select Hook must not rewrite final SQL because GoFrame exposes SQL text rather than a safe structured Select condition at that stage.

### 4.5 Raw SQL and joins

Complex joins and raw SQL use an explicit scope helper with a required table alias. Each tenant-bearing side of a join must have its ownership rule stated in the query or validated through a scoped parent record.

Tenant-sensitive module code must use the framework raw-query wrapper or scoped Model factory. A repository test scans module service code for newly introduced direct raw database entry points and requires a reviewed allowlist for infrastructure operations. This is a development guard in addition to integration tests; it is not represented as a database-global interceptor.

Relation tables without `tenant_id` are not given a synthetic predicate. The service first validates or locks the owning records through a tenant-scoped query, then mutates relation rows by the validated IDs in the same transaction.

## 5. Request Data Flow

1. Login or refresh reads the current database user snapshot and issues a nullable tenant claim.
2. Authentication validates the token and session, then writes a typed user context that distinguishes an authenticated platform user from missing authentication.
3. The CRUD Runtime resolves one TenantScope before request data is mapped.
4. Resource metadata decides whether scope enforcement is required.
5. The Runtime Hook removes untrusted tenant input and adds the structured write value or predicate.
6. The query executes without an additional tenant lookup.
7. Mutation results are validated before ModifyAfter or other side effects run.

Custom services follow the same scope resolution and pass it to a scoped Model or alias-qualified raw-query helper.

## 6. Mutation Semantics

### 6.1 Single-row operations

- Cross-tenant Info returns not found.
- Cross-tenant Update or Delete produces zero affected rows and returns not found.
- ModifyAfter is not called when no row was changed.
- The response does not reveal whether the ID exists in another tenant.

### 6.2 Batch operations

Batch writes are atomic. Every item uses the same resolved scope and transaction.

- AddMany overwrites every item's tenant value with the resolved write scope.
- UpdateMany rolls back the entire batch if any requested row is unavailable in scope.
- Delete normalizes duplicate IDs, requires every requested ID to be available in scope, and rolls back on a partial match.

Platform and Bypass operations follow the same affected-row validation; bypassing tenant filtering does not bypass consistency checks.

### 6.3 Recursive and cascading deletes

Recursive deletes, including Dict trees, first obtain the complete descendant set through scoped queries. The implementation must not traverse into another tenant's nodes. All parent, child, and dependent-row operations execute in one transaction.

## 7. Error Handling

The tenant package returns typed errors that the existing HTTP error layer maps consistently:

| Condition | External behavior |
| --- | --- |
| Missing authentication on tenant-aware resource | 401 or the existing authentication error |
| Authenticated caller lacks an allowed scope | 403 |
| Cross-tenant or absent target ID | Not found using the existing CRUD contract |
| Invalid tenant metadata | Application startup failure |
| Invalid explicit tenant ID | Validation error before SQL execution |
| Undeclared tenant-sensitive raw access | Test/build failure; runtime wrapper also rejects Missing scope |

Internal errors retain GoFrame error stacks. Tenant IDs may be recorded in structured audit context, but authorization errors must not include foreign row data.

## 8. Performance Requirements

The request hot path is limited to:

- One context lookup.
- One scope-kind branch.
- One compiled metadata lookup.
- At most one parameterized predicate/value append per tenant-aware base table.

The implementation adds no tenant authorization query, regular expression, SQL parser, per-request reflection, global mutex, or shared mutable request state. Compiled resource metadata is immutable after startup and safe for concurrent reads.

Existing indexes on `tenant_id` remain required. Query plans for tenant-scoped List, Page, Update, and Delete are verified against representative MySQL data. Composite indexes are considered per resource only where its real filter and ordering pattern justifies them.

## 9. Initial Implementation Scope

The first implementation increment includes:

1. Nullable tenant identity and legacy zero compatibility.
2. The central tenant scope package and derived contexts.
3. Compiled CRUD tenant metadata.
4. The mandatory generic Runtime tenant Hook for all CRUD actions.
5. A scoped GoFrame Model factory and mutation Hooks.
6. Complete Dict migration, including Type, Info, public data reads, and recursive deletes.
7. Compatibility verification for Base services that already add manual predicates.
8. Raw-access scanning with a narrow reviewed allowlist.
9. Unit, race, and real MySQL two-tenant integration coverage.

Complex Base joins are audited in this increment but are migrated incrementally where existing manual predicates are already correct. No path is described as universally protected until its raw and custom handlers have been verified.

## 10. Testing Strategy

### 10.1 Tenant package tests

- Tenant, Platform, Bypass, and Missing resolution.
- Nullable claims and legacy zero normalization.
- `ForTenant` and `WithoutTenant` context derivation.
- Nested scopes, errors, and concurrent request isolation.

### 10.2 CRUD Runtime tests

- Add and AddMany tenant overwrite.
- Info, List, Page, and Count predicates.
- Update, UpdateMany, and Delete affected-row behavior.
- Platform and explicit Bypass behavior.
- Missing scope rejection.
- Batch rollback and ModifyAfter ordering.

### 10.3 Database integration tests

MySQL fixtures contain tenant A, tenant B, and platform rows. Tests prove that tenant A cannot read, count, update, delete, or recursively reach tenant B, while the authenticated platform scope retains compatible cross-tenant visibility.

Dict Type and Info are the first full matrix. Tests include their custom Data and Types reads, tree traversal, cascading delete, and forged tenant input.

### 10.4 Framework boundary tests

- Scoped Model behavior inside and outside transactions.
- GoFrame Insert, Update, Delete, Count, and future soft-delete paths.
- Alias-qualified joins and parent-validated relation mutations.
- Raw SQL scanner and allowlist behavior.
- Public GlobalOnly endpoints.
- Token refresh after a user's tenant changes.

The normal unit suite, race suite, and opt-in real MySQL integration suite must pass before rollout.

## 11. Rollout and Compatibility

Implementation is introduced behind one framework-level tenant enable switch, but enabled tests exercise the complete policy. The switch is not treated as authorization; production deployments that require tenant isolation must fail startup if it is disabled contrary to deployment policy.

The data migration converts zero tenant values to `NULL` before nullable claims become canonical. Existing positive tenant values are unchanged. Old access tokens carrying zero are accepted during the transition and replaced on refresh or login.

Manual Base predicates remain temporarily valid and should produce equivalent conditions for positive tenants. They are removed only after each custom path has scoped Model or raw-helper coverage and integration tests.

## 12. Non-Goals

This design does not implement:

- Soft delete or the Recycle module.
- A database proxy or SQL parser.
- Automatic tenant inference from arbitrary request parameters.
- Automatic scoping of every possible third-party raw SQL call.
- Per-tenant uniqueness migrations for all existing unique indexes.

Unique-index semantics are reviewed per resource. A separate migration design is required before changing a globally unique field to tenant-local uniqueness.

## 13. Acceptance Criteria

The implementation is accepted when:

1. A new BaseFields-backed generic CRUD resource is isolated without service-specific tenant code.
2. Tenant A cannot observe or mutate tenant B through any generic CRUD action.
3. Client tenant fields never determine persisted ownership.
4. Authenticated platform users retain the approved cross-tenant behavior while platform records use `NULL` canonically.
5. Missing context never becomes implicit bypass.
6. Internal cross-tenant operations are explicit in source through Bypass or `ForTenant`.
7. Dict passes the complete two-tenant CRUD and recursive-delete matrix.
8. Tenant enforcement adds no extra database round trip or SQL text rewriting.
9. Unit, race, and real MySQL integration tests pass.

---
name: typescript-pro
description: "🔷 Advanced generics, strict configs, and type-safe architecture patterns. Use this skill whenever the user's task involves typescript, types, generics, frontend, node, or any related topic, even if they don't explicitly mention 'TypeScript Pro'."
---

# 🔷 TypeScript Pro

> **Category:** development | **Tags:** typescript, types, generics, frontend, node

Leverage the full power of TypeScript's type system to catch bugs at compile time, not runtime. Treat types as documentation that the compiler enforces -- every `any` is a missed opportunity.

## When to Use

- Designing generic libraries, utilities, or shared packages
- Tightening a codebase from loose types to strict mode
- Building type-safe API clients, form handlers, or state machines
- Refactoring JavaScript to TypeScript or upgrading `tsconfig` strictness
- Creating branded/nominal types for domain modeling

## Core Principles

- **Enable strict mode always.** Set `strict: true` in `tsconfig.json`. Treat `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`, and `verbatimModuleSyntax` as non-negotiable in new projects.
- **Eliminate `any`.** Use `unknown` for truly unknown data, then narrow with type guards. Reserve `any` only for legacy interop boundaries.
- **Encode invariants in types.** If a value cannot be negative, make the type prevent it. If a function requires a non-empty array, express that in the signature.
- **Prefer inference over annotation.** Let TypeScript infer return types and variable types. Annotate function parameters and public API boundaries explicitly.
- **Keep type utilities small and composable.** Build complex types from simple building blocks, not monolithic conditional chains.

## Workflow

1. **Audit the tsconfig.** Confirm `strict: true` and evaluate additional flags like `noUncheckedIndexedAccess`.
2. **Model the domain.** Define branded types, discriminated unions, and enums before writing logic.
3. **Write the contract.** Define function signatures, generics, and overloads. Let the types guide the implementation.
4. **Implement with narrowing.** Use `in`, `typeof`, discriminant checks, and exhaustive switches -- avoid casts.
5. **Validate at boundaries.** Parse external data with Zod, io-ts, or manual type guards at API/IO edges.
6. **Refactor with the compiler.** Rename, restructure, and tighten types -- let red squiggles reveal every call site that needs updating.

## Examples

### Branded Types for Domain Safety

```typescript
// Before: primitive obsession -- easy to mix up IDs
function getUser(id: string): User { ... }
getUser(orderId); // no error, but wrong!

// After: branded types prevent misuse
type UserId = string & { readonly __brand: unique symbol };
type OrderId = string & { readonly __brand: unique symbol };

const toUserId = (id: string): UserId => id as UserId;

function getUser(id: UserId): User { ... }
getUser(orderId); // compile error
```

### Exhaustive Discriminated Unions

```typescript
type Shape =
  | { kind: "circle"; radius: number }
  | { kind: "rect"; width: number; height: number };

function area(s: Shape): number {
  switch (s.kind) {
    case "circle": return Math.PI * s.radius ** 2;
    case "rect":   return s.width * s.height;
    default:       return s satisfies never; // compile error if a variant is missed
  }
}
```

## Common Patterns

### Constrained Generics with Defaults

```typescript
function merge<T extends Record<string, unknown>>(base: T, override: Partial<T>): T {
  return { ...base, ...override };
}
```

### Mapped Types for Form State

```typescript
type FormErrors<T> = { [K in keyof T]?: string };
type FormTouched<T> = { [K in keyof T]?: boolean };
```

### Template Literal Types for Routes

```typescript
type Method = "GET" | "POST" | "PUT" | "DELETE";
type Route = `/api/${string}`;
type Endpoint = `${Method} ${Route}`; // "GET /api/users", etc.
```

### Conditional Type Extraction

```typescript
type UnwrapPromise<T> = T extends Promise<infer U> ? U : T;
type Result = UnwrapPromise<Promise<string>>; // string
```

### Const Assertions for Literal Inference

```typescript
const ROLES = ["admin", "editor", "viewer"] as const;
type Role = (typeof ROLES)[number]; // "admin" | "editor" | "viewer"
```

## Anti-Patterns

- **Casting instead of narrowing.** `value as MyType` silences the compiler without safety. Use type guards or `satisfies` instead.
- **Exporting `any` from library boundaries.** Downstream consumers lose all type safety. Export precise types or `unknown`.
- **Overusing enums.** Prefer `as const` objects or union literals -- they are more tree-shakable and interoperate better with plain JS.
- **Giant conditional types.** If a type spans 20+ lines, break it into named helpers. Types should be readable too.
- **Ignoring `strictNullChecks`.** Optional chaining hides bugs when nullability is not tracked. Keep strict null checks on and handle every `| undefined`.
- **Using `Object`, `Function`, or `{}`.** These are almost never what you want. Use `Record<string, unknown>`, `(...args: unknown[]) => unknown`, or a specific interface.

## Capabilities

- typescript
- type-system
- generics
- strict-mode
- architecture
- branded-types
- conditional-types
- mapped-types
- template-literal-types
- discriminated-unions

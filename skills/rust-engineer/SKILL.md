---
name: rust-engineer
description: "🦀 Write safe, idiomatic Rust -- ownership-driven design, async Tokio services, error handling with thiserror/anyhow, FFI, and zero-cost abstractions. Activate for any Rust coding, architecture, borrow checker issues, or crate design."
---

# 🦀 Rust Engineer

Write safe, fast, idiomatic Rust. Let the borrow checker guide the design rather than fighting it -- ownership constraints produce better architectures.

## Core Principles

- **Own the ownership model.** Design data flow so ownership is clear. Prefer moving values over sharing references when lifetimes get complex.
- **Make illegal states unrepresentable.** Use enums and the type system to encode state machines. If a function cannot fail, do not return `Result`.
- **Zero-cost abstractions first.** Use generics and monomorphization over trait objects. Reach for `dyn Trait` only when dynamic dispatch is genuinely needed.
- **Handle errors precisely.** Use `thiserror` for library error types, `anyhow` for application code. Never `unwrap()` in library code; reserve it for prototypes and tests.
- **Unsafe is an audit boundary, not a shortcut.** Minimize `unsafe` blocks, document every safety invariant, and wrap unsafe code behind safe APIs.

## Workflow

1. **Model with types.** Define enums, structs, and traits before writing logic. Use `#[non_exhaustive]` on public enums.
2. **Establish ownership.** Decide who owns each piece of data. Draw the ownership tree before coding complex modules.
3. **Implement with the borrow checker.** Write the straightforward version first. If lifetimes grow complex, restructure ownership rather than adding lifetime annotations everywhere.
4. **Add error handling.** Define domain error enums with `thiserror`. Propagate with `?`. Convert foreign errors at module boundaries.
5. **Write tests alongside code.** Use `#[cfg(test)]` modules, `proptest` for property-based testing, and `tokio::test` for async tests.
6. **Profile before optimizing.** Use `cargo flamegraph`, `criterion` benchmarks, or `perf` before micro-optimizing. Measure, do not guess.

## Examples

### Ownership-Driven Design

```rust
// Before: fighting the borrow checker with indices
struct Graph {
    nodes: Vec<Node>,
    edges: Vec<(usize, usize)>, // fragile index-based references
}

// After: arena-based ownership with slotmap
use slotmap::{SlotMap, new_key_type};

new_key_type! { pub struct NodeKey; }

struct Graph {
    nodes: SlotMap<NodeKey, Node>,
    edges: Vec<(NodeKey, NodeKey)>, // stable keys, no dangling refs
}
```

### Structured Error Handling

```rust
use thiserror::Error;

#[derive(Error, Debug)]
pub enum AppError {
    #[error("database query failed")]
    Database(#[from] sqlx::Error),
    #[error("invalid input: {0}")]
    Validation(String),
    #[error("resource {id} not found")]
    NotFound { id: String },
}

// In application code, use anyhow for ad-hoc context:
use anyhow::Context;
let config = std::fs::read_to_string("config.toml")
    .context("failed to read config file")?;
```

## Common Patterns

### Trait-Based Abstraction

```rust
pub trait Repository: Send + Sync {
    async fn find(&self, id: &str) -> Result<Option<Item>, AppError>;
    async fn save(&self, item: &Item) -> Result<(), AppError>;
}

// Concrete impl for production; mock impl for tests.
```

### Builder Pattern and Newtypes

```rust
// Builder: use `bon` or `typed-builder` crate for derive-based builders.
// Manual builder when you need validation in build():
#[derive(Default)]
pub struct ServerConfigBuilder { port: Option<u16>, workers: Option<usize> }
impl ServerConfigBuilder {
    pub fn port(mut self, p: u16) -> Self { self.port = Some(p); self }
    pub fn build(self) -> Result<ServerConfig, &'static str> { /* validate + construct */ }
}

// Newtype: prevent mixing up IDs, emails, etc. at the type level
pub struct UserId(uuid::Uuid);
pub struct Email(String);
impl Email {
    pub fn parse(s: &str) -> Result<Self, ValidationError> { /* validate, wrap */ }
}
```

### Async Rate-Limited Task Spawning

```rust
use tokio::sync::Semaphore;
use tokio::task::JoinSet;
use std::sync::Arc;

async fn process_batch(items: Vec<Item>) -> Vec<Result<Output, AppError>> {
    let semaphore = Arc::new(Semaphore::new(10)); // max 10 concurrent
    let mut set = JoinSet::new();

    for item in items {
        let permit = semaphore.clone().acquire_owned().await.unwrap();
        set.spawn(async move {
            let result = process_item(item).await;
            drop(permit); // release slot when done
            result
        });
    }

    let mut results = Vec::new();
    while let Some(res) = set.join_next().await {
        results.push(res.expect("task panicked"));
    }
    results
}
```

### Exhaustive Pattern Matching

```rust
match event {
    Event::Connected { peer } => handle_connect(peer),
    Event::Message { peer, data } => handle_message(peer, data),
    Event::Disconnected { peer, reason } => handle_disconnect(peer, reason),
    // No wildcard -- compiler ensures new variants are handled.
}
```

## Output Template: Crate Architecture Recommendation

```
## Crate: [name]
- **Purpose:** [single sentence]
- **Public API surface:** [key types and traits]
- **Error strategy:** thiserror (lib) / anyhow (bin)
- **Async runtime:** tokio / none
- **Key dependencies:** [crate -> reason]
- **Module layout:**
  src/lib.rs        -- re-exports, top-level docs
  src/domain/       -- core types, no I/O
  src/infra/        -- database, HTTP, file I/O
  src/error.rs      -- AppError enum
- **Testing:** unit (inline), integration (tests/), property (proptest)
- **Unsafe audit:** [none | list locations + invariants]
```

## Anti-Patterns

- **`unwrap()` in library code.** Propagate errors with `?` or return `Option`. Panics in libraries break caller control flow.
- **Lifetime gymnastics.** If a struct needs three lifetime parameters, reconsider ownership. Clone strategically or use `Arc` rather than annotating the world.
- **Premature `unsafe`.** Almost every problem has a safe solution. Reach for `unsafe` only after exhausting safe alternatives, and always document the invariant.
- **`String` everywhere.** Use `&str` for borrowed data, `Cow<'_, str>` when ownership is conditional, and `String` only when ownership is needed.
- **Ignoring `clippy`.** Run `cargo clippy -- -W clippy::pedantic` regularly. Suppress lints individually with justification, never globally.
- **Blocking in async context.** Never call blocking I/O inside a Tokio task. Use `tokio::task::spawn_blocking` or `tokio::fs` instead.


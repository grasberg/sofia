---
name: golang-pro
description: "🐹 Goroutines, channels, microservices, and production-grade Go services. Use this skill whenever the user's task involves go, golang, concurrency, microservices, cli, or any related topic, even if they don't explicitly mention 'Go Developer'."
---

# 🐹 Go Developer

> **Category:** development | **Tags:** go, golang, concurrency, microservices, cli

Idiomatic Go following Effective Go guidelines. Check the standard library first before reaching for third-party packages -- `net/http`, `encoding/json`, `database/sql`, and `testing` cover more than most developers expect.

## When to Use

- Building HTTP services and microservices in Go
- Concurrent programming with goroutines and channels
- CLI tool development with cobra/viper
- Writing comprehensive Go tests
- Designing Go interfaces and error handling

## Approach

1. **Handle errors explicitly** -- return `error` as the last return value. Wrap errors with `fmt.Errorf("context: %w", err)` to build an error chain that callers can inspect with `errors.Is()` and `errors.As()`. Never ignore errors silently.
2. **Design concurrency with structure** -- use goroutines for concurrent work, channels for communication, and `context.Context` for cancellation propagation. Every goroutine must have a clear shutdown path. Use `sync.WaitGroup` to wait for completion, `sync.Once` for one-time initialization.
3. **Accept interfaces, return structs** -- define small interfaces at the call site (consumer), not the implementation. The standard `io.Reader`/`io.Writer` pattern is the gold standard. This enables easy testing with mock implementations.
4. **Build HTTP services cleanly** -- use `http.ServeMux` (Go 1.22+ has method routing) or chi for routing. Middleware for cross-cutting concerns (logging, auth, metrics). Implement graceful shutdown with `signal.NotifyContext`.
5. **Write table-driven tests** -- define test cases as a slice of structs with input and expected output. Run each case as a subtest with `t.Run()`. Use `go test -race` to detect data races.
6. **Build CLI tools with cobra** -- commands, flags, and shell completions. Use viper for configuration with env vars, config files, and flag defaults in priority order.
7. **Optimize with data** -- profile with `pprof`, benchmark with `testing.B`, and trace with `runtime/trace`. Use worker pools for bounded concurrency.

## Examples

**Graceful HTTP server shutdown:**
```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
defer stop()

srv := &http.Server{Addr: ":8080", Handler: mux}

go func() {
    if err := srv.ListenAndServe(); err != http.ErrServerClosed {
        log.Fatalf("server error: %v", err)
    }
}()

<-ctx.Done()
shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
srv.Shutdown(shutdownCtx)
```

**Table-driven test:**
```go
func TestAdd(t *testing.T) {
    tests := []struct {
        name     string
        a, b     int
        expected int
    }{
        {"positive", 2, 3, 5},
        {"negative", -1, -2, -3},
        {"zero", 0, 0, 0},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := Add(tt.a, tt.b)
            if got != tt.expected {
                t.Errorf("Add(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.expected)
            }
        })
    }
}
```

**Error wrapping chain:**
```go
func GetUser(id int) (*User, error) {
    row := db.QueryRow("SELECT * FROM users WHERE id = $1", id)
    var u User
    if err := row.Scan(&u.ID, &u.Name); err != nil {
        return nil, fmt.Errorf("get user %d: %w", id, err)
    }
    return &u, nil
}
// Caller: if errors.Is(err, sql.ErrNoRows) { ... }
```

## Common Patterns

- **`context.Context`** as first parameter for cancellation, deadlines, and request-scoped values
- **`sync.Pool`** for reducing GC pressure on frequently allocated objects
- **Functional options** for flexible constructors: `NewServer(WithPort(8080), WithLogger(log))`

**`embed.FS` for bundling static files:**
```go
//go:embed static/*
var staticFiles embed.FS

mux.Handle("/static/", http.FileServer(http.FS(staticFiles)))
```

**`slog` structured logging (Go 1.21+):**
```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
logger.Info("request handled", "method", r.Method, "path", r.URL.Path, "duration_ms", elapsed.Milliseconds())
// Output: {"time":"...","level":"INFO","msg":"request handled","method":"GET","path":"/api/users","duration_ms":12}
```

**`sync.Map` for concurrent read-heavy maps:**
```go
var cache sync.Map
cache.Store("key", value)
if v, ok := cache.Load("key"); ok { /* use v */ }
// Use sync.Map only when keys are stable and reads dominate writes. Otherwise, use sync.RWMutex + map.
```

**`atomic.Pointer` for lock-free config reload (Go 1.19+):**
```go
var cfgPtr atomic.Pointer[Config]
cfgPtr.Store(&Config{Port: 8080})
// Reader (no lock): cfg := cfgPtr.Load()
// Writer (on reload): cfgPtr.Store(&newConfig)
```

## Anti-Patterns

- Launching goroutines without a shutdown mechanism -- leaked goroutines accumulate memory and file descriptors. Always pair goroutine creation with a way to stop it (context, done channel, or WaitGroup).
- Using `panic` for expected errors -- panic is for programmer bugs (nil pointer, index out of bounds), not for file-not-found or network timeouts. Return errors instead.
- Overly large interfaces -- Go interfaces should be small (1-3 methods). Large interfaces are hard to implement and test. If an interface has 10 methods, split it.
- Channel overuse -- not every communication needs a channel. A `sync.Mutex` protecting a shared variable is simpler and faster for basic mutual exclusion.

## Guidelines

- Run `go vet` and `golangci-lint` on every commit. They catch common mistakes the compiler misses.
- Keep packages small and focused. A package named `utils` is a code smell -- the functions belong somewhere more specific.
- Use `go test -race ./...` in CI. Data races are among the hardest bugs to debug and the easiest to detect with this flag.

## Output Template

```
## Service Architecture: [Service Name]

### Structure
cmd/
  server/main.go        -- Entrypoint, signal handling, DI wiring
internal/
  handler/              -- HTTP handlers (accept interfaces)
  service/              -- Business logic (pure functions where possible)
  repository/           -- Database access (interface + implementation)
  model/                -- Domain types (no external dependencies)
pkg/
  middleware/            -- Reusable HTTP middleware (logging, auth, metrics)

### Key Decisions
| Decision            | Choice          | Rationale                    |
|---------------------|-----------------|------------------------------|
| Router              | stdlib ServeMux | Go 1.22+ method routing      |
| Database            | pgx             | Native PostgreSQL, pool mgmt |
| Logging             | slog            | Stdlib, structured, zero-dep |
| Config              | envconfig       | 12-factor, env var-based     |

### Concurrency Model
[Worker pool / Fan-out/fan-in / Pipeline -- describe goroutine lifecycle]

### Graceful Shutdown Order
1. Stop accepting new HTTP connections
2. Drain in-flight requests (30s timeout)
3. Close database connections
4. Flush logger
```

## Capabilities

- golang
- concurrency
- microservices
- cli
- networking

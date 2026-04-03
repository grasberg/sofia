---
name: fullstack-dev
description: "🚀 Ships vertical slices from React/Next.js UI through API routes to Prisma/database, with auth and deployment. Activate for anything involving full-stack apps, API design, tRPC, database schemas, monorepos, or end-to-end TypeScript projects."
---

# 🚀 Full-Stack Developer

Think in round-trips -- every button click triggers a chain from UI to API to database and back. Ship working vertical slices, not disconnected layers.

## Core Principles

- **Own the full round-trip.** A frontend dev who "just does UI" ships broken features. Trace every interaction from click to database row and back. You catch integration bugs others miss.
- **TypeScript end-to-end.** Shared types between client and server eliminate an entire class of bugs. Use tRPC or generated OpenAPI types to make the API contract a compile-time check.
- **Server components by default (Next.js).** Fetch data on the server, send HTML to the client. Only add `"use client"` when you need interactivity. This cuts bundle size and eliminates loading spinners.
- **Validate at the boundary.** Use Zod schemas at API entry points. Never trust client input, even from your own frontend. Validation schemas double as documentation.
- **Database access through an ORM layer.** Prisma or Drizzle give you typed queries, migrations, and protection against SQL injection. Raw SQL is for performance-critical queries only.
- **Fail visibly.** Every API call needs error handling on both sides. Server returns structured errors; client shows actionable messages. Silent failures are the worst bugs.

## Workflow

1. **Define the data model.** Start with the Prisma/Drizzle schema. The database shape drives everything else.
2. **Build the API layer.** Create route handlers with Zod validation. Test with curl or Postman before touching the UI.
3. **Wire up the frontend.** Use React Query or SWR for server state. Keep client state (forms, modals) separate from server state (user data, lists).
4. **Add auth.** Integrate NextAuth.js or Lucia. Protect API routes with middleware, not per-handler checks.
5. **Handle errors and edge cases.** Add error boundaries, loading states, and empty states. Test the sad paths.
6. **Optimize.** Code-split heavy routes, lazy-load below-the-fold components, optimize images with `next/image`.

## Examples

### Project Structure (Next.js App Router + Prisma)

```
my-app/
  src/
    app/
      layout.tsx             # Root layout with providers
      page.tsx               # Landing page (server component)
      dashboard/
        page.tsx             # Dashboard (server component, fetches data)
        _components/
          chart.tsx           # "use client" -- needs interactivity
    lib/
      db.ts                  # Prisma client singleton
      auth.ts                # NextAuth config
      validations/
        user.ts              # Zod schemas shared by API + forms
    server/
      routers/
        user.ts              # tRPC or API route handlers
  prisma/
    schema.prisma
    migrations/
```

### API Route Pattern (Zod + Auth + Prisma)

```typescript
// src/app/api/projects/route.ts -- Pattern: validate -> auth -> execute -> respond
const CreateProjectSchema = z.object({ name: z.string().min(1).max(100) });

export async function POST(req: NextRequest) {
  const session = await getServerSession();
  if (!session) return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  const parsed = CreateProjectSchema.safeParse(await req.json());
  if (!parsed.success) return NextResponse.json({ error: parsed.error.flatten() }, { status: 400 });
  const project = await db.project.create({ data: { ...parsed.data, ownerId: session.user.id } });
  return NextResponse.json(project, { status: 201 });
}
```

**Auth middleware:** Use `withAuth` in `src/middleware.ts` with path matchers -- never per-handler auth checks.

## Output Templates

### When recommending project architecture:

```
## Recommended Stack
- **Frontend:** Next.js 14 (App Router) + TypeScript + Tailwind
- **Backend:** Next.js API routes (or tRPC for type-safe RPC)
- **Database:** PostgreSQL via Prisma ORM
- **Auth:** NextAuth.js with [provider]

## Data Model (key entities)
[Prisma schema snippet]

## API Design (key endpoints)
| Method | Path | Purpose | Auth |
|--------|------|---------|------|
| POST   | /api/X | Create X | Yes |

## Key Decisions
- [Decision]: [Rationale]
```

## Common Patterns

- **Optimistic updates.** Update the UI immediately, then sync with the server. Roll back on failure. React Query's `onMutate` handles this cleanly.
- **Server-side pagination.** Never fetch all rows. Use cursor-based pagination (`where: { id: { gt: cursor } }, take: 20`) for stable results.
- **Environment-based config.** Use `env.mjs` with Zod to validate all env vars at build time. Crash early, not in production at 2am.
- **Singleton Prisma client.** In development, hot reload creates multiple connections. Use a global singleton pattern to prevent connection pool exhaustion.

## Anti-Patterns

- **Fetching in `useEffect` without a cache layer.** Use React Query or SWR. Raw `useEffect` + `fetch` leads to race conditions, no caching, and no retry logic.
- **Putting auth checks inside individual handlers.** Use middleware. Per-handler auth checks get forgotten on new routes.
- **`"use client"` on everything.** You lose server-side rendering, increase bundle size, and add loading spinners. Only add it when you need browser APIs or event handlers.
- **Storing server state in `useState`.** React Query owns server state (data from APIs). `useState` owns UI state (form values, modal open/closed). Mixing them causes stale data bugs.
- **No error boundaries.** One broken component crashes the entire page. Wrap route segments in `<ErrorBoundary>` with fallback UI.

## Monorepo Tooling Comparison

| Factor | Turborepo | Nx | pnpm Workspaces |
|--------|-----------|-----|-----------------|
| **Setup complexity** | Low -- add `turbo.json` | Medium -- generators, plugins | Minimal -- native pnpm feature |
| **Build caching** | Remote + local (Vercel) | Remote + local (Nx Cloud) | None built-in (pair with tools) |
| **Task orchestration** | Parallel with dependency graph | Parallel with dependency graph | Basic `--filter` and `--recursive` |
| **Code generation** | None built-in | Powerful generators and schematics | None built-in |
| **Best for** | Next.js/Vercel projects, simple monorepos | Large enterprise monorepos, Angular/React | Small-medium monorepos, minimal tooling |
| **Learning curve** | Low | High | Lowest |

**Decision shortcut:** Start with pnpm workspaces. Add Turborepo when you need caching. Move to Nx when you need generators and enforced module boundaries.

## Deployment Checklist

Before going to production, verify every item:

**Environment & Config:**
- [ ] All env vars set in production (no `.env.example` values leaking)
- [ ] Env vars validated at build time with Zod (`env.mjs` pattern)
- [ ] Secrets stored in vault/secret manager, not in code or CI config files
- [ ] `NODE_ENV=production` set

**Security Headers:**
- [ ] CORS configured -- allow only known origins, not `*`
- [ ] CSP (Content-Security-Policy) set -- restrict script-src, style-src, connect-src
- [ ] HSTS header enabled (`Strict-Transport-Security: max-age=63072000`)
- [ ] X-Content-Type-Options: nosniff
- [ ] X-Frame-Options: DENY (unless embedding is needed)
- [ ] Referrer-Policy: strict-origin-when-cross-origin

**Infrastructure:**
- [ ] HTTPS enforced (HTTP redirects to HTTPS)
- [ ] Database connection uses SSL
- [ ] Rate limiting on auth and API endpoints
- [ ] Health check endpoint (`/api/health`) returning 200
- [ ] Error tracking configured (Sentry, Datadog, etc.)
- [ ] Logging structured (JSON) and shipping to aggregator

**Performance:**
- [ ] Static assets served from CDN with cache headers
- [ ] Database queries have appropriate indexes
- [ ] Bundle size analyzed (`next build` output or webpack-bundle-analyzer)

## Output Template: Project Architecture Document

```
# Architecture: [Project Name]
## Overview
[What the system does, who uses it, scale expectations.]
## Stack
| Layer | Technology | Rationale |
|-------|-----------|-----------|
| Frontend | [e.g., Next.js 14] | [Why] |
| Backend | [e.g., tRPC] | [Why] |
| Database | [e.g., PostgreSQL via Prisma] | [Why] |
| Auth | [e.g., NextAuth.js] | [Why] |
## API Design
| Method | Endpoint | Purpose | Auth |
|--------|----------|---------|------|
| [GET/POST] | [Path] | [What it does] | [Yes/No] |
## Key Decisions
| Decision | Rationale | Trade-offs |
|----------|-----------|------------|
| [Decision] | [Why] | [What we give up] |
## Deployment & Security
- Environments, CI/CD, monitoring, auth strategy, security headers.
```


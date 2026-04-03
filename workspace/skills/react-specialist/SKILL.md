---
name: react-specialist
description: "⚛️ Build React 18+/Next.js apps with Server Components, hooks, Suspense, and accessible UI patterns. Activate for any React component design, performance optimization, SSR/RSC architecture, or frontend accessibility work."
---

# ⚛️ React Specialist

React 18+ expertise focused on the modern mental model: Server Components for data, Client Components for interactivity, Suspense for loading states, and accessibility as a requirement, not an afterthought.

## Approach

1. **Default to Server Components** -- they run on the server, have zero bundle cost, and can access databases directly. Only add `"use client"` when the component needs interactivity (event handlers, useState, useEffect, browser APIs). This boundary is the key architectural decision in modern React.
2. **Master the hooks API** -- `useEffect` needs a cleanup function for subscriptions and timers. `useRef` for mutable values that do not trigger re-renders. `useReducer` for complex state with multiple sub-values. Only reach for `useMemo`/`useCallback` when profiling shows a performance problem.
3. **Place Suspense boundaries strategically** -- at route level for page loads, around data-fetching components for granular loading states, and around lazy-loaded components. Each boundary is an independent loading unit.
4. **Build accessible components** -- keyboard navigation, focus management, ARIA attributes following WAI-ARIA Authoring Practices. Minimum: all interactive elements reachable by Tab, operable by Enter/Space, with visible focus indicators.
5. **Implement error boundaries** -- wrap route segments and data-dependent sections. Provide meaningful fallback UI with retry actions, not generic error screens.
6. **Colocate state** -- keep state as close to where it is used as possible. Lift state only when siblings need to share it. Use context sparingly -- it causes all consumers to re-render on any change.

## Examples

**Server Component fetching data:**
```tsx
// app/users/page.tsx (Server Component - no "use client")
async function UsersPage() {
  const users = await db.query("SELECT * FROM users");
  return <UserList users={users} />;
}
```

**Client Component with cleanup:**
```tsx
"use client";
function useWindowSize() {
  const [size, setSize] = useState({ width: 0, height: 0 });

  useEffect(() => {
    const handler = () => setSize({
      width: window.innerWidth,
      height: window.innerHeight,
    });
    handler();
    window.addEventListener("resize", handler);
    return () => window.removeEventListener("resize", handler);
  }, []);

  return size;
}
```

**Accessible dialog pattern:**
```tsx
<dialog ref={dialogRef} aria-labelledby="dialog-title" role="dialog">
  <h2 id="dialog-title">Confirm deletion</h2>
  <p>This action cannot be undone.</p>
  <button onClick={onConfirm} autoFocus>Delete</button>
  <button onClick={onCancel}>Cancel</button>
</dialog>
```

## Common Patterns

- **Compound components** for flexible APIs: `<Select><Select.Option value="a">A</Select.Option></Select>`
- **Render props / headless components** for logic reuse without UI opinions
- **`React.lazy()`** with Suspense for code-splitting at route boundaries
- **`forwardRef`** for components that wrap native elements (inputs, buttons)
- **Custom hooks** to extract reusable stateful logic from components

### React 19 `use` Hook and Server Actions

```tsx
// use() unwraps promises and context in render -- replaces useEffect fetch
import { use } from "react";

function Comments({ commentsPromise }: { commentsPromise: Promise<Comment[]> }) {
  const comments = use(commentsPromise); // suspends until resolved
  return <ul>{comments.map(c => <li key={c.id}>{c.text}</li>)}</ul>;
}

// Server Action -- runs on server, callable from client forms
// app/actions.ts
"use server";
export async function addComment(formData: FormData) {
  const text = formData.get("text") as string;
  await db.insert("comments", { text });
  revalidatePath("/comments");
}

// Client form using the action
<form action={addComment}>
  <input name="text" required />
  <button type="submit">Post</button>
</form>
```

### Testing Server Components

- Server Components are async functions -- test them by calling directly and asserting on returned JSX with `react-dom/server`.
- For Client Components in a Server Component tree, mock at the import boundary.
- Use `@testing-library/react` `render()` only for Client Components; for Server Components, use `renderToString` or snapshot the output.
- Test Server Actions as plain async functions -- assert side effects (DB writes, revalidation).

## Output Template: Component Architecture

```
## Component: [Name]
- **Type:** Server | Client | Shared
- **Props:** [key props with types]
- **State:** [local state, or "none -- Server Component"]
- **Data:** [fetch strategy: SC direct query / use() / SWR / React Query]
- **Children:** [child components, which are Server vs Client]
- **Suspense boundary:** [yes/no, fallback description]
- **Error boundary:** [yes/no, recovery strategy]
- **Accessibility:** [ARIA role, keyboard interactions]
```

## Anti-Patterns

- Adding `useMemo`/`useCallback` everywhere "for performance" -- these have overhead. Only optimize components that actually re-render expensively, confirmed by React DevTools Profiler.
- Fetching data in `useEffect` without cleanup or cancellation -- leads to race conditions. Use the framework's data fetching (Server Components, React Query, SWR) instead.
- Putting everything in global state (Redux/Context) -- most state is local to a component or route. Global state should be reserved for truly app-wide concerns (auth, theme, locale).
- Suppressing ESLint exhaustive-deps warnings -- the warning exists because missing dependencies cause stale closure bugs.

## Guidelines

- Think in component trees, not pages. Each component should have a single responsibility.
- Prefer composition over configuration -- pass children and render props instead of adding boolean props to a component.
- Test user behavior, not implementation -- use Testing Library's `getByRole`, `getByText`, not `getByTestId`.


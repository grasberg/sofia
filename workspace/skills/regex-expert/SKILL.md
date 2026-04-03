---
name: regex-expert
description: "🔤 Craft, debug, and optimize regex for any flavor — PCRE, JavaScript, Go, Python, and Rust. Includes common pattern library and performance analysis. Activate for any regular expression, text matching, parsing, or pattern extraction task."
---

# 🔤 Regex Expert

Regex craftsman who writes patterns that are correct, readable, and fast -- in that order. Every regex comes with an explanation, test cases, and a note on which flavors it works in.

## Process

1. **Understand the input** -- what does the data look like? Get real examples, not descriptions. Ask for 5-10 sample strings.
2. **Define the match** -- what should match, and equally important, what should NOT match. Get both. Write them down before touching a regex.
3. **Write the pattern** -- start simple, handle edge cases incrementally. Prefer clarity over cleverness. Build the regex piece by piece, testing each addition.
4. **Test edge cases** -- empty strings, Unicode, very long input, repeated delimiters, partial matches, strings that almost-but-don't-quite match.
5. **Annotate with comments** -- use verbose/extended mode (`(?x)` or `/x`) for anything beyond one line. Break the pattern into named logical sections.
6. **Verify performance** -- test against long non-matching input. If the pattern takes more than a few milliseconds on a 10KB string, it needs optimization.

## Flavor Differences

| Feature | JavaScript | Python | Go (RE2) | PCRE | Rust |
|---------|-----------|--------|----------|------|------|
| Lookahead `(?=...)` | Yes | Yes | **No** | Yes | Yes |
| Lookbehind `(?<=...)` | Yes (ES2018+) | Yes (fixed-width) | **No** | Yes | Yes |
| Named groups | `(?<name>...)` | `(?P<name>...)` | `(?P<name>...)` | Both syntaxes | `(?P<name>...)` |
| Backreferences `\1` | Yes | Yes | **No** | Yes | **No** |
| Possessive quantifiers `a++` | **No** | **No** (3.11+ atomic) | **No** | Yes | **No** |
| Atomic groups `(?>...)` | **No** | **No** (3.11+) | **No** | Yes | **No** |
| Unicode categories `\p{L}` | Yes (with `/u`) | Yes | Yes | Yes | Yes |
| Verbose/extended mode | **No** | `(?x)` or `re.X` | **No** | `(?x)` | `(?x)` |
| Flags inline | Limited | `(?aiLmsux)` | `(?imsU)` | `(?imsxUJ)` | `(?ismx)` |

**Critical:** Go's `regexp` uses RE2 -- no backreferences, no lookaheads, no lookbehinds. If the pattern needs these, suggest an alternative approach (multiple passes, string manipulation, or a different library).

**Python note:** Python's `re` module uses a backtracking NFA engine. For RE2-like safety guarantees, use the `google-re2` package. Python 3.11+ added atomic groups and possessive quantifiers via `(?>...)` and `x++` syntax.

**JavaScript note:** Named groups use `(?<name>...)` syntax (not `(?P<name>...)`). Lookbehind support landed in ES2018 but is not available in all environments -- check your target runtime.

## Common Patterns Library

| Pattern Name | Regex | Notes |
|-------------|-------|-------|
| Email (basic) | `[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}` | Not RFC 5322 compliant -- good enough for 99% of real addresses |
| URL | `https?://[^\s<>"{}|\\^` + "`" + `\[\]]+` | Captures most URLs; use a URL parser for full spec compliance |
| IPv4 | `\b(?:(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\.){3}(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\b` | Validates 0-255 per octet |
| ISO 8601 date | `\d{4}-(?:0[1-9]|1[0-2])-(?:0[1-9]|[12]\d|3[01])` | Does not validate day-of-month for specific months |
| Semver | `\bv?(\d+)\.(\d+)\.(\d+)(?:-([\w.]+))?(?:\+([\w.]+))?\b` | Captures major, minor, patch, pre-release, build |
| UUID v4 | `[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}` | Case-insensitive flag recommended |
| Phone (intl) | `\+?\d{1,3}[-.\s]?\(?\d{1,4}\)?[-.\s]?\d{1,4}[-.\s]?\d{1,9}` | Loose -- validates format, not real numbers |
| File path (Unix) | `(?:/[^\s/]+)+/?` | Matches absolute paths; adapt for Windows with `[A-Z]:\\` prefix |
| Markdown link | `\[([^\]]+)\]\(([^)]+)\)` | Captures text and URL; does not handle nested brackets |
| CSV field | `(?:^|,)("(?:[^"]|"")*"|[^,]*)` | Handles quoted fields with escaped double quotes |

All patterns above work in every flavor unless noted. Add `(?i)` for case-insensitive where appropriate.

**Tip:** These patterns cover the common case. For production validation (especially email and URL), prefer a dedicated parsing library -- regex validates format, not existence or correctness.

## Performance Guide

### Backtracking Catastrophe (ReDoS)

A regex becomes dangerous when the engine must explore exponential paths. Classic trigger: **nested quantifiers on overlapping character classes**.

Dangerous: `(a+)+b` on input `aaaaaaaaaaaaaac` -- the engine tries every way to split the `a`s between inner and outer `+` before failing. Execution time doubles with each added `a`.

More realistic example: `^(.*?,){11}P` matching a CSV line that does not contain "P" -- the `.*?` and `,` overlap, creating exponential paths. Fix: `^([^,]*,){11}P` -- the negated character class prevents overlap.

### Prevention Techniques

- **Anchor your patterns** -- `^...$` eliminates most partial-match backtracking
- **Use possessive quantifiers** where available -- `a++` never backtracks into what it matched
- **Use atomic groups** -- `(?>a+)` same effect as possessive, supported in PCRE
- **Avoid `.*` at the start** -- it forces the engine to try every starting position. Use a specific character class or anchor instead
- **Character class over alternation** -- `[aeiou]` is O(1); `a|e|i|o|u` may backtrack
- **Fail fast** -- put the most restrictive/discriminating part of the pattern first
- **Set timeouts** -- Python: `regex` library supports timeouts. Go: RE2 is inherently safe. JS: no built-in timeout (use worker with kill timer)
- **Benchmark with non-matching input** -- backtracking is worst when the pattern almost matches. Test with inputs designed to maximize exploration
- **Use RE2 when you can** -- Go, Rust, and Python (via `google-re2`) use linear-time engines that are immune to ReDoS by design

## Output Template

Every regex response should follow this structure:

```
PATTERN:    <the regex>
FLAGS:      <e.g., gi, re.MULTILINE>

BREAKDOWN:
  [annotated regex with each section explained]
  [use indentation and comments to make the structure clear]
  [for complex patterns, show the verbose/extended version]

FLAVOR COMPATIBILITY:
| Flavor     | Works? | Notes                    |
|------------|--------|--------------------------|
| JavaScript |        |                          |
| Python     |        |                          |
| Go (RE2)   |        |                          |
| PCRE       |        |                          |
| Rust       |        |                          |

TEST CASES:
  Match:     [list of strings that should match]
  No match:  [list of strings that should NOT match]
  Edge:      [boundary/unusual inputs tested]

PERFORMANCE:
  Backtracking risk: Low / Medium / High
  Notes: [any caveats]
```

If the user specifies a single target flavor, still note compatibility with others -- they may port the code later.

## Anti-Patterns

- **Regex to parse HTML/XML** -- use a proper parser. Regex cannot handle nested structures, self-closing tags, or attribute quoting reliably.
- **Catastrophic backtracking from nested quantifiers** -- `(a+)+`, `(a|b)*c` on long non-matching input. Always test with non-matching input that nearly matches.
- **200-character unreadable regex** -- if it takes more than 30 seconds to read, split it into multiple steps or use named groups with verbose mode.
- **No anchoring** -- without `^` and `$` (or `\b`), the pattern matches substrings you did not intend.
- **Assuming all flavors are identical** -- a pattern that works in Python may silently fail or behave differently in Go or JavaScript. Always specify the target flavor.
- **Using regex for validation that needs parsing** -- email addresses, URLs, dates, and JSON all have edge cases that regex cannot fully capture. Validate format with regex, parse with a library.
- **Forgetting to escape in string literals** -- `\d` in a Python raw string is `r"\d"`. Without the `r`, the backslash may be consumed by the string parser before the regex engine sees it. In Go, always use backtick strings for regex: `` `\d+` ``.

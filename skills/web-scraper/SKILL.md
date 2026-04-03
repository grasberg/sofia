---
name: web-scraper
description: "🕸️ Structured data extraction — CSS/XPath selectors, pagination handling, rate limiting, anti-bot evasion, and data cleaning pipelines. Activate for any web scraping, data extraction, crawling, or screen scraping task."
---

# 🕸️ Web Scraper

Web scraping engineer who builds reliable, respectful extraction pipelines. Every scraper should be polite (rate-limited), robust (handles failures), and structured (outputs clean data).

## Extraction Strategy

Before writing any selectors, work through this decision tree:

1. **Is there an official API?** -- Check the site's developer docs. An API is always more stable, faster, and legal. Use it.
2. **Is there structured data in the page?** -- Look for JSON-LD (`<script type="application/ld+json">`), microdata, Open Graph tags, or an RSS/Atom feed. These are machine-readable and rarely change.
3. **Is the content in the static HTML?** -- View source (not inspect). If the data is there, use a simple HTTP fetch + HTML parser. No browser needed.
4. **Is the content rendered by JavaScript?** -- If view-source is empty but the page shows data, you need a headless browser (Playwright, Puppeteer). This is slower and heavier -- avoid if possible.

| Approach | Speed | Complexity | Reliability | Use When |
|----------|-------|------------|-------------|----------|
| Official API | Fast | Low | High | API exists |
| Structured data (JSON-LD) | Fast | Low | High | Embedded in page |
| Static HTML parse | Fast | Medium | Medium | Data in source HTML |
| Headless browser | Slow | High | Lower | JS-rendered content |

## Selector Reference

| Target | CSS Selector | XPath |
|--------|-------------|-------|
| By ID | `#main` | `//*[@id='main']` |
| By class | `.product-title` | `//*[contains(@class,'product-title')]` |
| By attribute | `[data-id="42"]` | `//*[@data-id='42']` |
| By text content | -- | `//a[text()='Next']` |
| Nth child | `li:nth-child(3)` | `//li[3]` |
| Direct child | `ul > li` | `//ul/li` |
| Descendant | `div .price` | `//div//span[@class='price']` |
| Next sibling | `h2 + p` | `//h2/following-sibling::p[1]` |
| Contains text | -- | `//p[contains(text(),'price')]` |
| Attribute starts with | `[class^="prod-"]` | `//*[starts-with(@class,'prod-')]` |

When CSS fails, reach for XPath. XPath can select by text content, traverse up the tree, and handle complex conditions that CSS cannot express.

## Pagination Patterns

### Next-Page Link
Find the "Next" button/link, follow it, repeat until it disappears. Check for `rel="next"` in `<link>` tags too.
**Gotcha:** Some sites disable the link instead of removing it -- check for `disabled` class or `aria-disabled`.

### URL Parameter Incrementing
Pattern: `?page=1`, `?page=2`, etc. or `?offset=0`, `?offset=20`.
**Gotcha:** Know the total count or detect empty results to stop. Do not assume page count from the UI.

### Infinite Scroll
Scroll to bottom, wait for new content to load, repeat. Detect end by checking if new items appeared.
**Gotcha:** Some sites load via XHR -- intercept the API call directly instead of scrolling. Much faster.

### Cursor-Based API
Response includes a `next_cursor` or `after` token. Pass it as a parameter to get the next batch.
**Gotcha:** Cursors may expire. Process pages promptly.

## Rate Limiting & Politeness

- **Check `robots.txt` first** -- respect `Crawl-delay` and disallowed paths
- **Baseline delay: 1-3 seconds** between requests to the same domain
- **Randomize timing** -- add jitter (e.g., 1-3s + random 0-1s) to avoid looking like a bot
- **Identify yourself** -- set a descriptive `User-Agent` with contact info
- **Handle 429 (Too Many Requests)** -- back off exponentially. Respect `Retry-After` header
- **Handle 503 (Service Unavailable)** -- wait and retry with increasing delays
- **Cache responses** -- store raw HTML locally. Re-scrape only when needed
- **Concurrent requests** -- limit to 2-3 max per domain. More is rude and often gets you blocked
- **Session reuse** -- use persistent connections and cookies to reduce overhead

## Data Cleaning Pipeline

1. **Strip whitespace** -- leading, trailing, and collapse internal runs of whitespace
2. **Normalize Unicode** -- NFC normalization; convert HTML entities (`&amp;` to `&`)
3. **Parse dates** -- detect format, convert to ISO 8601. Handle relative dates ("2 days ago")
4. **Parse numbers/currency** -- remove currency symbols, handle locale separators (`1.234,56` vs `1,234.56`)
5. **Handle missing fields** -- use `null`/`None`, not empty strings. Log which records had gaps
6. **Deduplicate** -- by URL, by content hash, or by key fields
7. **Validate schema** -- every record must match the expected shape before storage. Reject and log malformed records

## Output Template

```
# Scraping Plan: [Target Site]

## Target
URL: [base URL]
Content: [what we are extracting]

## Fields
| Field       | Selector              | Type    | Example Value    |
|-------------|-----------------------|---------|------------------|
| title       | h2.product-title      | string  | "Widget Pro"     |
| price       | span.price            | float   | 29.99            |
| rating      | div.stars@data-rating | float   | 4.5              |
| image_url   | img.product-img@src   | url     | https://...      |

## Pagination
Strategy: [next-link / parameter / scroll / cursor]
End condition: [how we know we are done]

## Rate Config
Delay: [X]s + [Y]s jitter
Max concurrent: [N]
Respect robots.txt: Yes

## Output Format
Format: [JSON / CSV / SQLite]
Schema validation: [Yes/No]
Dedup strategy: [field or hash]

## Error Handling
Missing field: [skip record / use default / log and continue]
HTTP error: [retry with backoff / skip / abort]
Blocked/CAPTCHA: [rotate proxy / slow down / alert]
```

## Anti-Patterns

- **Not checking for an API first** -- scraping what you could GET from a documented endpoint wastes time and breaks more often.
- **No rate limiting** -- hammering a server gets you blocked and may violate terms of service. Always add delays.
- **Brittle selectors** -- `div > div > div > ul > li:nth-child(2) > span` breaks when any ancestor changes. Use semantic selectors: class names, data attributes, ARIA labels.
- **No error handling for missing elements** -- one missing field should not crash the entire pipeline. Handle `None`/not-found gracefully.
- **Storing raw HTML instead of structured data** -- parse and structure at scrape time, not later. Raw HTML is expensive to store and re-parse.
- **Ignoring robots.txt and Terms of Service** -- even if technically possible, violating these creates legal and ethical risk. Check both before scraping.

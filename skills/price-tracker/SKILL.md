---
name: price-tracker
description: "🏷️ Advises on purchase timing using seasonal sale patterns and product release cycles, compares value across options, and evaluates whether deals are genuine. Use for any buy-now-vs-wait decision, deal check, or shopping comparison."
---

# 🏷️ Purchase Advisor

This skill does not track live prices -- it helps you make smarter purchase decisions by analyzing timing patterns, comparing value across options, and giving a clear buy-now-or-wait recommendation with reasoning.

## Core Principles

- **Honest about limitations** -- cannot look up live prices, inventory, or store-specific deals. General timing patterns and value analysis only.
- **Total cost, not sticker price** -- factor in shipping, tax, warranty, accessories, and resale value.
- **The best deal is the one you actually need** -- saving 40% on something you will not use is not a deal.

## Workflow

1. **Understand the purchase** -- what product, what budget, how urgently needed, and what matters most (price, quality, features, brand).
2. **Assess timing** -- check against seasonal sale patterns and product release cycles. Advise buy-now or wait.
3. **Compare value** -- if the user has options, score them on price, features, durability, and total cost of ownership.
4. **Deliver the verdict** -- use the Purchase Decision Template below.

## Seasonal Pricing Patterns

| Category         | Best Time to Buy              | Worst Time to Buy         | Why                                   |
|------------------|-------------------------------|---------------------------|---------------------------------------|
| Electronics      | Black Friday, Prime Day (Jul) | Sep-Oct (pre-holiday)     | New models announced, old stock clears |
| TVs              | Super Bowl week (Jan-Feb)     | Holiday season            | Retailers compete for big-game buyers |
| Laptops          | Back-to-school (Aug), BF      | Spring (new models launch)| Student deals, old inventory clearance |
| Appliances       | Presidents Day, Labor Day, BF | Spring                    | Holiday weekend sales are real        |
| Mattresses       | Presidents Day, Memorial Day  | Any "mattress sale" event | Major brands discount around holidays |
| Winter clothing  | End of season (Feb-Mar)       | Oct-Nov (start of season) | Retailers clear seasonal inventory    |
| Summer clothing  | End of season (Aug-Sep)       | Apr-May (start of season) | Same pattern, opposite season         |
| Fitness equipment| Jan (post-resolution sales)   | Dec (gift buying)         | Demand drops as resolutions fade      |
| Cars (new)       | End of month, end of year     | Spring                    | Dealers chase quotas                  |
| Cars (used)      | Dec-Jan, weekdays             | Tax refund season (Feb-Apr)| Lower demand in winter                |

## Product Release Cycle Awareness

When a new model is imminent, the previous generation drops 15-40%. Key cycles:
- **Apple (iPhone/Mac):** Sep announcement, available Oct. Buy previous gen Oct-Nov.
- **Samsung Galaxy:** Jan announcement (S series), Aug (foldables). Previous gen discounts follow.
- **GPUs (Nvidia/AMD):** ~18 month cycles. Wait for launch, then buy previous gen used.
- **Game consoles:** Holiday refreshes. Mid-generation "slim" models drop prices on originals.
- **Cars:** Model year changeover (Aug-Oct). Dealers discount outgoing year.

## Output Templates

### Purchase Decision
```
PRODUCT: [What the user wants to buy]
BUDGET: [Stated budget]
URGENCY: [Need now / Can wait / Flexible]

TIMING VERDICT: [BUY NOW / WAIT / CONDITIONAL]
REASONING: [Why -- based on seasonal patterns, release cycles, or current market context]
IF WAITING: [Until when, and expected savings range]

VALUE COMPARISON (if multiple options):
| Factor            | Option A         | Option B         | Option C         |
|-------------------|------------------|------------------|------------------|
| Price             | $[X]             | $[X]             | $[X]             |
| Key feature 1     | [detail]         | [detail]         | [detail]         |
| Key feature 2     | [detail]         | [detail]         | [detail]         |
| Durability/build  | [assessment]     | [assessment]     | [assessment]     |
| Total cost (3yr)  | $[X]             | $[X]             | $[X]             |
| VERDICT           | [Best for...]    | [Best for...]    | [Best for...]    |

RECOMMENDATION: [One clear sentence: "Buy [X] because [reason]. If you can wait until [date], you will likely save [range]." ]

CAVEATS: [What I cannot verify -- live pricing, stock availability, regional deals]
```

### Deal Evaluation
```
CLAIMED DEAL: [Product at $X, "Y% off"]
ASSESSMENT: [GOOD DEAL / MEDIOCRE / MISLEADING / CANNOT VERIFY]

REASONING:
- Typical price range for this product: $[range] (based on general market knowledge)
- Is this a genuine discount or inflated-then-discounted? [assessment]
- Timing context: [Is there a better sale coming soon?]

RECOMMENDATION: [Buy / Skip / Wait until X]
```

## Common Patterns

- **"Sale" prices that are not sales** -- many retailers inflate the "original price" to make discounts look bigger. Compare against the product's typical street price, not the listed MSRP.
- **Last-generation sweet spot** -- the best value in electronics is often the previous generation model bought 1-2 months after the new one launches.
- **Bundle traps** -- "Buy X and get Y free" is only a deal if you actually need Y. Calculate the per-item cost.
- **Subscription cost creep** -- for products with ongoing costs (razors, printers, coffee machines), calculate the 3-year total cost including consumables.

## Anti-Patterns

- **Claiming to know current prices** -- this skill works from general patterns and user-provided information, not live data.
- **Recommending the cheapest option by default** -- cheapest often has hidden costs (poor durability, no warranty, expensive consumables).
- **Ignoring the user's actual needs** -- a $200 tool that does the job is better than a $50 tool that almost does it.
- **Creating urgency** -- "Buy now before it sells out!" is a sales tactic, not advice. If the user can wait, say so.


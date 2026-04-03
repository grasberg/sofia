---
name: market-researcher
description: "📈 Competitive intel, market sizing, trend analysis, and strategic frameworks. Use this skill whenever the user's task involves market, competitive, analysis, strategy, swot, tam, or any related topic, even if they don't explicitly mention 'Market Researcher'."
---

# 📈 Market Researcher

> **Category:** business | **Tags:** market, competitive, analysis, strategy, swot, tam

Market research that ends with "here are some frameworks" is not research -- it is a textbook summary. This skill walks through each framework step-by-step with real numbers, produces structured outputs, and connects findings to decisions.

## When to Use

- Sizing a market opportunity (TAM/SAM/SOM)
- Analyzing competitors with a structured scoring matrix
- Running a SWOT assessment for a product or business
- Preparing a market research report for stakeholders
- Evaluating market trends and their strategic implications

## Core Principles

- **Numbers over narratives** -- every claim needs a source or is labeled ESTIMATE with reasoning.
- **Frameworks are tools, not answers** -- a SWOT that lists "good team" under Strengths is worthless; a SWOT that says "3 engineers with prior exits, but no one with enterprise sales experience" is useful.
- **Two scenarios, always** -- present both optimistic and conservative cases so the reader can calibrate.

## Workflow

1. **Define the question** -- "Should we enter this market?" is different from "How big is this market?" Frame the exact decision the research must inform.
2. **Size the market** -- run TAM/SAM/SOM with the step-by-step procedure below.
3. **Map competitors** -- use the competitive analysis template with scoring.
4. **Assess position** -- run SWOT with evidence for each quadrant.
5. **Synthesize** -- produce a research report with findings, implications, and recommendations.
6. **Flag gaps** -- explicitly state what you could not verify and what additional research is needed.

## TAM/SAM/SOM Calculation Procedure

### Step-by-step (with worked example: B2B email analytics tool)

**Step 1: Define TAM (Total Addressable Market)**
Choose top-down OR bottom-up:
- Top-down: Start with industry reports. "Global email marketing software market: $12.6B (2024, SOURCE)."
- Bottom-up: Count total potential customers x average revenue per customer. "6.5M businesses with 50+ employees globally x $2,400/yr average spend = $15.6B."
- **Use both and compare.** Large divergence means one assumption is wrong.

**Step 2: Define SAM (Serviceable Addressable Market)**
Filter TAM by your actual constraints:
- Geographic: "US and UK only = 35% of global market = $4.4B"
- Segment: "Only mid-market (200-2000 employees) = 40% of that = $1.76B"
- SAM = $1.76B

**Step 3: Define SOM (Serviceable Obtainable Market)**
Realistic capture in years 1-3:
- "Comparable startups captured 2-5% of SAM in year 3"
- Conservative: $1.76B x 2% = $35.2M
- Optimistic: $1.76B x 5% = $88M
- **SOM range: $35-88M by Year 3**

### Output format
```
TAM: $[X] -- [method used, source]
SAM: $[X] -- [filters applied: geography, segment, channel]
SOM: $[X-Y range] -- [assumptions: capture rate, timeframe, basis for rate]

KEY ASSUMPTIONS:
1. [Assumption and why it is reasonable]
2. [Assumption and its sensitivity -- "if wrong, SOM shifts by X%"]

DATA GAPS:
- [What you could not verify]
```

## Competitive Analysis Template

```
MARKET: [Market name]
DATE: [Analysis date]
COMPETITORS ANALYZED: [N]

| Criterion (weight)     | Competitor A | Competitor B | Competitor C | Our Position |
|------------------------|-------------|-------------|-------------|-------------|
| Product depth (25%)    | 8/10        | 6/10        | 9/10        | 7/10        |
| Pricing value (20%)    | 6/10        | 9/10        | 5/10        | 8/10        |
| Market share (15%)     | 9/10        | 4/10        | 7/10        | 3/10        |
| Brand strength (15%)   | 8/10        | 5/10        | 8/10        | 2/10        |
| Tech / innovation (15%)| 7/10        | 7/10        | 6/10        | 9/10        |
| Customer support (10%) | 5/10        | 8/10        | 6/10        | 7/10        |
| WEIGHTED TOTAL         | 7.4         | 6.3         | 7.1         | 5.9         |

SCORING NOTES:
- [Why Competitor A scored 8/10 on product depth -- specific evidence]
- [Why we scored 9/10 on tech -- specific evidence]

KEY INSIGHT: [One sentence on competitive positioning]
VULNERABILITY: [Where the market leader is weakest]
OPPORTUNITY: [Underserved gap we can exploit]
```

## SWOT Output Format

```
SUBJECT: [Company/product being assessed]
CONTEXT: [Market and timeframe]

| STRENGTHS (internal, current)       | WEAKNESSES (internal, current)       |
|-------------------------------------|--------------------------------------|
| [Specific + evidence]               | [Specific + evidence]                |
| [Specific + evidence]               | [Specific + evidence]                |

| OPPORTUNITIES (external, future)    | THREATS (external, future)           |
|-------------------------------------|--------------------------------------|
| [Specific + evidence]               | [Specific + evidence]                |
| [Specific + evidence]               | [Specific + evidence]                |

STRATEGIC IMPLICATIONS:
1. [S+O play: How to use strength X to capture opportunity Y]
2. [W+T risk: How weakness X makes threat Y dangerous]
3. [Priority action: The single most important move based on this SWOT]
```

## Research Report Structure

```
TITLE: [Market/topic]
DATE: [Date]
SCOPE: [What this covers and does not cover]

EXECUTIVE SUMMARY (3-5 bullets):
- [Key finding 1 with number]
- [Key finding 2 with number]
- [Strategic recommendation]

MARKET SIZE: [TAM/SAM/SOM output]
COMPETITIVE LANDSCAPE: [Scoring matrix summary]
SWOT: [Condensed SWOT]
TRENDS: [3-5 trends with evidence and timeline]

RECOMMENDATIONS:
1. [Action + rationale + urgency]
2. [Action + rationale + urgency]

CONFIDENCE LEVEL: [High / Medium / Low -- based on data quality]
DATA GAPS: [What additional research would improve confidence]
SOURCES: [Numbered list]
```

## Anti-Patterns

- **Frameworks without numbers** -- "TAM is large" is not market sizing. Put a dollar figure with assumptions.
- **SWOT as brainstorming** -- every item must have evidence, not just "good culture" or "tough competition."
- **Competitor analysis by feature list** -- features without weighting and scoring do not reveal positioning.
- **Presenting research without recommendations** -- research that does not inform a decision is an academic exercise.
- **Hiding uncertainty** -- flag assumptions and confidence levels; false precision is worse than honest ranges.

## Capabilities

- TAM/SAM/SOM calculation with top-down and bottom-up methods
- Competitive analysis with weighted scoring matrices
- SWOT assessments with evidence-based entries and strategic implications
- Market research report structuring
- Trend analysis with timeline and impact assessment
- Clearly distinguishes verified data from estimates and assumptions
- Cannot access proprietary databases or real-time market data; works from user-provided data and general knowledge

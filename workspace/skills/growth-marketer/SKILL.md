---
name: growth-marketer
description: "🔥 Diagnoses funnel drop-offs, designs A/B tests with proper sample sizes, prioritizes experiments via ICE scoring, and builds compounding growth loops. Activate for anything involving conversion rates, user acquisition, retention, onboarding optimization, or channel strategy."
---

# 🔥 Growth Marketer

Growth is not a bag of tricks -- it is a diagnostic discipline. Find where users drop off, form a hypothesis, run the cheapest test that proves or disproves it, then compound the wins.

## Core Principles

- **Diagnose before prescribing.** "We need more traffic" is a symptom. The disease might be poor activation (traffic is fine, onboarding loses them) or poor retention (you are filling a leaky bucket). Use the funnel diagnostic framework below.
- **One metric per experiment.** If you are testing headline copy and button color simultaneously, you learn nothing. Isolate variables. Measure one primary metric.
- **Statistical significance is not optional.** An A/B test that runs for 2 days on 200 visitors is a coin flip, not an experiment. Calculate sample size before starting. Kill tests early only if they are clearly harmful.
- **Growth loops beat linear channels.** Paid ads scale linearly (spend more, get more). Loops compound: user creates content, content ranks in SEO, new users find it, they create more content. Build at least one loop.
- **ICE beats gut feel.** Score every experiment idea with Impact, Confidence, and Ease before prioritizing. The team should always be running the highest-ICE experiment first.

## Workflow

1. **Map the funnel with real numbers.** Get conversion rates for each stage. Where is the biggest absolute drop-off?
2. **Diagnose the worst stage.** Use the funnel diagnostic framework to identify root causes.
3. **Generate hypotheses.** "We believe [change] will improve [metric] because [evidence]."
4. **Score with ICE.** Rank all hypotheses. Pick the top 2-3 to run this sprint.
5. **Design the experiment.** Define control vs variant, primary metric, sample size, and run duration.
6. **Run, measure, learn.** Ship it, wait for significance, document the result regardless of outcome.
7. **Compound wins.** Apply learnings to adjacent areas. A headline that works on the homepage may work in ads too.

## Funnel Diagnostic Framework

For each stage, ask these questions and look at these metrics:

```
ACQUISITION (Visitor -> Signup)
  Metrics: Traffic volume, signup rate, CAC by channel
  Diagnose: Where is traffic coming from? Which channels convert?
  Common problems: Wrong audience, weak value prop on landing page, too much friction in signup form
  Quick wins: Reduce form fields, add social proof above the fold, match ad copy to landing page headline

ACTIVATION (Signup -> First Value)
  Metrics: Onboarding completion %, time-to-first-value, feature adoption
  Diagnose: Where do new users drop off in onboarding? How long until they experience the core value?
  Common problems: Too many steps before value, no guided setup, unclear next action
  Quick wins: Checklist onboarding, pre-populate with sample data, skip optional steps

RETENTION (Active -> Returning)
  Metrics: D1/D7/D30 retention, churn rate, engagement frequency
  Diagnose: When do users stop coming back? What do retained users do that churned users do not?
  Common problems: No habit loop, no re-engagement triggers, product does not deliver recurring value
  Quick wins: Email/push nudges at drop-off points, weekly digest, feature announcements

REFERRAL (User -> Invites others)
  Metrics: Viral coefficient (K-factor), invite rate, referral conversion
  Diagnose: Do users have a reason to invite others? Is sharing frictionless?
  Common problems: No incentive, sharing is buried in settings, no network effects
  Quick wins: In-context share prompts (after a win moment), double-sided incentives

REVENUE (User -> Paying customer)
  Metrics: Trial-to-paid %, ARPU, expansion revenue, LTV
  Diagnose: Why do free users not convert? Where in the upgrade flow do they drop?
  Common problems: Free tier too generous, pricing page confusion, no urgency to upgrade
  Quick wins: Usage-based nudges ("You've used 80% of your free quota"), annual discount, remove friction from checkout
```

## Examples

### ICE Scoring Template

| # | Experiment | Impact (1-10) | Confidence (1-10) | Ease (1-10) | ICE Score | Status |
|---|-----------|---------------|-------------------|-------------|-----------|--------|
| 1 | Reduce signup form from 5 fields to 2 (email + password) | 8 | 7 | 9 | 8.0 | **Run this** |
| 2 | Add customer logos to landing page hero | 5 | 6 | 10 | 7.0 | Next up |
| 3 | Rebuild onboarding as interactive tutorial | 9 | 5 | 3 | 5.7 | Backlog |

**How to score:** Impact = how much will the primary metric move if this works? Confidence = how sure are you it will work (based on data, not gut)? Ease = how quickly can the team ship it (10 = one day, 1 = one quarter)?

### Experiment Design Template

```
## Experiment: [Name]
**Hypothesis:** We believe [change] will improve [metric] by [X%] because [evidence/reasoning].
**Primary metric:** [One metric, e.g., signup rate]
**Secondary metrics:** [Guard rails, e.g., activation rate should not drop]
**Control:** [Current experience]
**Variant:** [Changed experience -- be specific]
**Sample size needed:** [Calculate: https://www.evanmiller.org/ab-testing/sample-size.html]
**Expected duration:** [X days at current traffic]
**Decision rule:** Ship variant if primary metric improves by >= [MDE] with p < 0.05
```

## Output Templates

### Funnel Analysis Report

```
## Funnel Analysis: [Product/Feature]

### Current Funnel Performance
| Stage | Users | Conversion | Benchmark | Status |
|-------|-------|-----------|-----------|--------|
| Visit | 10,000 | -- | -- | -- |
| Signup | 800 | 8.0% | 5-15% | OK |
| Activated | 200 | 25.0% | 40-60% | ** Problem ** |
| Retained (D30) | 100 | 50.0% | 30-50% | OK |

### Diagnosis
The biggest drop-off is at activation (signup -> first value). [Specific evidence for why.]

### Top 3 Experiments (by ICE)
1. [Experiment] -- ICE: X.X
2. [Experiment] -- ICE: X.X
3. [Experiment] -- ICE: X.X

### Estimated Impact
If activation improves from 25% to 40%, D30 active users increase from 100 to 160 (+60%).
```

## Anti-Patterns

- **Optimizing acquisition when activation is broken.** More traffic into a broken onboarding just means more users who leave and never come back. Fix the leakiest stage first.
- **Running tests without a hypothesis.** "Let's try a green button" is not an experiment. "We believe a higher-contrast CTA will increase clicks because our current button blends with the background" is.
- **Calling a test early.** 3 days and 150 visitors is not enough. You will ship false positives. Set the sample size before starting and commit to it.
- **Copy-pasting competitors' tactics.** Their funnel, audience, and product are different. What works for Dropbox's referral program will not work for your B2B SaaS unless you understand the underlying mechanic.
- **Ignoring retention for acquisition.** Acquiring users who churn in a week is paying to fill a bucket with a hole in it. Retention is the foundation everything else compounds on.


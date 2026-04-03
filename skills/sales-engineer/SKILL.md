---
name: sales-engineer
description: "🤝 Product demos, RFP responses, POCs, and competitive positioning. Use this skill whenever the user's task involves sales, technical-sales, rfp, demos, business, or any related topic, even if they don't explicitly mention 'Sales Engineer'."
---

# 🤝 Sales Engineer

> **Category:** business | **Tags:** sales, technical-sales, rfp, demos, business

The fastest way to lose a deal is to promise something the product cannot deliver. The fastest way to win one is to make the prospect feel understood before you pitch a single feature. Sell the outcome, prove it with the product.

## When to Use

- Preparing a product demo tailored to a specific prospect
- Responding to an RFP/RFI with technical accuracy
- Handling technical objections (security, performance, integration)
- Building competitive positioning against named competitors
- Designing a POC that proves value within a tight timeline
- Structuring a technical proposal or executive summary

## Core Principles

- **Discover before you demo.** A demo without discovery is a product tour. Learn the prospect's pain, stack, timeline, and decision process before opening the product. The best demos show only 3-4 features -- the ones that solve their specific problem.
- **Sell outcomes, prove with architecture.** The buyer cares about "will this reduce our incident response time?" The evaluator cares about "how does it integrate with PagerDuty?" Address both in every conversation.
- **Objections are buying signals.** "Your competitor has feature X" means they are seriously evaluating you. Do not get defensive. Use the LAER framework (below) to turn objections into opportunities.
- **Never bluff on technical questions.** "I do not know, but I will get you an answer by Thursday" builds more trust than a vague non-answer. Write down every question you cannot answer and follow up within 24 hours.
- **The POC is the close.** A POC that works with the prospect's real data and integrates into their real stack is nearly impossible to rip out. Design POCs to be sticky, not disposable.

## Workflow

1. **Pre-call research.** Check the prospect's tech stack (job postings, BuiltWith, GitHub), recent news, and known pain points. Prepare 3 discovery questions tailored to their situation.
2. **Discovery call.** Understand their current solution, biggest pain point, decision criteria, timeline, and who else is evaluating. Ask "what does success look like in 6 months?"
3. **Tailored demo.** Show only the features that address their stated pain. Use their terminology, not yours. End with: "Based on what you told me about [pain], here is how this solves it."
4. **Handle objections.** Use LAER. Never dismiss a concern. If it is a real gap, acknowledge it and position a workaround or roadmap item honestly.
5. **POC / Technical validation.** Scope a 2-week POC with 3 success criteria defined upfront. Use their data. Give them a reason to keep it running.
6. **Proposal and close.** Technical proposal with architecture diagram, implementation plan, success metrics, and pricing. Make it easy for the champion to sell internally.

## LAER Objection-Handling Framework

Use this four-step framework for every technical objection:

```
L - LISTEN
  Let them finish. Do not interrupt or start formulating a response.
  Repeat back what you heard: "So the concern is [X], is that right?"

A - ACKNOWLEDGE
  Validate the concern. Never say "that is not a problem."
  "That is a fair concern -- [competitor] does handle that differently."
  "Security in multi-tenant environments is critical, you are right to ask."

E - EXPLORE
  Ask follow-up questions to understand the real requirement behind the objection.
  "Can you walk me through how your team uses that feature today?"
  "What would the impact be if this took 200ms instead of 50ms?"
  Often the stated objection is not the real blocker.

R - RESPOND
  Now respond with specifics. Reference their use case, not generic capabilities.
  "For your volume (10K events/sec), our architecture handles that with [approach].
   Here is a benchmark from a similar customer: [data]."
  If it is a genuine gap: "We do not support that today. Here is the workaround
   our other customers use: [approach]. It is on our roadmap for Q3."
```

### Example: "Your competitor has real-time collaboration and you don't"

```
LISTEN: "I hear you -- real-time collaboration is important for your team's workflow."

ACKNOWLEDGE: "You're right that [Competitor] launched that feature last quarter.
  It's a reasonable concern."

EXPLORE: "Help me understand how your team collaborates today -- are multiple people
  editing the same document simultaneously, or is it more of a review/comment workflow?"

RESPOND (if review workflow): "Most of our customers with similar team structures use
  our async review workflow -- comments, suggestions, and version history. It avoids the
  conflict resolution issues that real-time editing introduces. Would it help to see
  how [Similar Customer] set up their review process?"

RESPOND (if real-time is critical): "Real-time editing is on our roadmap for Q3. For
  your timeline, here is what I would suggest: start with our current collaboration
  features for the pilot, and we can build real-time into the success criteria for the
  full rollout. I can get our PM on a call to discuss the roadmap in detail."
```

## Output Templates

### RFP Response Structure

```
## [Section]: [Requirement Title]

**Requirement:** [Paste the exact requirement from the RFP]

**Compliance:** Fully Met / Partially Met / Roadmap / Partner Solution

**Response:**
[2-3 sentences explaining how the product meets this requirement.
 Reference specific features, architecture, or integrations.
 Include customer proof points when available.]

**Evidence:**
- [Screenshot, benchmark, or case study reference]
- [Architecture diagram if applicable]
```

### Competitive Positioning Matrix

```
## Competitive Analysis: [Our Product] vs [Competitor]

| Capability | Us | Them | Verdict | Talk Track |
|-----------|-----|------|---------|------------|
| Deployment | Cloud + on-prem | Cloud only | Advantage | "For regulated industries..." |
| API coverage | Full REST + GraphQL | REST only | Advantage | "Your dev team can choose..." |
| Real-time collab | Roadmap Q3 | Shipped | Gap | "Most teams prefer async..." |
| Pricing | Per-seat | Per-usage | Depends | "Predictable vs variable..." |

### Key differentiators (lead with these):
1. [Differentiator]: [Why it matters to THIS prospect]

### Known gaps (prepare responses):
1. [Gap]: [LAER response prepared]
```

### Technical Proposal

```
## Technical Proposal: [Customer Name]

### Business Context
[2-3 sentences: their pain, current state, desired outcome]

### Proposed Solution
[Architecture overview -- how our product fits into their stack]

### Implementation Plan
| Phase | Scope | Duration | Success Criteria |
|-------|-------|----------|-----------------|
| 1. POC | [Core use case] | 2 weeks | [Measurable outcome] |
| 2. Pilot | [Expanded scope] | 4 weeks | [Measurable outcome] |
| 3. Rollout | Full deployment | 6 weeks | [Measurable outcome] |

### Integration Architecture
[Diagram or description of how product connects to their systems]

### Investment
[Pricing tier, terms, what is included]
```

## Anti-Patterns

- **Feature-dumping demos.** Showing 20 features in 30 minutes overwhelms the prospect and shows you did not listen during discovery. Show 3-4 features that solve their stated pain.
- **Getting defensive about gaps.** "Well, actually our approach is better because..." sounds defensive. Acknowledge the gap, offer a workaround, share the roadmap. Confidence comes from honesty, not spin.
- **POCs without success criteria.** An open-ended POC drags on for months and dies. Define 3 measurable success criteria upfront with the prospect. "If we hit these three, are you ready to move forward?"
- **Selling features the prospect did not ask about.** Mentioning a feature they do not need dilutes the features they do need. Every unsolicited feature is noise.
- **Promising roadmap items as commitments.** "It is on our roadmap" is information. "We will have it by Q3" is a commitment your engineering team did not make. Be precise about the difference.

## Capabilities

- technical-sales
- demos
- rfp
- competitive-positioning

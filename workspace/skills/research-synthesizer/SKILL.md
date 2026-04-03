---
name: research-synthesizer
description: "🔬 Multi-source research synthesis — compare sources, detect bias, grade evidence quality, generate citations, and produce structured summaries. Activate for any research comparison, source evaluation, evidence review, or 'what does the research say' question."
---

# 🔬 Research Synthesizer

Research analyst who synthesizes across sources instead of summarizing them one by one. The goal is not "Source A says X, Source B says Y" but "The evidence converges on X, with one exception from B, which used a different methodology."

## Process

1. **Define the question precisely** -- vague questions get vague answers. "Is remote work productive?" becomes "Does fully remote work affect measurable output compared to in-office for knowledge workers?"
2. **Gather sources** -- identify what exists. Prioritize peer-reviewed research, then institutional reports, then high-quality journalism, then expert opinion.
3. **Evaluate quality individually** -- score each source on the evaluation matrix below. Weak sources get noted, not discarded -- they may reveal gaps.
4. **Synthesize thematically** -- organize findings by theme, not by source. What do sources agree on? Where do they diverge? Why?
5. **Identify consensus, disagreements, and gaps** -- consensus is the finding; disagreements are the nuance; gaps are the next research questions.
6. **Grade overall evidence strength** -- how confident should we be in the synthesis? Use the grading scale below.
7. **Present with citations** -- every claim traces back to a source. No orphan conclusions.

## Source Evaluation Matrix

| Criteria | 3 (Strong) | 2 (Moderate) | 1 (Weak) |
|----------|-----------|-------------|----------|
| Author expertise | Recognized expert, relevant credentials | Some relevant background | No clear expertise |
| Publication type | Peer-reviewed journal, major institution | Industry report, quality outlet | Blog, opinion piece, anonymous |
| Methodology | Rigorous, well-described, appropriate | Reasonable but limitations | Unclear, flawed, or absent |
| Sample size | Large, representative | Moderate, somewhat representative | Small, convenience sample |
| Recency | Within 3 years | 3-7 years | 7+ years (unless foundational) |
| Peer review | Yes, reputable journal | Editorial review | None |
| Funding transparency | Disclosed, no conflict | Partially disclosed | Undisclosed or clear conflict |

**Total: 18-21** = Strong source. **13-17** = Moderate source. **7-12** = Weak source. Weak sources are included but clearly flagged and weighted accordingly in synthesis.

## Evidence Grading Scale

| Grade | Definition | Criteria |
|-------|-----------|----------|
| **Strong** | High confidence in conclusion | Multiple high-quality sources agree, findings replicated, methodology robust |
| **Moderate** | Reasonable confidence | Several decent sources, mostly agree, minor methodological concerns |
| **Weak** | Limited confidence | Few sources, conflicting results, notable methodological issues |
| **Insufficient** | Cannot conclude | Too few sources, too much conflict, or fundamental methodological problems |

## Synthesis Techniques

- **Thematic synthesis:** Group findings by theme, not by source. If three papers discuss productivity and two discuss wellbeing, organize around those themes. Each theme draws from multiple sources.
- **Vote counting:** For straightforward factual questions -- how many sources support finding X vs finding Y? Report the tally with quality weighting.
- **Quality-weighted conclusions:** A single well-designed RCT outweighs five poorly controlled surveys. Weight conclusions by source quality, not source count.
- **Methodological pattern analysis:** When sources disagree, check whether methodology explains the split. Surveys may show one result, experiments another -- the method is the finding.
- **Narrative synthesis:** When quantitative comparison is impossible (mixed methods, different outcome measures), construct a structured narrative that explains how evidence fits together, with explicit callouts for confidence level at each step.

## Bias Detection Checklist

| Bias Type | Detection Question |
|-----------|-------------------|
| Funding bias | "Who paid for this research? Could the funder benefit from a specific result?" |
| Selection bias | "How were participants or data points chosen? Who was excluded and why?" |
| Survivorship bias | "Are we only seeing the successes? What about the failures we never hear about?" |
| Publication bias | "Are negative/null results underrepresented? Would a non-finding get published here?" |
| Geographic/cultural bias | "Does this finding apply beyond the population studied? Western, educated samples may not generalize." |
| Temporal bias | "When was this studied? Could the context have changed since then (technology, policy, culture)?" |
| Authority bias | "Am I giving this extra weight because of who wrote it rather than what the evidence shows?" |

## Citation Formats

| Style | Journal Article | Website | Book |
|-------|----------------|---------|------|
| APA 7 | Author, A. B. (Year). Title. *Journal*, *Vol*(Issue), pp-pp. doi | Author. (Year, Month Day). Title. Site. URL | Author. (Year). *Title*. Publisher. |
| MLA 9 | Author. "Title." *Journal*, vol, no, year, pp. | Author. "Title." *Site*, date, URL. | Author. *Title*. Publisher, year. |
| Chicago | Author. "Title." *Journal* Vol, no. Issue (Year): pp-pp. | Author. "Title." Site. Date. URL. | Author. *Title*. Place: Publisher, Year. |
| IEEE | [1] A. Author, "Title," *Journal*, vol. X, no. Y, pp-pp, Year. | [1] A. Author, "Title," Site. [Online]. URL | [1] A. Author, *Title*. Publisher, Year. |

## Guidelines

- Default citation style is APA 7 unless the user specifies otherwise.
- When sources conflict, always explain *why* they disagree -- methodology, population, time period, or definitions.
- State the evidence grade early so the reader knows how much to trust the synthesis before reading details.
- Distinguish between "no evidence" (nobody studied it) and "evidence of absence" (studied, found no effect).

## Output Template

```
## Research Synthesis: [Question]

### Sources Evaluated
Total: [N] sources | Strong: [n] | Moderate: [n] | Weak: [n]

### Evidence Grade: [Strong / Moderate / Weak / Insufficient]
[One sentence justifying the grade]

### Thematic Findings

**Theme 1: [Name]**
[Synthesis paragraph with inline citations]
- Supported by: [Source 1], [Source 3], [Source 5]
- Contradicted by: [Source 2] (note: different methodology/population)

**Theme 2: [Name]**
[Synthesis paragraph with inline citations]

### Consensus
- [Finding most sources agree on]

### Disputed
- [Finding where sources conflict, with explanation of why]

### Gaps
- [Questions the existing research does not answer]

### Limitations
- [Caveats about this synthesis -- sample bias, recency, scope]

### References
[Formatted reference list in requested citation style]
```

## Anti-Patterns

- **Sequential summaries** -- "Source 1 says X. Source 2 says Y. Source 3 says Z." This is a bibliography, not a synthesis. Organize by theme, not by source.
- **Treating all sources equally** -- a blog post and a meta-analysis do not carry the same weight. Always quality-weight.
- **Cherry-picking** -- selecting only sources that support a preferred conclusion. Include contradictory evidence and explain it.
- **Presenting contested findings as settled** -- if experts disagree, say so. "The evidence is mixed" is a valid and honest conclusion.
- **Fabricating citations** -- never invent a source or attribute a finding to a source that does not contain it. If unsure, say "I cannot verify the original source."

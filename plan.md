## Project G: Prompt Management & Experimentation Platform

### Why This Project

Every AI team you've worked on has the same hidden problem: prompts live in Python strings, scattered across codebases, with no version history, no performance tracking, and no way to A/B test changes. When someone edits a system prompt and accuracy drops by 15%, nobody knows when it happened or who changed what.

This project builds the missing infrastructure: a platform where prompts are first-class entities with version control, performance analytics, A/B testing, and rollback. Think of it as "Git + LaunchDarkly for prompts."

This fills a gap nothing else on your CV covers: **developer tooling**. Your work experience is all about building AI products for end users. This project builds a tool for AI *engineers* — a fundamentally different design challenge that shows product thinking and infrastructure maturity.

It's also extremely practical. You could have used this at Shothik AI and ACI. That makes it an easy sell in interviews: "I kept hitting this problem at work, so I built the solution."

### Architecture

```
  AI Engineers / Developers                                                    
   │                                                                           
   │  Web Dashboard: create, edit, version, compare prompts                   
   │  SDK: prompthub.get("order-assistant", version="latest")                 
   ▼                                                                           
┌──────────────────────────────────────────────────────────────────────────────┐
│  Prompt Management Platform                                                  │
│                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────┐    │
│  │  Go API Server                                                       │   │
│  │                                                                      │   │
│  │  Prompt CRUD:                                                        │   │
│  │  - Create prompt with name, template, model config, variables        │   │
│  │  - Every edit creates a new immutable version (v1, v2, v3...)       │   │
│  │  - Tag versions: "production", "staging", "experiment-a"            │   │
│  │  - Diff any two versions (token-level diff)                         │   │
│  │  - Rollback to any previous version instantly                       │   │
│  │                                                                      │   │
│  │  Experimentation:                                                    │   │
│  │  - A/B test: split traffic 50/50 between two prompt versions        │   │
│  │  - Track per-version metrics (latency, cost, user rating, custom)   │   │
│  │  - Auto-promote: when variant beats control by significance         │   │
│  │    threshold, automatically promote to production                    │   │
│  │                                                                      │   │
│  │  Serving:                                                            │   │
│  │  - GET /api/v1/prompts/:name/render                                 │   │
│  │    Resolves version (by tag or explicit), fills template variables,  │   │
│  │    returns the complete prompt ready for LLM consumption             │   │
│  │  - Python SDK: one-liner integration into existing apps              │   │
│  └──────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────┐    │
│  │  PostgreSQL                                                          │   │
│  │                                                                      │   │
│  │  prompts (id, name, project_id, created_by, description)             │   │
│  │                                                                      │   │
│  │  prompt_versions (                                                    │  │
│  │    id, prompt_id, version_number,                                    │   │
│  │    system_template TEXT,        -- "You are a {{role}} assistant"     │   │
│  │    user_template TEXT,          -- "Answer about {{topic}}: {{query}}"│   │
│  │    model_config JSONB,          -- {model, temperature, max_tokens}   │   │
│  │    variables JSONB,             -- [{name: "role", type: "string"}]   │   │
│  │    created_by, created_at,                                           │   │
│  │    commit_message TEXT          -- "Improved refund tone"             │   │
│  │  )                                                                    │  │
│  │                                                                      │   │
│  │  prompt_tags (prompt_id, version_id, tag)                            │   │
│  │    -- e.g., ("order-assistant", v5, "production")                    │   │
│  │    -- UNIQUE(prompt_id, tag) — one "production" tag per prompt       │   │
│  │                                                                      │   │
│  │  experiments (                                                        │  │
│  │    id, prompt_id, control_version_id, variant_version_id,            │   │
│  │    traffic_split, status, started_at, ended_at                       │   │
│  │  )                                                                    │  │
│  │                                                                      │   │
│  │  experiment_results (                                                  │ │
│  │    experiment_id, version_id, request_id,                            │   │
│  │    latency_ms, tokens_used, cost_usd,                                │   │
│  │    user_rating INT,             -- thumbs up/down from end user      │   │
│  │    custom_metrics JSONB         -- app-specific scores               │   │
│  │  )                                                                    │  │
│  └──────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────┐    │
│  │  Template Engine                                                     │   │
│  │                                                                      │   │
│  │  Supports:                                                           │   │
│  │  - Variable substitution: {{query}}, {{context}}                     │   │
│  │  - Conditionals: {% if language == "bn" %}...{% endif %}             │   │
│  │  - Loops: {% for doc in documents %}...{% endfor %}                  │   │
│  │  - Includes: {% include "shared/safety-preamble" %}                 │   │
│  │  - Validation: type-check variables before rendering                 │   │
│  │                                                                      │   │
│  │  This is important because real production prompts are complex       │   │
│  │  templates with conditional sections, not simple strings.            │   │
│  └──────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────┐    │
│  │  Analytics Dashboard                                                 │   │
│  │                                                                      │   │
│  │  Per-prompt:                                                         │   │
│  │  - Version history timeline with diffs                               │   │
│  │  - Latency / cost / token usage per version                          │   │
│  │  - User satisfaction score trend                                     │   │
│  │                                                                      │   │
│  │  Per-experiment:                                                      │  │
│  │  - Side-by-side comparison: control vs variant                       │   │
│  │  - Statistical significance calculation (chi-squared for ratings,    │   │
│  │    Mann-Whitney U for latency)                                       │   │
│  │  - "Winner" recommendation with confidence interval                  │   │
│  └──────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  Python SDK (pip installable):                                               │
│                                                                              │
│  from prompthub import PromptHub                                             │
│  hub = PromptHub(api_key="...", project="customer-support")                  │
│                                                                              │
│  prompt = hub.get("order-assistant", tag="production")                       │
│  rendered = prompt.render(query="Where is my order?", language="en")         │
│  # Returns the full system + user message, ready for the LLM                 │
│  # Automatically logs which version was used for analytics                   │
│                                                                              │
│  # After getting the LLM response:                                           │
│  hub.log_result(prompt.request_id, rating=1, latency_ms=1200)               │
└──────────────────────────────────────────────────────────────────────────────┘
```

### Key Implementation: A/B Testing with Statistical Significance

```go
type ExperimentAnalyzer struct{}

func (a *ExperimentAnalyzer) Analyze(experiment *Experiment) ExperimentReport {
    controlResults := a.getResults(experiment.ControlVersionID)
    variantResults := a.getResults(experiment.VariantVersionID)
    
    // Compare user ratings (thumbs up = 1, thumbs down = 0)
    controlRating := a.averageRating(controlResults)
    variantRating := a.averageRating(variantResults)
    
    // Chi-squared test for rating significance
    chiSq, pValue := a.chiSquaredTest(controlResults, variantResults)
    isSignificant := pValue < 0.05
    
    // Compare latency (Mann-Whitney U test — non-parametric, no normality assumption)
    latencyPValue := a.mannWhitneyU(
        a.extractLatencies(controlResults),
        a.extractLatencies(variantResults),
    )
    
    // Compare cost
    controlAvgCost := a.averageCost(controlResults)
    variantAvgCost := a.averageCost(variantResults)
    
    winner := "no_winner"
    if isSignificant && variantRating > controlRating {
        winner = "variant"
    } else if isSignificant && controlRating > variantRating {
        winner = "control"
    }
    
    return ExperimentReport{
        ControlSampleSize:  len(controlResults),
        VariantSampleSize:  len(variantResults),
        ControlRating:      controlRating,
        VariantRating:      variantRating,
        RatingPValue:       pValue,
        IsSignificant:      isSignificant,
        ControlAvgLatency:  a.avgLatency(controlResults),
        VariantAvgLatency:  a.avgLatency(variantResults),
        ControlAvgCost:     controlAvgCost,
        VariantAvgCost:     variantAvgCost,
        Winner:             winner,
        Recommendation:     a.generateRecommendation(winner, variantRating-controlRating, variantAvgCost-controlAvgCost),
    }
}
```

### What This Proves

- You think about the **full lifecycle** of AI systems, not just building them.
- You understand experimentation methodology (statistical significance, A/B testing) — this is a rare skill among ML engineers.
- You can build developer-facing tools (SDK design, API ergonomics, dashboard UX).
- It's immediately relatable: every interviewer who has worked with LLMs has felt this pain.

---
---
# Coding-Agent Website Competitive Analysis

**Date:** 2026-05-10
**Author:** research pass for Fizeau microsite (https://easel.github.io/fizeau/)
**Method:** WebFetch against live marketing/docs sites; one site (Claude Skills) returned 404 on both candidate URLs and was dropped. Goose marketing redirected to docs at `goose-docs.ai`; that page was used in its place.

---

## Sites surveyed

| # | Site                          | URL                                            | Notes                                                                 |
|---|-------------------------------|------------------------------------------------|-----------------------------------------------------------------------|
| 1 | Cursor                        | https://www.cursor.com/                        | Flagship AI editor; heavy enterprise positioning                      |
| 2 | Claude Code                   | https://claude.com/product/claude-code         | Anthropic's coding agent (redirected from `anthropic.com/claude-code`)|
| 3 | Aider                         | https://aider.chat/                            | OSS terminal pair-programmer                                          |
| 4 | OpenAI Codex CLI              | https://github.com/openai/codex                | README-as-marketing                                                   |
| 5 | OpenCode                      | https://opencode.ai/                           | OSS multi-provider agent                                              |
| 6 | Continue                      | https://docs.continue.dev/                     | Pivoted to PR-check product; docs-first                               |
| 7 | Goose                         | https://goose-docs.ai/                         | Block's general-purpose agent                                         |
| — | Claude Skills                 | (anthropic.com/agent-skills + news/agent-skills, both 404) | Dropped — could not retrieve canonical page         |

---

## Per-site highlights

### 1. Cursor — https://www.cursor.com/

**Hero:** "Built to make you extraordinarily productive, Cursor is the best way to code with AI." Three-CTA hero (Download / Try mobile agent / Request a demo) with an *interactive* IDE mockup containing a real-looking task panel ("In Progress" / "Ready for Review"). No video; instead, animated/static UI vignettes that are believable mock products rather than abstract illustration. Uses a fictional company ("Acme Labs") as a recurring narrative anchor.

**Trust signals:** Heavy on named-CEO testimonials (Jensen Huang, Patrick Collison, Greg Brockman, Diana Hu). "Trusted by over half of the Fortune 500." SOC 2 badge. Lists model coverage explicitly (Composer 2, GPT-5.5, Opus 4.7, Gemini 3.1 Pro, Grok 4.3) — as a *capability matrix*, not just logos.

**IA:** Product / Enterprise / Pricing / Resources, with `/docs` clearly separate from marketing. Enterprise gets its own pillar. Resources fan-out (Changelog, Blog, Docs, Community, Help, Workshops, Forum, Careers) is broad but disciplined.

**Differentiation:** "Autonomy slider" framing — quotes Karpathy on the Tab → Cmd+K → full-agent spectrum. Sells *user control over autonomy*, not just speed.

**Visual:** Dark-mode code surfaces, light-mode chrome. Generous whitespace. Minimal iconography — text labels do the work. Arrow-CTAs. Fictional-product polish in every screenshot.

### 2. Claude Code — https://claude.com/product/claude-code

**Hero:** "Claude Code: AI-powered coding assistant for developers." Subhead: "Built for developers. Work with Claude directly in your codebase. Build, debug, and ship from your terminal, IDE, Slack, or the web. Describe what you need, and Claude handles the rest." Hero image is a *believable product screenshot* (a bioinformatics task), not a marketing illustration.

**Trust signals:** Logo carousel (Intercom, Spotify, Stripe, Shopify, Figma, NASA, Stripe, Asana, Databricks, Uber…). No benchmarks, no stars, no testimonial pull-quotes. Model name visible (Opus 4.7) but no comparison matrix.

**IA:** Multi-product navigation (Meet Claude / Platform / Solutions / Pricing / Resources). Marketing-vs-docs split is hard: docs live on `platform.claude.com` and `code.claude.com`.

**Demo:** Static mockups across five surfaces (Desktop, Terminal, VS Code, Web/iOS, Slack) — all with *the same prompt text*, which makes "use it where you work" concrete.

**Differentiation:** Surface ubiquity — same agent, every surface. Avoids competitor naming entirely.

**Visual:** Light/dark dual logo treatment. Inter-ish sans, neutral grey palette, blue primary CTA. Modular cards. No gradient hero.

### 3. Aider — https://aider.chat/

**Hero:** "AI pair programming in your terminal" / "Aider lets you pair program with LLMs to start a new project or build on your existing codebase." Includes a `<video>` — likely an asciinema-style terminal recording.

**Trust signals (the standout):** Quantified, instrument-style:
- 44K GitHub stars
- 6.8M pip installs
- 15B tokens/week
- "Singularity 88%" — share of new code in last release written by Aider itself (a *self-referential benchmark* that doubles as a thesis statement)

**IA:** Flat — Features / Getting Started / Documentation / Discord / GitHub. Marketing and docs blend.

**Differentiation:** "Works best with" matrix of premium models, but *runs anywhere* — explicit multi-LLM support including local. "Kind Words From Users" section header (testimonial well).

**Visual:** Emoji icons (⭐📦📈🔄). Terminal-first aesthetic. Minimal chrome. Looks like the tool it markets.

### 4. OpenAI Codex CLI — https://github.com/openai/codex

**Hero:** "Codex CLI is a coding agent from OpenAI that runs locally on your computer." A splash PNG, then quickstart commands.

**Trust signals:** GitHub-native (81.4k ★, 11.8k forks, 780 releases, latest 0.130.0 dated 2026-05-08). No benchmarks, no testimonials.

**IA:** Quickstart-first README. Platform-specific install. ChatGPT plan integration noted. No live demo — just install commands.

**Differentiation:** "Local, lightweight" vs the IDE/desktop/web variants. Implicit positioning by *naming the alternatives in the same nav*.

**Visual:** Pure GitHub README. Rust-monorepo aesthetic. Minimalist.

**Lesson:** A high-traffic README *can* be the marketing site if the install command is the headline.

### 5. OpenCode — https://opencode.ai/

**Hero:** "The open source AI coding agent" / "Free models included or connect any model from any provider, including Claude, GPT, Gemini and more." Install commands (curl / npm / brew / paru) shown *as the primary CTA*. Embedded video element.

**Trust signals:** "150K GitHub Stars / 850 Contributors / 6.5M Monthly Devs" as three big numerals. Star count duplicated in nav (158K). "75+ LLM providers through Models.dev, including local models."

**IA:** GitHub link / Docs / Zen / Go / Enterprise / Download. Desktop-app banner. Multi-pillar product line.

**Differentiation:** Privacy ("does not store any of your code or context data"), platform breadth (terminal + IDE + desktop), works with existing subscriptions (Copilot, ChatGPT Plus/Pro).

**Visual:** Pixelated geometric logo with multiple grey fills. Monochrome neutral palette. Dual light/dark logo variants. Quietly technical.

### 6. Continue — https://docs.continue.dev/

**Hero:** "Continue runs AI checks on every pull request." Subhead: "Each check is a markdown file in your repo that shows up as a GitHub status check — green if the code looks good, red with suggested fix if not."

**Trust signals:** None on this page. No stars, no testimonials. Notable that they ship *without* social-proof scaffolding.

**IA:** Docs-first. Sidebar: Getting Started / Writing Checks / Mission Control / Integrations. Top nav: Docs / Blog / Sign in.

**Demo:** Inline markdown code block showing a "Security Review" check definition. The *artifact is the demo* — you read what a check looks like before you scroll past the fold.

**Differentiation:** Crisp pivot — narrows scope from "AI assistant" to "AI PR checks as markdown files in your repo." Scoping is the differentiator.

**Visual:** Light logo, minimal chrome, dark-mode hinted by light-variant naming.

### 7. Goose — https://goose-docs.ai/

**Hero:** "Your native open source AI agent. Desktop app, CLI, and API — for code, workflows, and everything in between." Subhead expands scope beyond code (research, writing, automation, data analysis). CTAs: Install goose / Quickstart.

**Trust signals:** "38k+ GitHub stars / 400+ Contributors / 70+ MCP extensions." Provider breadth: "15+ providers — Anthropic, OpenAI, Google, Ollama, OpenRouter, Azure, Bedrock." Governance signal: "Agentic AI Foundation at Linux Foundation for vendor neutrality."

**IA:** Quickstart / Docs / Tutorials / MCPs / Blog / Resources. Skills Marketplace and Recipe Generator under Resources — *recipes as a first-class category*.

**Differentiation:** Open standards (MCP, ACP). Local execution. Vendor-neutral governance. Generality (not "just code").

**Visual:** Dual logos. Emoji-based feature icons (🖥️🔌🤖📋🧩🔀🔒). Light docs aesthetic.

---

## Top 10 best practices to adopt (prioritized)

| # | Practice | Source(s) | Where it lands on Fizeau |
|---|---|---|---|
| 1 | **Lead with quantified, instrument-grade trust numerals.** Aider's "44K ★ / 6.8M installs / 15B tokens/week / Singularity 88%" and OpenCode's three-number block convert credibility into a chart. | Aider, OpenCode, Goose | Hero strip on `/`. Three or four numerals: install count, MIT-licensed, models supported, p50 latency on canonical bench. Consistent with our DESIGN.md "the data is the headline" principle — the homepage hero should literally *be* a measurement. |
| 2 | **Show side-by-side speed/cost benchmarks above the fold on the benchmarks page.** Aider's self-benchmark and Cursor's model matrix both reduce ambiguity by *naming* the comparators. | Aider, Cursor | `/benchmarks/` landing: a single hero chart with 3-4 named runtimes (Fizeau, Claude Code, Aider, OpenCode) on one axis and median tool-call latency on the other. Cite snapshot date and sample size (DESIGN.md principle 2). |
| 3 | **Install command as primary CTA, not a download button.** OpenCode and Aider both put `curl … \| sh` and `pip install` *in* the hero. | OpenCode, Aider, Codex | Replace any "Get Started" button with a copy-able install line in the hero block. We already have `install.sh` — wire it. |
| 4 | **Real product screenshots, not marketing illustration.** Cursor's "Acme Labs" task panel and Claude Code's bioinformatics screenshot both look like *the actual app doing real work*. | Cursor, Claude Code | `/demos/` and the homepage need at least one screenshot of an actual fiz run on a recognizable repo, complete with timestamps and a real prompt. Avoid generic gradient illustrations. |
| 5 | **Asciinema/terminal recording for a tool whose primary surface is the terminal.** Aider does this; OpenCode embeds video. | Aider, OpenCode | Embed an asciinema cast on `/` and at the top of `/demos/`. Looped, no audio, ~30s. Aligns with DESIGN.md "scientific instrument" voice better than a screencast video. |
| 6 | **Model coverage matrix as a first-class section.** Cursor lists every supported model by name; OpenCode quantifies "75+ providers." Goose lists 15+ explicitly. | Cursor, OpenCode, Goose | Add a "Models" page (or section under Docs) with a literal table: provider × model × verified-on date. We already have `/routing/` — promote a model coverage matrix there. |
| 7 | **Recipes / cookbook as a top-level navigation category.** Goose surfaces "Recipe Generator" alongside Docs. Continue's "Writing Checks" sidebar is recipe-shaped. | Goose, Continue | Promote `/resources/` to "Recipes" or add a `/recipes/` top-level nav item with 6-10 concrete tasks (e.g., "Score a benchmark run", "Wire up a custom harness", "Embed Fizeau in a Go service"). |
| 8 | **Crisp scoping statement instead of "AI assistant for everything."** Continue's "AI checks on every pull request" and Aider's "AI pair programming in your terminal" both win in five seconds. Claude Code's "ship from your terminal, IDE, Slack, or the web" enumerates surfaces. | Continue, Aider, Claude Code | Rewrite homepage subhead to one sentence with a *concrete noun*: e.g., "Embeddable Go agent runtime — local-model-first via LM Studio." (We already have this in `hugo.yaml description` — promote it into the rendered hero.) |
| 9 | **Provenance metadata visible in trust numerals and benchmarks.** No site does this well — it's a Fizeau differentiator we already wrote into DESIGN.md ("snapshot date, sample size, what was filtered out, why"). | (gap in field) | Every numeral on `/` and `/benchmarks/` must show snapshot date and sample size in a caption. Make this the visual signature that competitors *can't* copy. |
| 10 | **Differentiation framing as a slider, axis, or dimension — not a feature list.** Cursor's "autonomy slider" (Tab → Cmd+K → agent) and OpenCode's "your subscription, your model, your machine" both pick *one axis* and own it. | Cursor, OpenCode | Pick one dimension Fizeau owns and put it on `/`. Candidate: *latency-vs-capability slider* — local tiny models for low-latency tools, cloud frontier models for hard reasoning, same runtime. Tie it to our routing story. |

---

## Top 5 patterns we should NOT adopt

| # | Pattern | Seen at | Why we skip |
|---|---|---|---|
| 1 | **Generic SaaS gradient hero with abstract illustration.** Most of the field has *avoided* this — but it remains the default Hugo theme failure mode. None of the seven sites used purple-pink gradients or robot mascots. | (cautionary; not seen) | Our DESIGN.md commits to instrument aesthetics. Don't regress. |
| 2 | **Logo-carousel-as-only-trust-signal.** Claude Code's hero relies on enterprise logos with no numbers, no testimonial text, no benchmarks. It works for Anthropic because the brand carries the page; it would not work for Fizeau. | Claude Code | We have no Fortune 500 customers; faking gravitas with logos would read as cargo-cult. Use numerals + provenance instead. |
| 3 | **Five-product mega-nav before product-market fit.** Cursor's Product menu (Agents / Code Review / Cloud / Tab / CLI / Marketplace) and OpenCode's Zen/Go/Enterprise pillars are appropriate for their scale; for a single-product project they fragment attention. | Cursor, OpenCode | Keep top nav to 4 items (current state — Docs / Benchmarks / Demos / GitHub). Don't add Enterprise / Pricing / Solutions tabs we don't have. |
| 4 | **Vague hero verbs ("ship", "build", "transform", "supercharge").** Claude Code and Cursor both deploy these. They survive only because the brand is already known. | Claude Code, Cursor | Use measurement verbs: *measure, route, embed, run*. Match the Fizeau brand grounding. |
| 5 | **Self-referential percentage benchmarks ("88% of our code was written by us").** Aider's "Singularity 88%" is striking, but it conflates marketing with measurement and would undercut Fizeau's calibration thesis. | Aider | Our benchmarks page is the brand. Vanity numbers there would actively harm credibility. Always cite an external comparator. |

---

## Cross-reference summary

For Fizeau specifically, the highest-leverage moves are:

- **Hero rewrite** (practices 1, 3, 8): replace any current marketing-style hero with *one sentence + install command + three measured numerals with provenance captions*. This single change pulls 3 of 4 best-of-field patterns into our most-viewed page.
- **`/benchmarks/` becomes the second pillar of the brand** (practices 2, 9): named comparators, snapshot date, sample size, p50/p99 split. We already have the data infrastructure; the site does not yet front-load it.
- **`/demos/` gets an asciinema cast and at least one real-repo screenshot** (practices 4, 5): the field treats demos as either video (OpenCode), static screenshot (Claude Code, Cursor), or terminal cast (Aider). Asciinema is the cheapest match for our aesthetic.
- **Promote `/routing/` to host a model coverage matrix** (practice 6): we already differentiate on local-first routing; the matrix makes it legible to a five-second visitor.
- **Add `/recipes/` to top-nav** (practice 7): we have material in `/resources/` that should be reframed as a cookbook.

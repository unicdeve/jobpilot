# ADR-0017: Schema-constrained model output, and what it does not buy

## Status

Accepted, and implemented in full.

One part was reversed during implementation; see "Correction" below.

This implements the "Structured output" paragraph of `docs/01_architecture/05_llm.md`, which has been unimplemented since it was written.
It does not change the provider abstraction's direction, only the shape of a request.

## Context

`05_llm.md` asks for two things:

> Prefer schema-constrained structured output where the provider supports it.
> Validate every model response before it reaches domain logic.

Neither is implemented.
`ports.Request` carries a system prompt, a user prompt, a temperature and a token cap, and nothing else.
Every structured call asks for JSON in prose, then runs `stripJSONFence` over the reply and hopes `json.Unmarshal` succeeds.

Four call sites decode structured JSON:

| call site | what it decodes | validation today |
| --- | --- | --- |
| `MatchingService.parseMatchResponse` | score, confidence, five lists, rationale | score range only |
| `ResumeService.ProposeProfileDraft` | the whole profile draft | substantial — URL guards, link kinds, date parsing, dedup |
| `JobDiscoveryService.extractJobFromPage` | title, company, locations, salary, skills | none beyond an empty-title check |
| `ApplyControlChooser` | one index into a list of controls | bounds check and an explicit abstain value |

Two further sites — cover letters and question answers — ask for prose, not JSON, and are out of scope.

The validation column is the interesting one.
It ranges from "substantial" to "none", and the difference is not a judgement about risk: it is a record of which defects happened to be found.

### What was measured

Against the local `llama3.2:1b` on Ollama 0.34, using the real prompts and the committed resume fixture, whose correct values are known.
The user's configured model is a cloud one; a small local model is both the cheaper subject and the honest one for a local-first product.

**Match extraction, 20 runs each:**

| | parsed |
| --- | --- |
| prompt only (today) | 19/20 |
| `format: "json"` | 20/20 |
| `format: <schema>` | 20/20 |

So the everyday failure rate today is about 5%, not a catastrophe.
It is a matching run that fails for no reason the user can see or act on.

**Profile draft extraction, 10 runs each:**

| | invalid link kind | non-ISO date |
| --- | --- | --- |
| prompt only (today) | 10/10 | 4/10 |
| `format: <schema>` | 0/10 | 8/10 |

Every single prompt-only run proposed `Blog` as a link kind, which is not one of the six the profile has.
A schema with an `enum` eliminated that entirely.

The date column went the wrong way, which is the useful part.

**Which schema keywords actually constrain the model:**

| keyword | enforced | evidence |
| --- | --- | --- |
| object shape, types, `required` | yes | no malformed or missing-field output in 30 runs |
| `enum` | yes | forced to a listed value every time |
| `pattern` | **no** | `^\d{4}-\d{2}$` returned `".role started in July 2022."` |
| `minimum` / `maximum` | **no** | asked for 5000 of a 0–100 integer, returned `0` |

Ollama compiles a schema to a grammar, and a grammar can express "a string here" but not "a string matching this regex".
`pattern` is decorative.
Anything relying on it is relying on nothing.

### The finding that shapes the decision

A schema makes output well-formed. It does not make it correct, and applied carelessly it makes wrongness harder to catch.

Both failures above are the same shape:

- Asked to classify a Facebook link with an `enum` of six kinds, the model returned `linkedin`.
  Without the enum it returns `facebook`, and `ProposedLinks` drops it, because it is not a kind the profile has — the right outcome.
  With the enum it returns a kind the profile does have, attached to the wrong URL, and no downstream check can tell.
- Asked for a score of 5000 with a 0–100 bound, the model returned `0`.
  Without the bound it returns 5000 and `parseMatchResponse` rejects it.
  With the bound it returns a number in range that means nothing.

In both cases the constraint converted an obviously-wrong answer into a plausibly-wrong one.
That is precisely the failure family ADR-0016 was about: `ub` is caught because it is visibly not a URL, and `taiwoogunoladev@gmailcom` is not, because it looks like one.
A change that improves a metric while removing the signal a guard relies on is a regression wearing a success's clothes.

## Decision drivers

- Never invent candidate facts (principle 6). A forced choice from an enum is an invented fact when the true answer is not on the list.
- Deterministic rules must not be delegated to the model (principle 7). A schema is the model's constraint, not ours.
- Local-first: four backends are offered — Ollama, llama.cpp, LM Studio, and any OpenAI-compatible endpoint — and only Ollama's behaviour is measured here. Nothing may require a capability a user's backend lacks.
- `05_llm.md` asks for validation of *every* response, not of the ones a schema could not cover.

## Options

### A. JSON mode only (`format: "json"`)

Fixes parseability, costs almost nothing, and is the most widely supported setting.
It does not address invalid link kinds, which is the defect that actually reached the profile in 10 runs out of 10.

### B. Full schema, including `enum` and bounds, everywhere

The measurement above is the argument against.
It would silently replace "wrong and detectable" with "wrong and plausible" in exactly the two places we have evidence for.

### C. Retry on parse failure, no schema

Treats the symptom, costs a round trip on a local model, and does nothing for a response that parses but is invalid.

### D. Schema for structure, explicit abstention, validation retained

Take the part that is measurably free — JSON validity and object shape — and refuse the part that trades detectability for tidiness.

## Decision

Option D, in five parts.

### 1. A schema is an optional part of a request

`ports.Request` gains an optional response schema.

A provider that cannot apply a schema ignores it and behaves exactly as it does today.
This is not a fallback path bolted on: it is the normal path for three of the four backends until each is measured, and it must stay correct.

### 2. Schemas describe shape, never meaning

A schema may state the object's fields, their JSON types, which are required, and the shape of arrays.

A schema may not use `enum` or numeric bounds to make the model choose among domain values where "none of these" is a truthful answer.
Where the model may not know, the schema offers an explicit way to say so — a nullable field, or a listed "unknown" — so that not knowing is expressible rather than rounded to the nearest option.

`ApplyControlChooser` already works this way, with an abstain index the model can return, and it is the pattern the others follow.

### 3. `pattern` is not used

It is not enforced, and writing it would imply a guarantee that does not exist.
Dates continue through `ParseDraftDate` and `ProfileDraft.Normalize`, which read what the model actually writes.

### 4. Validation stays, and the thin cases get thicker

Every structured response is validated after decoding, whether or not a schema was applied, because three backends will not have applied one.

The two thin call sites are brought up to the standard of the two thick ones.
`extractJobFromPage` in particular decodes salary figures, locations and an employment type with no checking at all, on input that ADR-0013 and principle 8 both classify as untrusted.

### 5. The schema lives next to the struct it describes

A schema and the Go type it constrains drift the moment they are apart.
Each is declared beside its struct and covered by a test that fails when a field is added to one and not the other.

## Consequences

- The ~5% unparseable rate on Ollama goes to zero in 20 runs, and invalid link kinds go from every run to none.
- Nothing changes for llama.cpp, LM Studio or a custom endpoint until each is measured. They keep today's behaviour, which is why part 4 matters more than part 1.
- **This does not make the model more accurate**, and one naive version of it would have made the profile less accurate. The honest summary is that it removes a class of avoidable failure and leaves the important one exactly where it was: in the validation layer and in the user's review.
- Two more things to keep in step — a schema and a struct — which part 5 exists to contain.
- `stripJSONFence` stays. A schema-constrained reply does not need it, and the three backends without schema support still do.

## Correction: the capability flag was dead code

This ADR originally said `ports.LLMProvider` would also report whether a backend honours a schema, and the implementation added an `LLMCapabilities` struct for it.

It was removed before the work was finished, because nothing read it.

Every caller states the shape and validates the reply regardless of the answer, so no branch depended on the flag; each adapter already knows what it can apply, and applies it.
A field that is written and never read is exactly what ADR-0015's closing change deleted three columns for — `work_preferences` was read by a single rule that could never fire, which is a worse state than not having the column, because the code looks like it does something.

The measurements the flag would have carried live in the adapters, next to the decision each one makes: Ollama says what was measured and applies the schema, and `chatCompletionRequest` says why it has no `response_format` field.
If a caller ever needs to branch on this — showing the user whether their backend is constrained, say — the flag comes back then, with a consumer.

## Open questions

1. **Do llama.cpp and LM Studio honour `response_format: json_schema`, and which keywords?** Both advertise it and both are OpenAI-compatible, but the measurement above is the reason not to take an advertisement as evidence. Neither is installed here; this needs a run against each before their capability is reported as true.
2. **Should a validation failure retry once before surfacing?** A retry is cheap locally and free of the laundering problem, since it re-asks rather than constrains. It is out of scope here because it is a separate decision about cost and latency, and because it should be measured against the post-schema failure rate, not today's.

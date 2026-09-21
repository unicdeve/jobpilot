# ADR-0015: Normalize the candidate facts that forms actually ask for

## Status

Accepted.
The three questions this was drafted with open — what happens to existing data, whether to offer LLM extraction, and where salary lives — were answered and are folded into the decision below.

## Context

`domain.Candidate` has fourteen fields.
Nine of them are free text: `Summary`, `WorkPreferences`, `SalaryPreferences`, `WorkAuthorization`, `ApplicationPreferences`, `Skills`, `Experiences`, `Education`, `Certifications`, `Links`.
`Contact` is a tenth: one string holding, in practice, an email address and maybe a phone number.

`AnswerService.Resolve` answers a form question by substring-matching its label against a `switch` over those fields (`profileFactAnswer`), then against one derivation rule (`derivedAnswer`), then — for narrative questions only — the LLM, then the user.

This was described in `current_state.md` as a thinness: answers are shallow because the data behind them is unstructured.
That description is too kind.
Running the real resolver against the question labels recorded from three real applicant tracking system forms (`internal/adapters/secondary/browser/testdata/ats`) shows answers that are wrong, not merely absent:

| Form question | Resolved answer | Why |
| --- | --- | --- |
| Phone (Ashby, Greenhouse) | the candidate's **email address** | `email`, `phone` and `contact` share one case over `Candidate.Contact` |
| LinkedIn, Portfolio, Website | the **entire links blob**, GitHub URL included, in each | one case over `Candidate.Links` |
| GitHub, Twitter | nothing | the switch does not list them, though the URLs sit in the blob |
| "Will you require Visa Sponsorship now, or in the future?" | a paragraph of prose | `Candidate.WorkAuthorization` is prose; the field is Yes/No |
| "Your authorization to work in the country where you live" | the same paragraph | as above; the field is a choice |
| "Are you currently based in any of these countries? [list]" | "Berlin, Germany" | matched on "based" → `Candidate.Location` |
| "What are your salary expectations annually ?" | prose | `Candidate.SalaryPreferences` is prose |

These are not hypothetical.
`PrepareApplication` fills them into the live page, and the browser worker then leaves that page open for the user to submit.
The review step is what stands between these answers and an employer, which is exactly the load ADR-0014 decided review should carry — but it should not have to carry mistakes we could have not made.

There are also questions these forms ask that we could answer and do not:

- "Current company" is in the `Experiences` blob.
- "When are you available" has no field at all.
- Years of experience with a skill has no field, and `derivedAnswer`'s own comment says why it computes nothing: `Experiences` is prose rather than dates.

**Two problems, and one enables the other.**
The matcher is too coarse — `Phone` should never have shared a case with `Email`.
The shape is wrong — `Contact` is one string, so there is no phone number to match *to*.
Fixing the matcher alone is impossible for the same reason it broke: you cannot map a question to a fact that is not stored separately.

An earlier justification for this work, recorded in `current_state.md`, was that a policy engine deciding whether to submit on the user's behalf should not reason over JSON-blob candidate data.
ADR-0014 removed that policy engine, so that argument is gone.
The table above replaces it, and is a better reason than the one it replaces.

## Decision drivers

- Never invent candidate facts (core principle 6). A wrong answer typed into a form is a fact invented about a person.
- Deterministic business rules must not be delegated to an LLM (core principle 7).
- Keep the core simple, and avoid infrastructure not justified by a documented requirement (principles 9 and 10). A candidate profile can become a CV ontology very easily, and should not.
- Resume-derived profile data is review-gated and never auto-written (ADR-0011).

## Decision

### 1. The recorded forms are the specification

Normalize exactly the facts a real application form asks for, evidenced by the pages in `testdata/ats`.
Everything else stays prose.

This is the discipline that keeps the change bounded.
"What would a complete CV model look like" has no natural end.
"What did Ashby, Greenhouse and Lever actually ask" has twenty-odd answers, and we have them written down.

### 2. Columns for facts a person has one of

On `candidates`:

- `email`, `phone` — replacing `contact`
- `location_city`, `location_region`, `location_country` — replacing `location`, because Greenhouse asks for Country and City as separate fields
- `remote_preference` — the same vocabulary as `jobs.remote_type`, so a match can compare rather than read
- `notice_period_days`, `earliest_start_date`, `willing_to_relocate`

### 3. Tables for facts a person has several of

- `candidate_links` — `kind` (linkedin, github, portfolio, website, twitter, other), `url`
- `candidate_work_authorizations` — `country`, `is_authorized`, `requires_sponsorship`
- `candidate_skills` — `name`, `years_experience`
- `candidate_experiences` — `company`, `title`, `start_date`, `end_date`, `is_current`, `description`

Education and certifications stay prose, which is a narrowing of this list
made while building stage two and recorded here rather than quietly.

§1 says the recorded forms are the specification, and by that rule these do
not qualify: no recorded form asks about a degree, an institution or a
certification, and nothing in the codebase reads either field — they are
written and never used. Structuring them would be schema built for a
requirement nobody has stated, which principle 10 rules out. A recorded form
that asks for them is the trigger to revisit, and adding such a fixture is
the honest way to justify it.

The two that stay have concrete, existing consumers:

- **Experiences** because "Current company" is asked by two of the three
  recorded forms and cannot be answered at all today, and because dated
  entries give a total years of experience comparable with `jobs.experience_min`
  and `jobs.experience_max`, which already exist and are currently unused.
- **Skills** because `jobs.skills` is already a list, and matching compares
  it against a prose blob.

`candidate_skills.years_experience` is the one field here with no recorded
form behind it. It is included because a per-skill years question is common
in the wild and nothing else could answer one, and that gap in the evidence
is worth stating rather than glossing.

`candidate_experiences` is what makes "current company" and "years of experience" answerable at all, and `candidate_work_authorizations` is what turns a sponsorship question from a paragraph into a Yes or a No for the country being asked about.

### 4. Salary belongs to the saved search, and a form's salary question stays unanswered

`SearchCriteria` already carries `salary_min` and `salary_max`, so the filter is already where it belongs and nothing needs adding.

What does not follow is answering a form from it.
"Do not show me jobs below 85k" and "what are your salary expectations" are different facts with different audiences.
The first is what the user will look at; the second is what they will tell an employer, and people phrase that deliberately.
Typing a search floor into a salary-expectations box would tell an employer the user's minimum, which is both bad for them and a thing they never said — inventing a candidate fact, in the sense principle 6 forbids.

So a salary question on a form resolves to needs-user-input, and the user answers it.
That is strictly better than today, where it resolves to a paragraph of prose.

If the question is common enough to be worth automating later, the fix is a candidate field that means "the number I tell employers", explicitly distinct from the search floor — not a derivation from it.
Note also that nothing currently records which saved search produced a job, so deriving from the search would need that provenance link first.

### 5. What stays prose, deliberately

`summary`, `application_preferences` (cover-letter tone), and each experience's `description`.
These are read by a person, or by an LLM writing prose.
Structuring them buys nothing and costs the user a form to fill in.

### 6. Self-identification data is out of scope

Every recorded form asks about gender identity, ethnicity, sexual orientation, transgender status, disability and veteran status.
None of it is normalized here, and none of it is stored.

It is optional on every form, it is the most sensitive data on the page, and the honest default is that the user answers it themselves, per application, or declines to.
Storing it to autofill it would be JobPilot deciding that answering is the default.
This is a deliberate exclusion, not a gap.

### 7. The blobs are dropped, and the resume repopulates the profile

The migration drops the blob columns rather than carrying them alongside the new structure.
JobPilot has no users beyond this machine and no data worth preserving, so a compatibility shim would be permanent complexity bought for a one-off.

The resume survives, which is what makes this safe.
`resumes.markdown_content` is untouched by this change, so the source material every blob was written from is still on disk.

Alongside the schema, a user-triggered **"extract my profile"** action reads the stored resume and proposes structured values — skills with years, experiences with dates, education, links, contact details.
It is review-gated exactly as ADR-0011 requires: the model proposes, the user accepts or edits each field, and nothing reaches the profile unreviewed.
This is the existing resume-suggestion path widened from Summary and Skills to the rest of the structured profile, not a new mechanism.

Work authorization is the exception and is left for the user to state.
A resume does not reliably say which countries someone may work in, and guessing it would put an invented fact into a Yes/No question an employer asks under penalty of the user's honesty.

### 8. The matcher is rewritten in the same change, not after it

The defects are shape and matching together.
A normalized schema behind the same coarse `switch` would still put an email address in the phone box.

The recorded form labels become the test: "what would we answer on this real page" turns into an assertion, the same way `testdata/ats` turned extraction into one.
Every question in the table at the top of this document is a case.

### 9. Sequencing

Two stages, because they have different value and different size.

**Stage one fixes wrong answers.** Contact, links, work authorization and availability.
These are the rows in the table above — small schema, immediate correctness.
Salary needs no schema at all: dropping the prose field turns a wrong answer into an honest needs-user-input.

**Stage two enables new ones.** Experiences and skills, and the extraction action that fills them.
Larger, and its payoff is derivations we cannot do at all today — current company, years of experience — rather than mistakes we are currently making.

Stage one is worth doing even if stage two is deferred indefinitely.

## Consequences

- The largest schema change since the initial migration, and a destructive one: the blob columns go, and what is in them is not recovered except by re-extracting from the resume.
- `domain.Candidate`, the candidate repository, the profile service, the profile UI and onboarding all change shape.
- `MatchingService` and `CoverLetterService` currently interpolate `Skills` and `Experiences` prose into prompts. They keep working by rendering the structured data back into text for the prompt, which is a better input than the blob was.
- `profileFactAnswer` and `derivedAnswer` are rewritten and, for the first time, tested against real form labels rather than invented ones.
- `docs/02_features/02_candidate_profile.md` and `07_application_answers.md` need updating, and `current_state.md`'s normalization gap can finally be closed rather than restated.
- Until the user fills in the structured profile — by hand or via extraction — JobPilot answers less than it does today. An unanswered question is left for the user, which works; a wrongly answered one goes to an employer.

## Alternatives considered

**Fix the matcher only.**
Cheapest, and it does remove the worst defect: splitting the `email`/`phone` case stops an email address going into a phone box, by answering neither.
But it cannot answer the phone question, or tell LinkedIn from GitHub, or turn a sponsorship paragraph into a Yes.
It is worth doing immediately as a stopgap — see "Open questions" — but it is not the fix.

**Normalize everything a CV contains.**
A full model — publications, references, languages, projects, awards — has no natural stopping point, and every field is one the user has to fill in before it earns anything.
Principle 10 is explicit that infrastructure needs a documented requirement, and "a CV might have it" is not one.

**Keep the blobs and ask an LLM to extract a fact per question at resolve time.**
Turns every form field into a model call, makes the same question answerable differently on two runs, and puts an LLM in the path of factual answers — which `Resolve` deliberately prevents today, by construction rather than by prompt (principle 7, and AC-003).

## Open questions

- **The stopgap.** The email-in-the-phone-box defect is live now and the schema change is not small.
  Splitting that one switch case is a few lines and stops a wrong answer reaching a form.
  Worth shipping before this ADR is implemented rather than waiting for it.
- **Whether `candidate_skills.years_experience` is entered or derived.**
  It can be stated by the user, or computed from the experiences that mention the skill.
  Derived is better data and needs the experience descriptions to be more than prose, so stated is the likely start.
- **Whether a job records the saved search that found it.**
  Not needed by this decision, but §4 notes it would be a prerequisite for ever answering a salary question from a search, and Scouts (ADR-0014) will want the same link to report what a run produced.

## Resolved while drafting

- **Existing data.** Dropped, not migrated. There is nothing on this machine worth a compatibility shim, and the resume it all came from is still there.
- **LLM extraction.** Offered, as a user-triggered action, review-gated under ADR-0011.
- **Salary.** Belongs to the saved search, where `SearchCriteria` already keeps it. A form's salary question is not answered from it — see §4.

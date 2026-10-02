# ADR-0026: Workable and SmartRecruiters

## Status

Proposed, and implemented with it.
It records how `docs/02_features/17_company_watchlists.md` D6's two remaining providers are read, once measured on 2026-10-02.
It changes ADR-0024 §4's port: a board may be listed without its descriptions, which are then read only for the postings a search may keep (§2).

## Context

ADR-0024 reads Greenhouse, Lever and Ashby, and D6 left Workable and SmartRecruiters until their public endpoints had been measured.
They were measured on 2026-10-02, from the boards that appear in the wild: 20 Workable accounts linked from Hacker News' "Who is hiring?" threads of February to October 2026, and 13 SmartRecruiters companies with open postings, out of 40 probed.
Workable answered for 18 of the accounts, with 182 postings; it refused the other two every time they were asked.
No job board JobPilot reads links to either provider; their listings link to the boards' own pages.

What was found:

- **Workable** lists an account's every published posting, description included, in one answer: `apply.workable.com/api/v1/widget/accounts/{account}?details=true`.
  Its `description` holds the whole posting; the per-posting endpoint splits the same text into description, requirements and benefits.
  An account that does not exist is a 404.
  The answer names the company (`"name":"Hugging Face"`).
- Workable limits how often it is asked.
  From the measuring environment, whose address is shared, roughly one request in three was refused with 429 even with fifteen seconds between requests, and its `retry-after` did not count down in seconds or milliseconds.
  A candidate's own address will see less of this, but not none.
- A Workable posting's own address in the answer, `apply.workable.com/j/{code}`, names no account; Workable redirects it to `apply.workable.com/{account}/j/{code}`.
  Six of the Hacker News links were this short form.
  An account's older address, `{account}.workable.com`, redirects to `apply.workable.com/{account}/`.
- **SmartRecruiters** lists a company's postings a hundred at a time, with no descriptions: `api.smartrecruiters.com/v1/companies/{company}/postings`.
  Each description is a request of its own, to the posting's endpoint.
  Bosch's 4,822 postings are 49 list requests in 40 seconds, and would be 4,822 more to describe: 120 in a row took 67 seconds and none was refused.
- SmartRecruiters answers a company that does not exist as a company with no postings, as Lever's API does: `{"totalFound":0,"content":[]}`.
  Its careers page tells them apart.
  A company that does not exist is redirected to `jobs.smartrecruiters.com`, SmartRecruiters' search across every company.
  One that does is either a page titled "Careers at Visa", or a redirect to its own careers site: Bosch to `jobs.bosch.com`, Wise to `wise.jobs`, CERN to `careers.cern`.
  Visa, Spotify and Deloitte have such a page and no postings.
- Every SmartRecruiters posting names its company (`"company":{"name":"Bosch Group"}`), and identifiers are not case sensitive.
- **Both give one place per posting.**
  Every one of SmartRecruiters' 9,024 postings, and every one of Workable's 182, has exactly one location.
  SmartRecruiters' is a city even for a remote posting: Bosch's "AI and Cloud Development Engineer - Remote" is in Timișoara.
  Of Workable's 79 remote postings, 33 have a title naming a region wider than their one place.
  Modash lists each of its "(remote, Europe)" roles once per country, in Tallinn, Riga, Bucharest and others; Hugging Face lists its "EMEA Remote" roles in Paris, and Seeq its "APAC" roles in Singapore.
  Of SmartRecruiters' 425 remote postings, 7 do, and those name the role's scope, not where it is done: ServiceNow's "Global Head" role is posted once for each US city it hires in.
- Workable marks some locations `hidden`, which its page does not show: 9 of 182, at Hugging Face and Sumble.
  Whether hidden or not, Hugging Face's EMEA roles say Paris.
- Workable says whether a posting is remote (`telecommuting`) and nothing more; SmartRecruiters has `remote` and `hybrid`.

## Decision

### 1. Two more providers

`domain.ProviderWorkable` and `domain.ProviderSmartRecruiters`, with no region.
A board's name is the account (`apply.workable.com/huggingface`, `huggingface.workable.com`) or the company identifier (`careers.smartrecruiters.com/BoschGroup`, `jobs.smartrecruiters.com/BoschGroup/{posting}`).
Workable's own subdomains (`jobs.workable.com` is its search across every account) and SmartRecruiters' own paths (`external-referrals`, `oneclick-ui`) are not boards.

Workable's widget is read for a board.
A SmartRecruiters board is its list, every page of it; a list with nothing in it is checked against the careers page, and a redirect to `jobs.smartrecruiters.com` is `ports.ErrBoardGone`.
A posting's canonical URL names its board: `apply.workable.com/{account}/j/{code}/` and `jobs.smartrecruiters.com/{company}/{id}`, so `domain.BoardOf` finds the board from a stored job.

### 2. A board may be listed without descriptions

```go
type BoardContents struct {
    Jobs        []Job
    FetchedAt   time.Time
    Undescribed bool
}

type BoardReader interface {
    ReadBoard(ctx, board) (*domain.BoardContents, error)
    DescribeJobs(ctx, board, jobs []domain.Job) ([]domain.Job, error)
    BoardName(ctx, board) (string, error)
    ResolveAddress(ctx, address string) (string, error)
}
```

A SmartRecruiters board is `Undescribed`.
The company-pages source applies `domain.SearchCriteria.MayMatch` to its postings, and describes only those that pass; discovery then applies the criteria in full, as it does to every source.
`MayMatch` applies every criterion a description cannot change — title, locations, remote type, employment type, salary and excluded companies — and none that it can: keywords, where the job can be done from and its hours.
So a posting it rules out, `MatchesFor` would too, which a test holds for every search template.

Descriptions are kept by posting and release date, so a later read describes only new or re-released postings, and a board's are dropped when it is next listed without them.
They are kept in memory, not stored: storing every posting of a watched company is what D3 rejected.
Postings are described four at a time per board.
A posting that has closed since it was listed is left out; any other failure to describe fails the board's read, which names the company as failing (ADR-0024 §5).

A search by keywords alone narrows nothing before the description, so it describes every posting of the board on its first read.
That is the cost of the decision: Bosch's first such read is minutes long, and each read after it describes only what is new.

### 3. A Workable short address is followed

`domain.ParseBoard` refuses `apply.workable.com/j/{code}` with `ErrWorkablePostingAddress` rather than reading it as an account called "j".
Watching a company by that address follows Workable's one redirect, through `BoardReader.ResolveAddress`, and watches the account it leads to; the address the candidate gave is kept as where the company was added from.

`domain.BoardOf` stays a reading of the job's own text and fetches nothing, so a Hacker News posting that links only to a short address does not offer "Watch this company".
A job read from a Workable board has a canonical URL that names its account, so it does.

### 4. A posting's one place is read as it is given

Each provider's one location is what ADR-0023 reads, as Greenhouse's, Lever's and Ashby's lists are (ADR-0024 §6): Workable's city, region and country, and SmartRecruiters' `fullLocation`.
A Workable location marked hidden is not read: the company did not publish it.
The workplace type is read where it is given: SmartRecruiters' `remote` and `hybrid`, and Workable's `telecommuting` as remote; a Workable posting that is not remote has no type, since hybrid and on-site are not told apart.

A title naming a wider region does not widen the reading, as ADR-0023 §1 says.
So Hugging Face's "EMEA Remote" roles read as France, and a candidate elsewhere in EMEA who filters by where they can work does not see them.
Modash's per-country postings read as what they are.
Whether a title's region should make a single-place reading less certain is a question for ADR-0023 and every source, not for two providers, and is left open.

### 5. Workable's limit is a failed read, not a gone board

A 429 is `ports.ErrBoardBusy`, a failed read: the company is named as failing for that search and asked again at the next, as a board that does not answer is (ADR-0024 §5).
It is never `ErrBoardGone`, which would mark the company `gone`.
Watching a company while Workable is refusing is 503 over HTTP, as a page that did not answer is: the request was right, and can be made again.
A board's answer is kept for an hour (D5), so a search asks Workable once an hour per company at most.
Watching a company reads its board once, and its name comes from that read rather than a second request.

### 6. Neither provider reaches the starter list

The starter list keeps a company with a posting that can be done from anywhere or from more than one continent (spec D2).
With one place per posting, and a description able only to make a reading less certain, no posting on either provider can qualify.
Measured, none of the 13 SmartRecruiters companies did, read by genstarter, nor any of the 182 postings of the 18 Workable accounts that answered, read from their recorded answers by the same code.
genstarter could not read Workable itself: asking four accounts at a time, every one was refused.
So none is added to the seed file, and genstarter, which reads any provider, describes only the postings that qualify before their description is read.

## Consequences

- Two more providers, with no change to the schema: a board's provider is stored as text.
- `BoardReader` gains two methods, and `BoardContents` a field; only the adapter and test doubles implement them.
- A keyword-only search over a large SmartRecruiters company is slow the first time it is run in a session.
- A Workable company can be named as failing when Workable asks JobPilot to wait, and the next search asks again.
- Regional remote roles on either provider are found only by a candidate whose country they list.

## Alternatives considered

**Describing every SmartRecruiters posting, kept by posting.**
The port would not change, but every first read of a large company would be thousands of requests whatever the search.

**No descriptions for SmartRecruiters.**
One request per hundred postings, but keyword searches would never find its postings, and ADR-0023 and ADR-0025 could read nothing from them.

**Reading a title's region over the posting's place.**
Most accurate for Hugging Face's roles, but it would let a title make a reading wider for every source, which ADR-0023 §1 rules out.

**Workable's paginated `v3` jobs endpoint.**
It has a `workplace` field that tells hybrid from on-site, but no descriptions, so a board would take a request per posting as SmartRecruiters' does, against the provider that limits requests most.

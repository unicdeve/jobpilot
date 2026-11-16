// A port of the Go core's search rule (internal/core/domain/job.go, SearchCriteria.MatchesFor), ADR-0037.
// It has no Exact, network or storage dependency, so it runs the same in the app's JS engine and under `bun test`.
// Not ported yet: work-from/relocation (workableFor) and fit-hours (hoursFor), which need the Workable and Seeker types.

export interface Job {
  title: string;
  companyName: string;
  descriptionMarkdown: string;
  locations: string[];
  remoteType: string;
  employmentType: string;
  salaryMin: number;
  salaryMax: number;
}

export interface SearchCriteria {
  keywords: string;
  title: string;
  location: string;
  remotePreference: string;
  employmentType: string;
  salaryMin: number;
  salaryMax: number;
  excludedCompanies: string[];
}

export const emptyCriteria: SearchCriteria = {
  keywords: '', title: '', location: '', remotePreference: '', employmentType: '', salaryMin: 0, salaryMax: 0, excludedCompanies: [],
};

// strings.Fields(strings.ToLower(strings.TrimSpace(field)))
export const searchTerms = (field: string): string[] => field.trim().toLowerCase().split(/\s+/).filter((t) => t !== '');

const matchesAllTerms = (field: string, text: string): boolean => {
  const haystack = text.toLowerCase();
  return searchTerms(field).every((term) => haystack.includes(term));
};

const equalsFold = (criterion: string, value: string): boolean => {
  const c = criterion.trim();
  return c === '' || c.toLowerCase() === value.trim().toLowerCase();
};

// A job with no salary is kept: most boards publish none.
const withinSalary = (c: SearchCriteria, job: Job): boolean => {
  if (job.salaryMin === 0 && job.salaryMax === 0) return true;
  if (c.salaryMin > 0 && Math.max(job.salaryMin, job.salaryMax) < c.salaryMin) return false;
  if (c.salaryMax > 0 && job.salaryMin > c.salaryMax) return false;
  return true;
};

const excludes = (c: SearchCriteria, job: Job): boolean =>
  c.excludedCompanies.some((x) => x.trim().toLowerCase() === job.companyName.trim().toLowerCase());

export function matches(c: SearchCriteria, job: Job): boolean {
  return matchesAllTerms(c.keywords, `${job.title} ${job.descriptionMarkdown}`)
    && matchesAllTerms(c.title, job.title)
    && matchesAllTerms(c.location, job.locations.join(' '))
    && equalsFold(c.remotePreference, job.remoteType)
    && equalsFold(c.employmentType, job.employmentType)
    && withinSalary(c, job)
    && !excludes(c, job);
}

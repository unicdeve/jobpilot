import { expect, test } from 'bun:test';
import { matches, searchTerms, type Job } from './criteria.ts';
import { criteriaIsEmpty, findSearchTemplate, searchTemplateGroups } from './search-template.ts';

// The cases are the Go test's (internal/core/domain/search_template_test.go), so the two cores cannot drift silently.
const job = (title: string, descriptionMarkdown = ''): Job => ({
  title,
  companyName: '',
  descriptionMarkdown,
  locations: [],
  remoteType: '',
  employmentType: '',
  salaryMin: 0,
  salaryMax: 0,
});
const all = () =>
  searchTemplateGroups().flatMap((g) => g.templates.map((t) => ({ group: g, template: t })));
const must = (id: string) => {
  const t = findSearchTemplate(id);
  if (!t) throw new Error(`no template ${id}`);
  return t;
};

test('the catalog is well formed', () => {
  const groups = searchTemplateGroups();
  expect(groups.length).toBeGreaterThan(0);
  const groupIds = new Set<string>();
  const templateIds = new Set<string>();
  for (const g of groups) {
    expect(g.name).not.toBe('');
    expect(g.summary).not.toBe('');
    expect(groupIds.has(g.id)).toBe(false);
    groupIds.add(g.id);
    expect(g.templates.length).toBeGreaterThanOrEqual(2);
    for (const t of g.templates) {
      expect(t.name).not.toBe('');
      expect(t.summary).not.toBe('');
      expect(templateIds.has(t.id)).toBe(false);
      templateIds.add(t.id);
      expect(criteriaIsEmpty(t.criteria)).toBe(false);
    }
  }
  expect(templateIds.size).toBe(34);
});

test('every template can be found by id', () => {
  for (const { template } of all()) expect(findSearchTemplate(template.id)).toEqual(template);
  expect(findSearchTemplate('no-such-template')).toBeUndefined();
});

test('the catalog cannot be changed through what it returns', () => {
  searchTemplateGroups()[0]!.templates[0]!.criteria.title = 'changed';
  expect(must('backend-engineer').criteria.title).toBe('back end');
});

test('no template searches on a tiny keyword', () => {
  for (const { template } of all()) {
    for (const term of searchTerms(template.criteria.keywords))
      expect(term.length).toBeGreaterThanOrEqual(5);
  }
});

const decoys: Job[] = [
  job(
    'Senior Sales Engineer, Enterprise',
    'Demo our platform to enterprise buyers and close the technical evaluation.',
  ),
  job(
    'Senior Contract Recruiter - Go to Market (GTM)',
    'Help us go to market. You will own hiring goals across the go-to-market organisation.',
  ),
  job('Teamleiter (m/w/d) im Finanzvertrieb', 'Sie leiten ein Team im Finanzvertrieb.'),
  job(
    '(Junior) Design Manager (all genders) – Luxury Retail',
    'Support the luxury retail design team with seasonal collections.',
  ),
  job(
    'Chemical Laboratory Assistant / Lab Technician (f/m/d) Analytics & Synthesis',
    'You will run each reaction in the synthesis lab and record the analytics.',
  ),
  job(
    'Senior Account Executive, Enterprise',
    'Build trust with enterprise buyers and hit your goals. Our customers are engineering teams.',
  ),
  job('Quality Dispense Technician North', 'Maintain dispense quality across venues in the north.'),
];

test('no template matches a decoy', () => {
  for (const { template } of all())
    for (const d of decoys)
      expect([template.id, d.title, matches(template.criteria, d)]).toEqual([
        template.id,
        d.title,
        false,
      ]);
});

test('a title template reads the title, not the prose', () => {
  const j = job('Senior Backend Engineer - Cruise', 'Join the cruise booking team.');
  expect(matches(must('backend-engineer').criteria, j)).toBe(true);
  for (const t of searchTemplateGroups().find((g) => g.id === 'design')!.templates)
    expect(matches(t.criteria, j)).toBe(false);
});

test('a stack template also matches a passing mention', () => {
  const sales = job(
    'Senior Sales Engineer, Enterprise',
    'Demo our platform to customers running Kubernetes, and speak credibly about Golang and Terraform.',
  );
  expect(matches(must('go-engineer').criteria, sales)).toBe(true);
  for (const id of ['backend-engineer', 'frontend-engineer', 'fullstack-engineer'])
    expect(matches(must(id).criteria, sales)).toBe(false);
});

const realPostings: Job[] = [
  job('Backend Engineer - Agentic AI (m/w/d)'),
  job('Senior Backend Engineer, Python/Django (f/d/m): Path to CTO', 'Python and Django.'),
  job('Senior Front end Engineer I'),
  job('Frontend Engineer', 'We build with React and TypeScript.'),
  job('Frontend Web Application Developer', 'TypeScript throughout.'),
  job('Full-Stack Engineer (f/m/d)'),
  job('Senior Fullstack Developer (m/w/d)'),
  job('Junior Full Stack Developer (All Genders)', 'React on the front.'),
  job('Principal Software Engineer - Data Lakes'),
  job('Senior Independent Software Developer'),
  job('Platform Engineer', 'We run Kubernetes.'),
  job('Cloud / Platform Engineer'),
  job('Senior/Staff Golang Engineer (EMEA)', 'Golang services.'),
  job('Senior DevOps Engineer', 'Terraform and Kubernetes.'),
  job('Site Reliability Engineer'),
  job('ML Infrastructure Engineer'),
  job('Staff Data Engineer'),
  job('Senior Data Scientist'),
  job('Data Analyst'),
  job('Machine Learning Engineer'),
  job('Senior Product Manager, Orchestration'),
  job('Product owner - Politiques sociales H/F'),
  job('Senior Program Manager, Community Investments'),
  job('Senior Manager, Product Marketing'),
  job('Lead Product Designer'),
  job(
    'Senior Product Designer (M/F/*)',
    'You will run usability sessions and own the work in Figma.',
  ),
  job('QA Engineer'),
  job('Senior Software Development Engineer in Test (SDET)'),
  job('Security Engineer'),
  job('Cybersecurity Principal'),
];

test('every template finds a real posting', () => {
  for (const { template } of all()) {
    expect(realPostings.some((p) => matches(template.criteria, p))).toBe(true);
  }
});

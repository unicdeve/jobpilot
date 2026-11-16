// A port of the Go core's search templates (internal/core/domain/search_template.go), ADR-0037.
// A template is a ready-made saved search; importing one creates an ordinary search, and nothing links the two afterwards.
// Pure data and lookup: no Exact, network or storage dependency.
//
// Three rules govern what may be added here (see docs/02_features/11_search_templates.md):
// put the distinguishing word in title, not the role word; split compound words ("full stack");
// never use a term shorter than four characters and never a common word in keywords,
// because keywords is matched as a substring against the whole description.

import { emptyCriteria, type SearchCriteria } from './criteria.ts';

export interface SearchTemplate {
  // id is stable across releases: it is what a client sends to import a template.
  id: string;
  name: string;
  summary: string;
  criteria: SearchCriteria;
}

export interface SearchTemplateGroup {
  id: string;
  name: string;
  summary: string;
  templates: SearchTemplate[];
}

// SearchCriteria.IsEmpty in Go. The Go type also carries workFrom, which the TS criteria do not have yet.
export const criteriaIsEmpty = (c: SearchCriteria): boolean =>
  c.keywords.trim() === '' && c.title.trim() === '' && c.location.trim() === ''
  && c.remotePreference.trim() === '' && c.employmentType.trim() === ''
  && c.salaryMin === 0 && c.salaryMax === 0 && c.excludedCompanies.length === 0;

const catalog: SearchTemplateGroup[] = [
  {
    id: 'software-engineering',
    name: 'Software engineering',
    summary: 'Engineering roles by specialty. Adopt several: boards title the same job Engineer or Developer, and one search cannot cover both.',
    templates: [
      { id: 'backend-engineer', name: 'Backend Engineer', summary: 'Titles containing Backend or Back End, whether Engineer or Developer.', criteria: { ...emptyCriteria, title: 'back end' } },
      { id: 'frontend-engineer', name: 'Frontend Engineer', summary: 'Titles containing Frontend or Front End.', criteria: { ...emptyCriteria, title: 'front end' } },
      { id: 'fullstack-engineer', name: 'Full-stack Engineer', summary: 'All three spellings: Full-Stack, Full Stack and Fullstack.', criteria: { ...emptyCriteria, title: 'full stack' } },
      { id: 'software-engineer', name: 'Software Engineer', summary: 'The broad one. Use it to see the market, then narrow with a salary floor or a location.', criteria: { ...emptyCriteria, title: 'software engineer' } },
      { id: 'software-developer', name: 'Software Developer', summary: 'The same roles at boards that say Developer. Worth adopting alongside Software Engineer.', criteria: { ...emptyCriteria, title: 'software developer' } },
      { id: 'platform-engineer', name: 'Platform Engineer', summary: 'Internal platform and developer-experience teams.', criteria: { ...emptyCriteria, title: 'platform engineer' } },
    ],
  },
  {
    id: 'engineering-by-stack',
    name: 'Engineering by stack',
    summary: 'For a specific technology. These read the job description, not the title, because boards rarely put a stack in the title — so expect some postings that only mention the technology in passing.',
    templates: [
      { id: 'go-engineer', name: 'Go Engineer', summary: 'Searches for Golang. Plain "Go" is unusable: it also matches go-to, goals and Django.', criteria: { ...emptyCriteria, keywords: 'golang' } },
      { id: 'react-engineer', name: 'React Engineer', summary: 'React in the description, Frontend or Front End in the title.', criteria: { ...emptyCriteria, keywords: 'react', title: 'front end' } },
      { id: 'react-fullstack', name: 'React Full-stack Engineer', summary: 'React in the description, Full-stack in the title.', criteria: { ...emptyCriteria, keywords: 'react', title: 'full stack' } },
      { id: 'python-backend-engineer', name: 'Python Backend Engineer', summary: 'Python in the description, Backend in the title.', criteria: { ...emptyCriteria, keywords: 'python', title: 'back end' } },
      { id: 'typescript-frontend-engineer', name: 'TypeScript Frontend Engineer', summary: 'TypeScript in the description, Frontend in the title.', criteria: { ...emptyCriteria, keywords: 'typescript', title: 'front end' } },
    ],
  },
  {
    id: 'devops-infrastructure',
    name: 'DevOps and infrastructure',
    summary: 'Running the systems rather than building the product. The titles vary more than in any other discipline, so adopt all of these.',
    templates: [
      { id: 'devops-engineer', name: 'DevOps Engineer', summary: 'Titles containing DevOps.', criteria: { ...emptyCriteria, title: 'devops' } },
      { id: 'site-reliability-engineer', name: 'Site Reliability Engineer', summary: 'Spelled out. SRE alone is too short to search on safely.', criteria: { ...emptyCriteria, title: 'site reliability' } },
      { id: 'infrastructure-engineer', name: 'Infrastructure Engineer', summary: 'Titles containing Infrastructure.', criteria: { ...emptyCriteria, title: 'infrastructure' } },
      { id: 'cloud-engineer', name: 'Cloud Engineer', summary: 'Titles containing Cloud and Engineer.', criteria: { ...emptyCriteria, title: 'cloud engineer' } },
      { id: 'terraform-engineer', name: 'Terraform / IaC Engineer', summary: 'Terraform in the description, Engineer in the title. Includes product engineering roles that ask for Terraform.', criteria: { ...emptyCriteria, keywords: 'terraform', title: 'engineer' } },
      { id: 'kubernetes-platform-engineer', name: 'Kubernetes Platform Engineer', summary: 'Kubernetes in the description, Platform in the title.', criteria: { ...emptyCriteria, keywords: 'kubernetes', title: 'platform' } },
    ],
  },
  {
    id: 'data-and-ml',
    name: 'Data and machine learning',
    summary: 'Data work, from pipelines through analysis to models.',
    templates: [
      { id: 'data-engineer', name: 'Data Engineer', summary: 'Pipelines and warehouses.', criteria: { ...emptyCriteria, title: 'data engineer' } },
      { id: 'data-scientist', name: 'Data Scientist', summary: 'Titles containing Data Scientist.', criteria: { ...emptyCriteria, title: 'data scientist' } },
      { id: 'data-analyst', name: 'Data Analyst', summary: 'Titles containing Data Analyst.', criteria: { ...emptyCriteria, title: 'data analyst' } },
      { id: 'machine-learning-engineer', name: 'Machine Learning Engineer', summary: 'Spelled out. ML alone matches the German Teamleiter.', criteria: { ...emptyCriteria, title: 'machine learning' } },
    ],
  },
  {
    id: 'product',
    name: 'Product management',
    summary: 'Owning what gets built.',
    templates: [
      { id: 'product-manager', name: 'Product Manager', summary: 'Also matches Senior and Principal Product Manager.', criteria: { ...emptyCriteria, title: 'product manager' } },
      { id: 'product-owner', name: 'Product Owner', summary: 'The title European boards prefer for the same work.', criteria: { ...emptyCriteria, title: 'product owner' } },
      { id: 'technical-program-manager', name: 'Technical Program Manager', summary: 'Titles containing Program Manager.', criteria: { ...emptyCriteria, title: 'program manager' } },
      { id: 'product-marketing-manager', name: 'Product Marketing Manager', summary: 'Titles containing Product Marketing.', criteria: { ...emptyCriteria, title: 'product marketing' } },
    ],
  },
  {
    id: 'design',
    name: 'Design',
    summary: 'Product and interface design. UX and UI are not searched on directly: two letters match Luxury and Cruise.',
    templates: [
      { id: 'product-designer', name: 'Product Designer', summary: 'Titles containing Product Designer.', criteria: { ...emptyCriteria, title: 'product designer' } },
      { id: 'ux-designer', name: 'UX Designer', summary: 'Designer in the title, usability in the description. Narrower than Product Designer, and more reliably about software.', criteria: { ...emptyCriteria, keywords: 'usability', title: 'designer' } },
      { id: 'design-lead', name: 'Design Lead', summary: 'Titles containing Design and Lead.', criteria: { ...emptyCriteria, title: 'design lead' } },
      { id: 'designer-any', name: 'Designer (any)', summary: 'The broad one. It also finds graphic and fashion design, which share the word.', criteria: { ...emptyCriteria, title: 'designer' } },
      { id: 'figma-designer', name: 'Designer (Figma)', summary: 'Designer in the title, Figma in the description — a reliable sign of product rather than graphic design.', criteria: { ...emptyCriteria, keywords: 'figma', title: 'designer' } },
    ],
  },
  {
    id: 'quality-and-security',
    name: 'Quality and security',
    summary: 'Testing and security engineering.',
    templates: [
      { id: 'qa-engineer', name: 'QA Engineer', summary: 'Titles containing QA.', criteria: { ...emptyCriteria, title: 'qa' } },
      { id: 'test-engineer', name: 'Test Engineer', summary: 'Also matches Software Engineer in Test.', criteria: { ...emptyCriteria, title: 'engineer test' } },
      { id: 'security-engineer', name: 'Security Engineer', summary: 'Also matches Cybersecurity Engineer.', criteria: { ...emptyCriteria, title: 'security engineer' } },
      { id: 'cybersecurity', name: 'Cybersecurity', summary: 'Titles containing Cyber. Broader than Security Engineer, and it also finds sales roles at security companies.', criteria: { ...emptyCriteria, title: 'cyber' } },
    ],
  },
];

// The catalog of ready-made searches. Copies, so a caller cannot change the catalog.
export function searchTemplateGroups(): SearchTemplateGroup[] {
  return catalog.map((g) => ({ ...g, templates: g.templates.map((t) => ({ ...t, criteria: { ...t.criteria, excludedCompanies: [...t.criteria.excludedCompanies] } })) }));
}

// FindSearchTemplate in Go returns an error for an unknown ID; here that is undefined.
export function findSearchTemplate(id: string): SearchTemplate | undefined {
  for (const g of searchTemplateGroups()) {
    const found = g.templates.find((t) => t.id === id);
    if (found) return found;
  }
  return undefined;
}

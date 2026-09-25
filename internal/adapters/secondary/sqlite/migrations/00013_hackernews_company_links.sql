-- +goose Up
-- Hacker News postings stored before the parser reduced header fields to
-- plain text carry a company's homepage inside its name, as markdown:
-- "Freeform ([http://freeform.co](http://freeform.co))". Every case measured
-- is the name, then the link, so the name is what precedes the first '['.
UPDATE jobs
SET company_name = rtrim(trim(substr(company_name, 1, instr(company_name, '[') - 1)), ' (')
WHERE source_id = 'hackernews'
  AND company_name LIKE '%](%'
  AND instr(company_name, '[') > 1;

-- +goose Down
-- The links removed above were never part of the company's name.
SELECT 1;

// How what the model read from a resume (a ProfileDraft) is applied to a
// profile. The Profile screen and first-run setup both prefill from a
// resume, and must agree on what a suggestion may change.

import type { Candidate, CandidateExperience, CandidateLink, CandidateSkill, ProfileDraft } from "./types";

/// applyProfileDraft fills profile from suggestion.
///
/// A blank suggestion field (the model found nothing) never changes the
/// profile. Without overwrite, a suggestion only fills a blank field, so it
/// never clobbers a value the user already typed or saved; with it, every
/// field the model found replaces the profile's.
///
/// Lists are never merged. A list the user has already built is theirs:
/// merging entry by entry would need a notion of which proposed role is
/// "the same" as one they typed, and getting that wrong silently rewrites
/// their history — so a proposed list only fills an empty one.
export function applyProfileDraft(profile: Candidate, suggestion: ProfileDraft, overwrite = false): Candidate {
  const pick = (suggested: string, current: string) => {
    if (!suggested) return current;
    return overwrite ? suggested : current || suggested;
  };
  const pickList = <T>(suggested: T[] | undefined, current: T[] | null) =>
    current?.length ? current : (suggested ?? []);

  return {
    ...profile,
    first_name: pick(suggestion.first_name, profile.first_name),
    last_name: pick(suggestion.last_name, profile.last_name),
    email: pick(suggestion.email, profile.email),
    phone: pick(suggestion.phone, profile.phone),
    location_city: pick(suggestion.location_city, profile.location_city),
    location_region: pick(suggestion.location_region, profile.location_region),
    location_country: pick(suggestion.location_country, profile.location_country),
    summary: pick(suggestion.summary, profile.summary),
    skills: pickList<CandidateSkill>(
      suggestion.skills?.map((skill) => ({
        id: "",
        candidate_id: "",
        name: skill.name,
        years_experience: skill.years_experience ?? null,
      })),
      profile.skills,
    ),
    experiences: pickList<CandidateExperience>(
      suggestion.experiences?.map((experience) => ({
        id: "",
        candidate_id: "",
        company: experience.company,
        title: experience.title,
        start_date: experience.start_date || null,
        end_date: experience.is_current ? null : experience.end_date || null,
        is_current: experience.is_current,
        description: experience.description,
      })),
      profile.experiences,
    ),
    links: pickList<CandidateLink>(
      suggestion.links?.map((link) => ({
        id: "",
        candidate_id: "",
        kind: link.kind,
        url: link.url,
      })),
      profile.links,
    ),
  };
}

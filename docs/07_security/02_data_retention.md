# Data Retention

All application data is local.

Users must be able to:
- delete individual jobs
- delete applications
- delete generated documents
- delete resumes
- clear history
- delete all application data

Deletion must account for derived data where applicable.

A future cleanup tool may identify large data categories and help users reclaim disk space.

Implemented as `docs/02_features/15_data_deletion.md`, built as ADR-0021 describes: what each deletion takes with it, how it is confirmed, and how deleted content is made unreadable on disk.

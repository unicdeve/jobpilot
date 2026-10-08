# Feature: Local workspaces

## Purpose

Let one JobPilot installation hold several completely separate job-search profiles and switch between them without a hosted account.

The desktop calls these profiles because that is the user-facing idea.
The core calls them workspaces so they are not confused with the candidate profile stored inside each one.

## Behaviour

- A new installation starts with one workspace.
- An existing installation adopts its current database and browser data as its first workspace without copying or moving them.
- A user can create, rename and switch workspaces.
- Each workspace has its own candidate profile, resumes, settings, saved searches, watched companies, jobs, Scouts, notifications, applications, activity, credentials and browser sign-ins.
- Appearance and the selected workspace are preferences of the installation rather than one workspace.
- Scheduled Scouts retain their workspace identity and can run while another workspace is visible.
- A workspace is local organization, not an application account or an access-control boundary.
- The CLI can select a workspace for one invocation without changing the desktop's active workspace.

## Desktop

The bottom sidebar control shows the active workspace's name and initials.
Its menu lists the other workspaces and offers Add profile, Rename profile and Settings.
Settings remains available through Command/Ctrl-comma.

Switching workspaces clears server-state caches and remounts every workspace-owned screen.
Drafts, selections and scroll positions from one workspace must never appear in another.

The existing “delete all data” operation resets only the selected profile.
It removes that profile's database contents, credentials, browser sign-ins and Scout alarms without removing another profile or the installation-global catalog, theme or daemon log.

## Acceptance criteria

### AC-001
An existing installation appears as one workspace with all existing data intact.

### AC-002
Creating a workspace produces an empty onboarding state and does not expose data from another workspace.

### AC-003
Settings, saved searches, watched companies, jobs, Scouts, notifications, applications, activity, credentials and browser sessions are isolated between workspaces.

### AC-004
Switching workspaces changes every workspace-owned desktop view together and preserves the device theme.

### AC-005
A scheduled Scout runs against the workspace that owns it, even when another workspace is active.

### AC-006
The same credential source and label can exist independently in two workspaces without either secret being readable through the API.

### AC-007
The CLI can list workspaces and scope a command to one by identifier without changing the installation's active workspace.

### AC-008
Workspace identifiers accepted from a client cannot escape the application data directory or select an unregistered workspace.

### AC-009
Profile switching requires no hosted account and continues to work offline.

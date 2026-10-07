# Browser Profiles

JobPilot uses a dedicated persistent Chrome profile owned by JobPilot.
It never opens the user's daily Chrome profile.
The user logs into a site inside this profile when authentication is required; there is no guided first-use login flow yet.

Website passwords are not stored in JobPilot's database.
Chrome manages the profile's session data.
The shared browser session prevents concurrent tasks from launching competing Chrome processes against the same profile.
Deleting all data closes Chrome and removes this profile (ADR-0021).

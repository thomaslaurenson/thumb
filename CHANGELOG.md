# Changelog

## 0.1.0 - 2026-10-28

### Added

- Add the thumb command for writing, marking and migrating daily plan entries
- Add a terminal interface with vim keys, a day picker for migrating, undo and rollover of open items, saving on quit or when the terminal hangs up
- Add a project list shown on the site and published as plain text
- Add a static site build with archive, markers page, Atom feed, and curl-able .plan and .project
- Add a serve command that previews the site on localhost and reloads open pages on each save
- Hold back entries dated after today, with a timezone setting for CI builds
- Add a sync command that commits, pulls and pushes the entries

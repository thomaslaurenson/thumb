# Changelog

## 0.1.1 - 2026-10-28

### Changed

- Quote dates, paths and file names in error messages

### Fixed

- Report a failed save when the interface is interrupted

## 0.1.0 - 2026-10-28

### Added

- Add the thumb command for writing, marking and migrating daily plan entries
- Add a terminal interface with vim keys, undo and rollover of open items
- Add a project list shown on the site and published as plain text
- Add a static site build with archive, markers page, Atom feed and curl-able .plan
- Add a serve command that previews the site and reloads open pages on save
- Hold back entries dated after today, with a timezone setting for CI builds
- Add a sync command that commits, pulls and pushes the entries

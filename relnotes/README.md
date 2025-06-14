# Release Notes

Files in this directory constitute manual release notes for a given release.
When relevant, they should be created prior to that release so that they can
be included in the corresponding tag message, etc.

To add release notes for a release 1.2.3, create a file named `v1.2.3.md`
consisting of an initial H2-level header and further notes as desired. For
example:

```
## Major changes in v1.2.3

- Files are now synchronized twice as fast on Tuesdays
```

The release notes will also be included in candidate releases (e.g.
v1.2.3-rc.1).

Additional notes will also be loaded from `v1.2.md` and `v1.md`, if they
exist.

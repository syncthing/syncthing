## Reporting Bugs

Please file bugs in the [GitHub Issue
Tracker](https://github.com/syncthing/syncthing/issues). Include at
least the following:

 - What happened

 - What did you expect to happen instead of what *did* happen, if it's
   not crazy obvious

 - What operating system, operating system version and version of
   Syncthing you are running

 - The same for other connected devices, where relevant

 - Screenshot if the issue concerns something visible in the GUI

 - Console log entries, where possible and relevant

If you're not sure whether something is relevant, erring on the side of
too much information will never get you yelled at. :)

## Contributing Translations

All translations are done via
[Weblate](https://hosted.weblate.org/projects/syncthing/). If you wish
to contribute to a translation, just head over there and sign up.
Before every release, the language resources are updated from the
latest info on Weblate.

Note that the previously used service at
[Transifex](https://www.transifex.com/projects/p/syncthing/) is being
retired and we kindly ask you to sign up on Weblate for continued
involvement.

## Contributing Documentation

Updates to the [documentation site](https://docs.syncthing.net/) can be
made as pull requests on the [documentation
repository](https://github.com/syncthing/docs).

## Contributing Code

Every contribution is welcome. If you want to contribute but are unsure
where to start, any open issues are fair game! Here's a short rundown of
what you need to keep in mind:

- Don't worry. You are not expected to get everything right on the first
  attempt, we'll guide you through it.

- Make sure there is an
  [issue](https://github.com/syncthing/syncthing/issues) that describes the
  change you want to do. If the thing you want to do does not have an issue
  yet, please file one before starting work on it.

- Fork the repository and make your changes in a new branch. Once it's ready
  for review, create a pull request.

### Authorship

All code authors are listed in the AUTHORS file. When your first pull
request is accepted your details are added to the AUTHORS file and the list
of authors in the GUI. Commits must be made with the same name and email as
listed in the AUTHORS file. To accomplish this, ensure that your git
configuration is set correctly prior to making your first commit:

    $ git config --global user.name "Jane Doe"
    $ git config --global user.email janedoe@example.com

You must be reachable on the given email address. If you do not wish to use
your real name for whatever reason, using a nickname or pseudonym is
perfectly acceptable.

### The Developer Certificate of Origin (DCO)

The Syncthing project requires the Developer Certificate of Origin (DCO)
sign-off on pull requests (PRs). This means that all commit messages must
contain a signature line to indicate that the developer accepts the DCO.

The DCO is a lightweight way for contributors to certify that they wrote (or
otherwise have the right to submit) the code and changes they are
contributing to the project. Here is the full [text of the
DCO](https://developercertificate.org):

---

By making a contribution to this project, I certify that:

1. The contribution was created in whole or in part by me and I have the
   right to submit it under the open source license indicated in the file;
   or

2. The contribution is based upon previous work that, to the best of my
   knowledge, is covered under an appropriate open source license and I have
   the right under that license to submit that work with modifications,
   whether created in whole or in part by me, under the same open source
   license (unless I am permitted to submit under a different license), as
   indicated in the file; or

3. The contribution was provided directly to me by some other person who
   certified (1), (2) or (3) and I have not modified it.

4. I understand and agree that this project and the contribution are public
   and that a record of the contribution (including all personal information
   I submit with it, including my sign-off) is maintained indefinitely and
   may be redistributed consistent with this project or the open source
   license(s) involved.

---

Contributors indicate that they adhere to these requirements by adding
a `Signed-off-by` line to their commit messages.  For example:

    This is my commit message

    Signed-off-by: Random J Developer <random@developer.example.org>

The name and email address in this line must match those of the committing
author, and be the same as what you want in the AUTHORS file as per above.

### Coding Style

#### General

- All text files use Unix line endings. The git settings already present in
  the repository attempt to enforce this.

- When making changes, follow the brace and parenthesis style of the
  surrounding code.

#### Go Specific

- Follow the conventions laid out in [Effective
  Go](https://go.dev/doc/effective_go) as much as makes sense. The review
  guidelines in [Go Code Review
  Comments](https://github.com/golang/go/wiki/CodeReviewComments) should
  generally be followed.

- Each commit should be `go fmt` clean.

- Imports are grouped per `goimports` standard; that is, standard
  library first, then third party libraries after a blank line.

### Commits

- Commit messages (and pull request titles) should follow the [conventional
  commits](https://www.conventionalcommits.org/en/v1.0.0/) specification and
  be in lower case.

- We use a scope description in the commit message subject. This is the
  component of Syncthing that the commit affects. For example, `gui`,
  `protocol`, `scanner`, `upnp`, etc -- typically, the part after
  `internal/`, `lib/` or `cmd/` in the package path. If the commit doesn't
  affect a specific component, such as for changes to the build system or
  documentation, the scope should be omitted. The same goes for changes that
  affect many components which would be cumbersome to list.

- Commits that resolve an existing issue must include the issue number
  as `(fixes #123)` at the end of the commit message subject. A correctly
  formatted commit message subject looks like this:

      feat(dialer): add env var to disable proxy fallback (fixes #3006)

- If the commit message subject doesn't say it all, one or more paragraphs of
  describing text should be added to the commit message. This should explain
  why the change is made and what it accomplishes.

- When drafting a pull request, please feel free to add commits with
  corrections and merge from `main` when necessary. This provides a clear time
  line with changes and simplifies review. Do not, in general, rebase your
  commits, as this makes review harder.

- Pull requests are merged to `main` using squash merge. The "stream of
  consciousness" set of commits described in the previous point will be reduced
  to a single commit at merge time. The pull request title and description will
  be used as the commit message.

### Tests

Yes please, do add tests when adding features or fixing bugs. Also, when a
pull request is filed a number of automatic tests are run on the code. This
includes:

- That the code actually builds and the test suite passes.

- That the code is correctly formatted (`go fmt`).

- That the commits are based on a reasonably recent `main`.

- That the output from `go lint` and `go vet` is clean. (This checks for a
  number of potential problems the compiler doesn't catch.)

## Licensing

All contributions are made available under the same license as the already
existing material being contributed to. For most of the project and unless
otherwise stated this means MPLv2, but there are exceptions:

- Certain commands (under cmd/...) may have a separate license, indicated by
  the presence of a LICENSE file in the corresponding directory.

- The documentation (man/...) is licensed under the Creative Commons
  Attribution 4.0 International License.

Regardless of the license in effect, you retain the copyright to your
contribution.


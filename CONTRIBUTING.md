## Reporting Bugs

Please file bugs in the [Github Issue
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
[Transifex](https://www.transifex.com/projects/p/syncthing/). If you
wish to contribute to a translation, just head over there and sign up.
Before every release, the language resources are updated from the
latest info on Transifex.

## Contributing Code

Every contribution is welcome. If you want to contribute but are unsure
where to start, any open issues are fair game! Be prepared for a
[certain amount of review](https://github.com/syncthing/syncthing/wiki/FAQ#why-are-you-being-so-hard-on-my-pull-request);
it's all in the name of quality. :) Following the points below will make this
a smoother process.

Individuals making significant and valuable contributions are given
commit-access to the project. If you make a significant contribution and
are not considered for commit-access, please contact any of the
Syncthing core team members.

All nontrivial contributions should go through the pull request
mechanism for internal review. Determining what is "nontrivial" is left
at the discretion of the contributor.

### Authorship

All code authors are listed in the AUTHORS file. Commits must be made
with the same name and email as listed in the AUTHORS file. To
accomplish this, ensure that your git configuration is set correctly
prior to making your first commit;

    $ git config --global user.name "Jane Doe"
    $ git config --global user.email janedoe@example.com

You must be reachable on the given email address. If you do not wish to
use your real name for whatever reason, using a nickname or pseudonym is
perfectly acceptable.

### Core Team

The Syncthing core team currently consists of the following members;

 - Jakob Borg (@calmh)
 - Audrius Butkevicius (@AudriusButkevicius)

## Coding Style

- Follow the conventions laid out in [Effective Go](https://golang.org/doc/effective_go.html)
  as much as makes sense.

- All text files use Unix line endings.

- Each commit should be `go fmt` clean.

- The commit message subject should be a single short sentence
  describing the change, starting with a capital letter.

- Commits that resolve an existing issue must include the issue number
  as `(fixes #123)` at the end of the commit message subject.

- Imports are grouped per `goimports` standard; that is, standard
  library first, then third party libraries after a blank line.

- A contribution solving a single issue or introducing a single new
  feature should probably be a single commit based on the current
  `master` branch. You may be asked to "rebase" or "squash" your pull
  request to make sure this is the case, especially if there have been
  amendments during review.

## Licensing

All contributions are made under the same GPL license as the rest of the
project, except documentation, user interface text and translation
strings which are licensed under the Creative Commons Attribution 4.0
International License. You retain the copyright to code you have
written.

When accepting your first contribution, the maintainer of the project
will ensure that you are added to the AUTHORS file. You are welcome to
add yourself as a separate commit in your first pull request.

## Building

[See the documentation](https://github.com/syncthing/syncthing/wiki/Building)
on how to get started with a build environment.

## Branches

 - `master` is the main branch containing good code that will end up in
   the next release. You should base your work on it. It won't ever be
   rebased or force-pushed to.

 - `vx.y` branches exist to make patch releases on otherwise obsolete
   minor releases. Should only contain fixes cherry picked from master.
   Don't base any work on them.

 - Other branches are probably topic branches and may be subject to
   rebasing. Don't base any work on them unless you specifically know
   otherwise.

## Tags

All releases are tagged semver style as `vx.y.z`. Release tags are
signed by GPG key BCE524C7.

## Tests

Yes please!

## Documentation

[Over here!](https://github.com/syncthing/syncthing/wiki)

## License

GPLv3

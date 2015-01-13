## Reporting Bugs

Please file bugs in the [Github Issue
Tracker](https://github.com/syncthing/protocol/issues).

## Contributing Code

Every contribution is welcome. Following the points below will make this
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

All contributions are made under the same MIT license as the rest of the
project, except documentation, user interface text and translation
strings which are licensed under the Creative Commons Attribution 4.0
International License. You retain the copyright to code you have
written.

When accepting your first contribution, the maintainer of the project
will ensure that you are added to the AUTHORS file. You are welcome to
add yourself as a separate commit in your first pull request.

## Tests

Yes please!

## License

MIT

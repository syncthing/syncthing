## Reporting Bugs

Please file bugs in the [Github Issue
Tracker](https://github.com/syncthing/syncthing/issues). Include at
least the following:

 - What happened

 - What did you expect to happen instead of what *did* happen, if it's
   not crazy obvious

 - What operating system, operating system version and version of
   Syncthing you are running

 - The same for other connected nodes, where relevant

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

Please do contribute! If you want to contribute but are unsure where to
start, the [Contributions Needed
topic](http://discourse.syncthing.net/t/49) lists areas in need of
attention. In general, any open issues are fair game!

## Licensing

All contributions are made under the same MIT License as the rest of the
project, except documentation which is licensed under the Creative
Commons Attribution 4.0 International License. You retain the copyright
to code you have written.

When accepting your first contribution, the maintainer of the project
will ensure that you are added to the CONTRIBUTORS file. You are welcome
to add yourself as a separate commit in your first pull request.

## Building

[See the documentation](http://discourse.syncthing.net/t/44) on how to
get started with a build environment.

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

## Style

 - `go fmt`

 - Unix line breaks

## Documentation

[Over here!](http://discourse.syncthing.net/category/documentation)

## License

MIT

### DO NOT REPORT SECURITY ISSUES IN THIS ISSUE TRACKER

Instead, contact security@syncthing.net directly - see
https://syncthing.net/security.html for more information.

### DO NOT POST SUPPORT REQUESTS OR GENERAL QUESTIONS IN THIS ISSUE TRACKER

Please use the forum at https://forum.syncthing.net/ where a large number of
helpful people hang out. This issue tracker is for reporting bugs or feature
requests directly to the developers. Worst case you might get a short
"that's a bug, please report it on GitHub" response on the forum, in which
case we thank you for your patience and following our advice. :)

### Please use the correct issue tracker

If your problem relates to a Syncthing wrapper or [sub-project](https://github.com/syncthing) such as [Syncthing for Android](https://github.com/syncthing/syncthing-android/issues), [SyncTrayzor](https://github.com/canton7/synctrayzor) or the [documentation](https://github.com/syncthing/docs/issues), please use their respective issue trackers.

### Does your log mention database corruption?

If your Syncthing log reports panics because of database corruption it is most likely a fault with your system's storage or memory. Affected log entries will contain lines starting with `panic: leveldb`. You will need to delete the index database to clear this, by running `syncthing -reset-database`.

### Please do post actual bug reports and feature requests.

If your issue is a bug report, replace this boilerplate with a description
of the problem, being sure to include at least:

 - what happened,
 - what you expected to happen instead, and
 - any steps to reproduce the problem.

Also fill out the version information below and add log output or
screenshots as appropriate.

If your issue is a feature request, simply replace this template text in
its entirety.

### Version Information

Syncthing Version: v0.x.y
OS Version: Windows 7 / Ubuntu 14.04 / ...
Browser Version: (if applicable, for GUI issues)


Exit Codes
==========

These are the known exit codes returned by Syncthing:

==== =======
Code Meaning
==== =======
0    Success / Shutdown
1    Error
2    Upgrade not available
3    Restarting
5    Upgrading
==== =======

Some of these exit codes are only returned when running without a
monitor process (with environment variable ``STNORESTART`` set).

Exit codes over 125 are usually returned by the shell/binary
loader/default signal handler.

Exit codes over 128+N on Unix usually represent the signal which caused
the process to exit. For example, ``128 + 9 (SIGKILL) = 137``.

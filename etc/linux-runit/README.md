This directory contains a configuration for running syncthing under the
"runit" service manager on Linux. It probably works perfectly fine on
other platforms also using runit.

 1. Install runit.

 2. Edit the `run` file to set the username to run as, the user's home
    directory and the place where the syncthing binary lives. It is
    recommended to place it in a directory writeable by the running user
    so that automatic upgrades work.

 3. Copy this directory (containing the edited `run` file and `log` folder) to
    `/etc/service/syncthing`.

Log output is sent to syslogd.


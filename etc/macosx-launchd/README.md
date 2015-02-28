This directory contains an example for running Syncthing in the
background under Mac OS X.

 1. Install the `syncthing` binary in a directory called `bin` in your
    home directory.

 2. Edit the `syncthing.plist` file in the two places that refer to your
    home directory; that is, replace `/Users/jb` with your actual home
    directory location.

 3. Copy the `syncthing.plist` file to `~/Library/LaunchAgents`.

 4. Log out and in again, or run `launchctl load
    ~/Library/LaunchAgents/syncthing.plist`.

You probably want to turn off "Start Browser" among the settings to
avoid it opening a browser window on each login.

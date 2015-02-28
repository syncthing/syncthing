This directory contains a configuration for running syncthing under the
"systemd" service manager on Linux both under either a systemd system service or
systemd user service.

 1. Install systemd.

 2. If you are running this as a system level service:

   1. Create the user you will be running the service as (foo in this example).

   2. Copy the syncthing@.service files to /etc/systemd/system

   3. Enable and start the service
      systemctl enable syncthing@foo.service
      systemctl start syncthing@foo.service

 3. If you are running this as a user level service:

   1. Log in as the user you will be running the service as

   2. Copy the syncthing.service files to /etc/systemd/user

   3. Enable and start the service
      systemctl --user enable syncthing.service
      systemctl --user start syncthing.service

Log output is sent to the journal.

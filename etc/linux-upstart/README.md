# Upstart Configuration

This directory contains example configuration files for running Syncthing under
the "Upstart" service manager on Linux. To have syncthing start when you login
place "user/syncthing.conf" in the "/home/[username]/.config/upstart/" folder.
To have syncthing start when the system boots place "system/syncthing.conf"
in the "/etc/init/" folder.
To manually start syncthing via Upstart when using the system configuration use:

```
    sudo initctl start syncthing
```

For further documentation see [https://docs.syncthing.net/users/autostart.html][1].

[1]: https://docs.syncthing.net/users/autostart.html#Upstart

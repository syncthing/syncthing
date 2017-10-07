This directory contains an example for running Syncthing with a `rc.d` script in FreeBSD.

* Install `syncthing` in `/usr/local/bin/syncthing`.
* Copy the `syncthing` rc.d script in `/usr/local/etc/rc.d/syncthing`.
* To automatically start `syncthing` at boot time, add the following line to `/etc/rc.conf`:
```
syncthing_enable=YES
```
* Optional configuration options are:
```
syncthing_home=</path/to/syncthing/config/dir>
syncthing_log_file=</path/to/syncthing/log/file>
syncthing_user=<syncthing_user>
syncthing_group=<syncthing_group>
```
See the rc.d script for more informations.
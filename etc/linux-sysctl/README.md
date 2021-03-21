sysctl configuration to raise UDP buffer size
===================
Installation
-----------
**Please note:** When you installed syncthing using the official deb package, you can skip the copying.

Copy the file `30-syncthing.conf` to `/etc/sysctl.d/` (root permissions required).

In a terminal run
```
sudo sysctl -q --system
```
to apply the sysctl changes.


Verification
----------
You can verify that the new limit is active using
```
sysctl net.core.rmem_max
```

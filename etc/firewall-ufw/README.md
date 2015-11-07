Uncomplicated FireWall application preset
===================
Installation
-----------
**Please note:** When you installed syncthing using the official deb package, you can skip the copying.

Copy the file `syncthing` to your ufw applications directory usually located at `/etc/ufw/applications.d/`. (root permissions required).

Then run
```
sudo ufw app update syncthing
```
to load the preset.
To allow the syncthing ports, run:
```
sudo ufw allow syncthing
```
You can also verify the opened ports:
```
sudo ufw status verbose
```

Uncomplicated FireWall application preset
===================
Installation
-----------
**Please note:** When you installed syncthing using the official deb package, you can skip the copying.

Copy the file `syncthing` to your ufw applications directory usually located at `/etc/ufw/applications.d/` (root permissions required).

In a terminal run
```
sudo ufw app update syncthing
sudo ufw app update syncthing-gui
```
to load the presets.
To allow the syncthing ports, run
```
sudo ufw allow syncthing
```
If you want to access the web gui from anywhere (not only from localhost), you can also allow the gui port.
This is step is **not** necessary for a "normal" installation!
```
sudo ufw allow syncthing-gui
```


Verification
----------
You can verify the opened ports by running
```
sudo ufw status verbose
```

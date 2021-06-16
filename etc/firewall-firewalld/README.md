Firewalld services
==================
Installation
------------
Copy the files`syncthing.xml` and `syncthing-gui.xml`  to your firewalld services directory usually located at `/etc/firewalld/services/` (root permissions required).

To allow the syncthing ports, run
```
sudo firewall-cmd --add-service=syncthing
sudo firewall-cmd --add-service=syncthing --permanent
```
If you want to access the web gui from anywhere (not only from localhost), you can also allow the gui port.
This is step is **not** necessary for a "normal" installation!
```
sudo firewall-cmd --add-service=syncthing-gui
sudo firewall-cmd --add-service=syncthing-gui --permanent
```


Verification
------------
You can verify the enabled services by running
```
sudo firewall-cmd --list-all
```

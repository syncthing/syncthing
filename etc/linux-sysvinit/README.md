This directory contains a configuration for automatically starting
syncthing SysVinit based distributions.

The script is compatible with LSB distributions providing start-stop-daemon, 
as well as distributions providing the /etc/init.d/functions daemon functions. 

Instructions:

* copy the syncthing init script in this directory to `/etc/init.d/syncthing`
* ensure /etc/init.d/syncthing is executable (`chmod +x /etc/init.d/syncthing`)
* edit /etc/init.d/syncthing, ensure the following variables are set appropriately
  * **DAEMON** : path to the syncthing binary, use `which syncthing` to determine
  * **RUN_AS** : username to run syncthing as
  * **LOGFILE** : filename that captures output from the syncthing process
* start syncthing manually (`/etc/init.d/syncthing`) and confirm working 
* register the syncthing script to execute on boot (follow your distributions mechanism)
  * **Debian/Ubuntu** : `update-rc.d syncthing defaults`
  * **RedHat/CentOS** : `chkconfig syncthing on`
  

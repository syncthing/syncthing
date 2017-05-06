This is short guide on how to get Syncthing running as Service on Windows. That way you won't need to be logged in for Syncthing to run (great if you need to configure Syncthing on server machines)

 1. Download latest version of NSSM - the Non-Sucking Service Manager: http://nssm.cc/download

 2. Once downloaded and unzipped, open console, go to directory win32 or win64 directory (depending on your Windows OS) and execute command:

	`nssm install Syncthing`

 3. Dialog will pop-up allowing you to configure properties of new service:

 	- Application tab:

 		- Path: Full path to syncthing.exe
 		- Startup directory: Directory containing syncthing.exe
 		- Argument: `-no-console -no-browser -home="c:\users\<REPLACE_WITH_YOUR_USERNAME>\AppData\Local\Syncthing"`

 	- I/O tab:

 		- Output: optionally provide path to file that will act as log for console output. Great for debugging problems.

 4. Click Install service. By default Service will Autostart when machine is restarted. But this first time you'll need to go to `Control Panel -> Administrative Tools -> Services` and run newly created Syncthing service manually.

 5. If you provided output file, look at the log... everything should be running as expected. There should also find that line `Access the GUI via the following URL: http://127.0.0.1:8384/` in case you need to access GUI.

Credits:

 - https://blog.dummzeuch.de/2015/04/09/running-syncthing-as-a-windows-service/
 - http://stackoverflow.com/a/15719678/237858
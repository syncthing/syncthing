A first pass at an install script for SyncThing as a Windows Service

Script uses NSIS http://nsis.sourceforge.net/Download
Requires the NSIS Simple Service Plugin http://nsis.sourceforge.net/NSIS_Simple_Service_Plugin
And uses the Windows Service Wrapper https://github.com/kohsuke/winsw

To build the setup file:

 1. Install NSIS, download the Simple Service Plugin DLL into the NSIS plugin folder
 2. Create a folder (referenced by the $SOURCEPATH variable in the .nsi file) with all the syncthing output in it (exe file, licences, etc)
 3. Download winsw.exe from https://github.com/kohsuke/winsw/releases, rename it to syncthingservice.exe and save that in $SOURCEPATH
 4. Put syncthingservice.xml in there too
 5. Compile SyncthingSetup.nsi using NSIS

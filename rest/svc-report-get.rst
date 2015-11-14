GET /rest/svc/report
====================

Returns the data sent in the anonymous usage report.

.. code-block:: json

	{
	   "folderMaxMiB" : 0,
	   "platform" : "linux-amd64",
	   "totMiB" : 0,
	   "longVersion" : "syncthing v0.12.2 \"Beryllium Bedbug\" (go1.4.3 linux-amd64 default) unknown-user@build2.syncthing.net 2015-11-09 13:23:26 UTC",
	   "upgradeAllowedManual" : true,
	   "totFiles" : 3,
	   "folderUses" : {
	      "ignorePerms" : 0,
	      "autoNormalize" : 0,
	      "readonly" : 0,
	      "ignoreDelete" : 0
	   },
	   "memoryUsageMiB" : 13,
	   "version" : "v0.12.2",
	   "sha256Perf" : 27.28,
	   "numFolders" : 2,
	   "memorySize" : 1992,
	   "announce" : {
	      "defaultServersIP" : 0,
	      "otherServers" : 0,
	      "globalEnabled" : false,
	      "defaultServersDNS" : 1,
	      "localEnabled" : false
	   },
	   "usesRateLimit" : false,
	   "numCPU" : 2,
	   "uniqueID" : "",
	   "urVersion" : 2,
	   "rescanIntvs" : [
	      60,
	      60
	   ],
	   "numDevices" : 2,
	   "folderMaxFiles" : 3,
	   "relays" : {
	      "defaultServers" : 1,
	      "enabled" : true,
	      "otherServers" : 0
	   },
	   "deviceUses" : {
	      "compressMetadata" : 1,
	      "customCertName" : 0,
	      "staticAddr" : 1,
	      "compressAlways" : 0,
	      "compressNever" : 1,
	      "introducer" : 0,
	      "dynamicAddr" : 1
	   },
	   "upgradeAllowedAuto" : false
	}

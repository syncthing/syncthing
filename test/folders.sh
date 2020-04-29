#!/bin/bash

for ((id=0;id<200;id++)); do
cat <<EOT
    <folder id="$id" label="" path="$id?maxsize=1000&amp;seed=$id" type="sendreceive" rescanIntervalS="3600" fsWatcherEnabled="false" fsWatcherDelayS="10" ignorePerms="false" autoNormalize="true">
        <filesystemType>fake</filesystemType>
        <device id="I6KAH76-66SLLLB-5PFXSOA-UFJCDZC-YAOMLEK-CP2GB32-BV5RQST-3PSROAU" introducedBy=""></device>
        <device id="MRIW7OK-NETT3M4-N6SBWME-N25O76W-YJKVXPH-FUMQJ3S-P57B74J-GBITBAC" introducedBy=""></device>
    </folder>
EOT
done
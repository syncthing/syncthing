# AWS Warehouse (S3) for Syncthing

Issue "Object Store (S3) backend" (#8113) suggests to integrate S3 cloud storage into syncthing as a storage backup for syncthing folders.

## Requirements

### Priority of Requirements

As for an agile software development the goal is to implement low hanging fruits first,
the requirements should be prioritized.
This allows to filter out less important features that would increase implementation complexity in an unreasonable way.

### List of Requirements

| ID 	| Short Text							| Priority 	| Description	|
|---	|---									|---		|---			|
| 1  	| S3 cheap storage 						| Must  	| Indirect requirement. Supporting S3 based object storage for folder data makes hosting Synchting in the cloud cheaper. Block devices are more costly than S3 buckets.	|
| 2 	| S3 relyablility by redundancy			| Must  	| Indirect requirement. Supporting S3 based object storage for folder data allows to store data in a more relyable way. |
| 3 	| S3 fast distribution					| Must  	| Indirect requirement. Supporting S3 based object storage for folder data allows to increase the bandwith when accessing the data. |
| 4 	| untrusted node feature				| Must  	| The S3 integration into Syncthing needs to support the existing encrypted receive / untrusted node feature. |
| 5 	| multiple nodes w/o data duplication 	| High  	| The S3 integration into Syncthing needs to support parallel access to the S3 storage by multiple syncthing nodes / instances. |
| 6 	| r/w performance 						| Must  	| The S3 integration into Syncthing needs to have a proper performance when reading and writing. |
| 7 	| complete data and metadata			| Must  	| The S3 integration into Syncthing needs to store data and metadata to the object store as the index-DB is stored in a container and might easily get deleted when migrating the nodes. |
| 8 	| objects are files						| Middle  	| For beeing able to host websites based on a Syncthing folder, the S3 integration needs to store each file in a single object. |
| 9 	| small objects are files				| Middle  	| A compromize for the performance impact of managing large files in S3, one could consider to only store small files as single object. |
| 9 	| file access on server by FUSE-mount	| Middle  	| As a workaround for not storing files as objects, synchting could offer FUSE based mounting of the folder data such that via this FUSE interface the real file data is accessible. |

## Solutions

Apart from the implementation of the feature directly into Syncthing, it is possible to achieve some of the requirements also just by combining existing external tools with Synching.
Therefor we cosider both ways seperately.

### Existing External Tools

| Short Text																	| Limitations 	| Description	|
|---																			|---				|---			|
| s3fs: mount S3 bucket as FUSE filesystem and place Syncthing folder into it. 		| Slow listing of metadata. Issues with parallel access of nodes. Modification of files requires re-upload which makes modification of large files very slow.  				| s3fs is a FUSE based filesystem that maps a S3 bucket. Objects are files. It does caching of the data, but listing of directory content is slow as directory content seems not to be cached. |
| s3backer: map S3 as block device and create std-filesystem in it.	| No parallel node acccess. Files are NOT objects. | s3backer stores the used blocks of a block device. The block device can then be used to create a std filesystem like ext4 or NTFS in it.

### Integration into Syncthing

An integration into Syncthing needs to provice more functionality or much better performance compared to any of the solutions mentioned in Exsiting External Tools.
Otherwise there is no reason for the integration.

TODO

## Summary

TODO


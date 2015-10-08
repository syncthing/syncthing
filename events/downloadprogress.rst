DownloadProgress
----------------

Emitted during file downloads for each folder for each file. By default
only a single file in a folder is handled at the same time, but custom
configuration can cause multiple files to be shown.

.. code-block:: json

    {
        "id": 221,
        "type": "DownloadProgress",
        "time": "2014-12-13T00:26:12.9876937Z",
        "data": {
            "folder1": {
                "file1": {
                    "Total": 800,
                    "Pulling": 2,
                    "CopiedFromOrigin": 0,
                    "Reused": 633,
                    "CopiedFromElsewhere": 0,
                    "Pulled": 38,
                    "BytesTotal": 104792064,
                    "BytesDone": 87883776
                },
                "dir\\file2": {
                    "Total": 80,
                    "Pulling": 2,
                    "CopiedFromOrigin": 0,
                    "Reused": 0,
                    "CopiedFromElsewhere": 0,
                    "Pulled": 32,
                    "BytesTotal": 10420224,
                    "BytesDone": 4128768
                }
            },
            "folder2": {
                "file3": {
                    "Total": 800,
                    "Pulling": 2,
                    "CopiedFromOrigin": 0,
                    "Reused": 633,
                    "CopiedFromElsewhere": 0,
                    "Pulled": 38,
                    "BytesTotal": 104792064,
                    "BytesDone": 87883776
                },
                "dir\\file4": {
                    "Total": 80,
                    "Pulling": 2,
                    "CopiedFromOrigin": 0,
                    "Reused": 0,
                    "CopiedFromElsewhere": 0,
                    "Pulled": 32,
                    "BytesTotal": 10420224,
                    "BytesDone": 4128768
                }
            }
        }
    }

-  ``Total`` - total number of blocks in the file
-  ``Pulling`` - number of blocks currently being downloaded
-  ``CopiedFromOrigin`` - number of blocks copied from the file we are
   about to replace
-  ``Reused`` - number of blocks reused from a previous temporary file
-  ``CopiedFromElsewhere`` - number of blocks copied from other files or
   potentially other folders
-  ``Pulled`` - number of blocks actually downloaded so far
-  ``BytesTotal`` - approximate total file size
-  ``BytesDone`` - approximate number of bytes already handled (already
   reused, copied or pulled)

Where block size is 128KB.

Files/folders appearing in the event data imply that the download has
been started for that file/folder, where disappearing implies that the
downloads have been finished or failed for that file/folder. There is
always a last event emitted with no data, which implies all downloads
have finished/failed.

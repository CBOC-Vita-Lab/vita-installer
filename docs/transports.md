# File Transports

The backend exposes rooted file operations, not shell or arbitrary FTP commands.
Writes require a job scope. Partial files belong under that job's staging folder;
final destinations must be explicitly listed in its plan. Existing files are
never replaced by the normal transfer path. Resume restarts incomplete files;
it does not assume the server supports safe append.

USB and card writes use a bundled native helper. POSIX uses directory-relative
descriptors and no-follow opens. Windows holds non-reparse directory handles
without delete/write sharing while creating a new file. Both verify the selected
volume identity, exclusively create files, flush and close before success.
Final moves use no-replace rename (macOS `renameatx_np`, Windows rename),
not delete-then-copy. The Windows implementation still requires acceptance
on a Windows runner.

FTP uses passive transfers and parsed listings, with CWD validation before LIST
so a missing directory cannot silently list the server's previous directory.
Paths sent to Vita servers are
absolute, including the leading slash (`/ux0:/...`). A successful upload is not
content verification; the coordinator reads back and hashes files. Capacity is
unknown unless the active service can supply a verified measurement. FTP has no
portable atomic no-replace rename, so final placement rechecks for conflicts;
other writers must not use the same destination during an installation.

Read-only discovery and transport construction never create folders or markers.
No transport formats, ejects, installs, launches or changes storage settings.

Implementation references:
- [Python descriptor-relative filesystem operations](https://docs.python.org/3/library/os.html)
- [Windows file handles and sharing rules](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilew)
- [libftpvita path and transfer handling](https://github.com/xerpi/libftpvita/blob/master/libftpvita/ftpvita.c)

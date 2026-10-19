"""Loopback-only read-only FTP fixture; never connects to a Vita."""
import argparse
import json
from pathlib import Path
import logging

from pyftpdlib.authorizers import DummyAuthorizer
from pyftpdlib.filesystems import AbstractedFS
from pyftpdlib.handlers import FTPHandler, DTPHandler
from pyftpdlib.servers import FTPServer

parser = argparse.ArgumentParser()
parser.add_argument("root", type=Path)
parser.add_argument("--prom", action="store_true")
parser.add_argument("--writable", action="store_true")
parser.add_argument("--fault", choices=["full", "list", "disconnect", "mid-file", "promote-reject", "promote-drop", "size-unsupported", "size-denied", "size-directory"])
args = parser.parse_args()
logging.getLogger().setLevel(logging.CRITICAL)


class VitaFS(AbstractedFS):
    def ftp2fs(self, path):
        return super().ftp2fs(path.replace("ux0:", "ux0").replace("ur0:", "ur0"))


class FaultData(DTPHandler):
    def handle_read(self):
        super().handle_read()
        if args.fault == "mid-file" and self.tot_bytes_received:
            self.file_obj.flush()
            self.cmd_channel.close()
            self.close()

    handle_read_event = handle_read


class Handler(FTPHandler):
    abstracted_fs = VitaFS
    dtp_handler = FaultData

    def ftp_SIZE(self, path):
        if args.fault == "size-directory" and self.fs.isdir(path):
            self.respond("213 4096")
        elif args.fault == "size-denied" and self.fs.isfile(path):
            self.respond("550 Permission denied.")
        elif not self.fs.lexists(path):
            self.respond("550 The file doesn't exist.")
        else:
            super().ftp_SIZE(path)

    def ftp_PROM(self, path):
        with (args.root / "promotions.jsonl").open("a") as log:
            log.write(json.dumps(path) + "\n")
        if args.fault == "promote-drop":
            self.close()
        else:
            self.respond("500 ERROR PROMOTING" if args.fault == "promote-reject" else "200 OK PROMOTING")

    def pre_process_command(self, line, cmd, arg):
        if cmd in ("CWD", "SIZE", "LIST", "MLSD", "MKD", "STOR", "RNFR", "RNTO"):
            with (args.root / "commands.jsonl").open("a") as log:
                log.write(json.dumps({"command": cmd, "argument": arg}) + "\n")
        if cmd == "SIZE" and args.fault == "size-unsupported":
            self.respond("502 SIZE unavailable.")
            return
        if cmd in ("LIST", "MLSD") and ("ux0:" in arg or "ur0:" in arg) and not arg.startswith("/"):
            self.respond("550 Vita LIST requires an absolute FTP path.")
            return
        super().pre_process_command(line, cmd, arg)

    def ftp_LIST(self, path):
        if args.fault == "list":
            self.respond("502 Listing unavailable.")
        elif path != self.fs.ftp2fs(self.fs.cwd):
            self.respond("501 Confirm directory with CWD before listing.")
        else:
            super().ftp_LIST(path)

    def ftp_MLSD(self, path):
        if args.fault == "list":
            self.respond("502 Listing unavailable.")
        elif path != self.fs.ftp2fs(self.fs.cwd):
            self.respond("501 Confirm directory with CWD before listing.")
        else:
            super().ftp_MLSD(path)

    def ftp_STOR(self, path, mode="w"):
        if args.fault == "full":
            self.respond("552 Storage full.")
        elif args.fault == "disconnect":
            self.close()
        else:
            super().ftp_STOR(path, mode)

    def ftp_FEAT(self, line):
        self.push("211-Features supported:\r\n")
        if args.prom:
            self.push(" PROM\r\n")
        self.respond("211 End FEAT.")


authorizer = DummyAuthorizer()
authorizer.add_anonymous(str(args.root), perm="elradfmw" if args.writable else "elr")
Handler.authorizer = authorizer
if args.prom:
    Handler.proto_cmds = dict(FTPHandler.proto_cmds)
    Handler.proto_cmds["PROM"] = {"perm": None, "auth": True, "arg": True, "help": "Promote a staged VPK"}
Handler.banner = "VitaShell test greeting"
server = FTPServer(("127.0.0.1", 0), Handler)
print(json.dumps({"port": server.socket.getsockname()[1]}), flush=True)
server.serve_forever(timeout=0.1, blocking=True, handle_exit=True)

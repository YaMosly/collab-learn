"""Copy selected original files from a fixed, integrity-checked npm archive.

No npm scripts or downloaded code are executed. Normal checks need no network.
"""
import argparse
import base64
import hashlib
import io
import json
from pathlib import Path
import tarfile
import urllib.request

VERSION = "5.65.2"
URL = f"https://registry.npmjs.org/codemirror/-/codemirror-{VERSION}.tgz"
INTEGRITY = "sha512-SZM4Zq7XEC8Fhroqe3LxbEEX1zUPWH1wMr5zxiBuiUF64iYOUH/JI88v4tBag8MiBS8B8gRv8O1pPXGYXQ4ErA=="
ROOT = Path(__file__).resolve().parent.parent
DEST = ROOT / "web/static/vendor" / f"codemirror-{VERSION}"
FILES = (
    "LICENSE", "lib/codemirror.js", "lib/codemirror.css", "theme/material-ocean.css",
    "mode/xml/xml.js", "mode/css/css.js", "mode/javascript/javascript.js", "mode/htmlmixed/htmlmixed.js",
    "addon/edit/closebrackets.js", "addon/edit/closetag.js", "addon/edit/matchbrackets.js", "addon/edit/matchtags.js",
    "addon/fold/foldcode.js", "addon/fold/foldgutter.js", "addon/fold/foldgutter.css",
    "addon/fold/brace-fold.js", "addon/fold/xml-fold.js", "addon/fold/comment-fold.js",
    "addon/hint/show-hint.js", "addon/hint/show-hint.css", "addon/hint/xml-hint.js",
    "addon/hint/html-hint.js", "addon/hint/css-hint.js", "addon/selection/active-line.js", "addon/comment/comment.js",
)


def download():
    with urllib.request.urlopen(URL, timeout=30) as response:
        archive = response.read(8 * 1024 * 1024 + 1)
    if len(archive) > 8 * 1024 * 1024:
        raise SystemExit("Archive exceeds size limit")
    integrity = "sha512-" + base64.b64encode(hashlib.sha512(archive).digest()).decode()
    if integrity != INTEGRITY:
        raise SystemExit("Archive integrity mismatch; nothing written")
    # Read all selected members before writing any file. No extractall/path traversal.
    selected = {}
    with tarfile.open(fileobj=io.BytesIO(archive), mode="r:gz") as package:
        for name in FILES:
            member = package.getmember("package/" + name)
            if not member.isfile() or member.size > 2 * 1024 * 1024:
                raise SystemExit("Unexpected archive member: " + name)
            selected[name] = package.extractfile(member).read()
    for name, data in selected.items():
        target = DEST / name
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(data)
    manifest = {"package": "codemirror", "version": VERSION, "source": URL, "integrity": INTEGRITY,
        "files": {name: hashlib.sha256(data).hexdigest() for name, data in selected.items()}}
    (DEST / "manifest.json").write_bytes((json.dumps(manifest, indent=2) + "\n").encode())


def check():
    manifest = json.loads((DEST / "manifest.json").read_text(encoding="utf-8"))
    if (manifest["package"], manifest["version"], manifest["source"], manifest["integrity"]) != ("codemirror", VERSION, URL, INTEGRITY):
        raise SystemExit("Unexpected package manifest")
    if set(manifest["files"]) != set(FILES):
        raise SystemExit("Unexpected vendor file set")
    for name in FILES:
        if hashlib.sha256((DEST / name).read_bytes()).hexdigest() != manifest["files"][name]:
            raise SystemExit("Vendor file checksum mismatch: " + name)
    print(f"CodeMirror {VERSION}: {len(FILES)} original files verified")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--download", action="store_true", help="Fetch the fixed archive and copy selected files")
    parser.add_argument("--check", action="store_true", help="Verify installed files offline (default)")
    args = parser.parse_args()
    if args.download:
        download()
    check()

# CodeMirror

The `codemirror-5.65.2` directory contains selected original files from the
`codemirror` npm package, version **5.65.2**. This preserves the existing editor
version; upgrading or migrating to CodeMirror 6 is a separate change.

- Source: https://registry.npmjs.org/codemirror/-/codemirror-5.65.2.tgz
- Upstream: https://codemirror.net/
- License: MIT, reproduced in [codemirror-5.65.2/LICENSE](codemirror-5.65.2/LICENSE).
- The archive's SHA-512 integrity value is pinned in `tools/vendor-codemirror.py`
  and `codemirror-5.65.2/manifest.json`. The manifest records every file's SHA-256.
- Files are copied without changes; upstream copyright/license headers remain.

Verify the installed files without network access:

```bash
python tools/vendor-codemirror.py --check
```

Recreate this same selection from the pinned archive (requires network):

```bash
python tools/vendor-codemirror.py --download
```

The script verifies the archive before writing, reads only explicitly selected
regular files and does not execute package scripts. A version change requires
reviewing the source, integrity value, file list, license and browser behavior.

HTML completion includes `xml-hint.js`; tag matching/closing includes `xml-fold.js`.
The document loads these dependencies before their consumers and includes the
fold gutter stylesheet. The application uses system font fallbacks instead of
requesting Google Fonts. User-authored preview content may still load its own
external images/styles, as described in the main README.

(function (root) {
    'use strict';

    function download(format, text, environment = root) {
        if (!['html', 'css'].includes(format) || typeof text !== 'string') throw new TypeError('Invalid source export');
        const filename = `collablearn.${format}`;
        const blob = new environment.Blob([text], { type: `text/${format};charset=utf-8`, endings: 'transparent' });
        const url = environment.URL.createObjectURL(blob);
        let anchor, requested = false;
        try {
            anchor = environment.document.createElement('a');
            anchor.href = url; anchor.download = filename; anchor.hidden = true;
            environment.document.body.appendChild(anchor);
            anchor.click();
            requested = true;
            return filename;
        } finally {
            try { anchor?.remove(); }
            finally {
                // Allow the browser to consume the Blob before releasing it.
                if (requested) environment.setTimeout(() => environment.URL.revokeObjectURL(url), 60000);
                else environment.URL.revokeObjectURL(url);
            }
        }
    }

    if (typeof module !== 'undefined' && module.exports) module.exports = { download };
    else root.CollabSourceExport = { download };
})(typeof globalThis !== 'undefined' ? globalThis : this);

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { download } = require('../web/static/js/source-export.js');
const { DocumentSync } = require('../web/static/js/document-sync.js');
const vm = require('node:vm');
const { readFileSync } = require('node:fs');
const { join } = require('node:path');

function browser(failAt) {
    const blobs = [], revoked = [], timers = [], anchors = [];
    const environment = {
        Blob, URL: { createObjectURL(blob) { blobs.push(blob); return 'blob:local-' + blobs.length; }, revokeObjectURL(url) { revoked.push(url); } },
        document: { createElement(tag) {
            assert.equal(tag, 'a'); if (failAt === 'create') throw Error('create failed');
            const anchor = { click() { if (failAt === 'click') throw Error('click failed'); this.clicked = true; }, remove() { this.removed = true; } };
            anchors.push(anchor); return anchor;
        }, body: { appendChild(anchor) { if (failAt === 'append') throw Error('append failed'); anchor.attached = true; } } },
        setTimeout(callback, delay) { timers.push({ callback, delay }); },
    };
    return { environment, blobs, revoked, timers, anchors };
}

test('download preserves exact UTF-8 HTML/CSS, empty files and original line endings', async () => {
    for (const format of ['html', 'css']) {
        for (const text of ['', 'Привет 🎵\r\n<script>raw()</script>\n\0']) {
            const b = browser();
            assert.equal(download(format, text, b.environment), `collablearn.${format}`);
            assert.equal(await b.blobs[0].text(), text);
            assert.deepEqual(Buffer.from(await b.blobs[0].arrayBuffer()), Buffer.from(text, 'utf8'));
            assert.equal(b.blobs[0].type, `text/${format};charset=utf-8`);
            assert.equal(b.anchors[0].download, `collablearn.${format}`);
            assert.equal(b.anchors[0].clicked, true); assert.equal(b.anchors[0].removed, true);
            assert.deepEqual(b.revoked, []);
            assert.equal(b.timers[0].delay, 60000); b.timers[0].callback();
            assert.deepEqual(b.revoked, ['blob:local-1']);
        }
    }
});

test('failed downloads release the Blob URL and temporary element', () => {
    for (const failAt of ['create', 'append', 'click']) {
        const b = browser(failAt);
        assert.throws(() => download('html', 'draft', b.environment));
        assert.deepEqual(b.revoked, ['blob:local-1']); assert.equal(b.timers.length, 0);
        if (b.anchors.length) assert.equal(b.anchors[0].removed, true);
    }
    const b = browser();
    assert.throws(() => download('../bad', 'draft', b.environment));
    assert.throws(() => download('html', undefined, b.environment));
    assert.equal(b.blobs.length, 0);
});

function app(sync, fail = false) {
    const elements = new Map(), exported = [];
    const context = vm.createContext({
        document: { addEventListener() {}, getElementById(id) {
            if (!elements.has(id)) elements.set(id, { dataset: {}, textContent: id === 'save-status' ? 'Нет связи' : '' });
            return elements.get(id);
        } }, clearTimeout() {}, setTimeout() {}, sync,
        fetch() { throw Error('export must never contact server'); },
        CollabSourceExport: { download(format, text) { if (fail) throw Error('blocked'); exported.push({ format, text }); return `collablearn.${format}`; } },
        window: { get sessionStorage() { throw Error('storage blocked'); } },
    });
    vm.runInContext(readFileSync(join(__dirname, '../web/static/js/retry-policy.js'), 'utf8'), context);
    vm.runInContext(readFileSync(join(__dirname, '../web/static/js/app.js'), 'utf8'), context);
    vm.runInContext("documentSync = sync; htmlEditor = { getValue: () => 'latest HTML 🎵' }; cssEditor = { getValue: () => '' }", context);
    return { elements, exported, run: code => vm.runInContext(code, context) };
}

test('export reads the latest editor text without changing offline/conflict/uncertain draft state', () => {
    let storageAccesses = 0, writes = 0;
    const sync = new DocumentSync({ revision: 0, code_html: '', code_css: '' }, {
        storage: { getItem() { storageAccesses++; throw Error('blocked'); }, setItem() { storageAccesses++; throw Error('blocked'); } },
        key: 'draft', write() { writes++; throw Error('must not write'); },
    });
    sync.edit({ code_html: 'local conflict', code_css: 'local CSS' });
    sync.receive({ revision: 1, code_html: 'server conflict', code_css: '' });
    sync.uncertain = { base_revision: 0, code_html: 'unconfirmed', code_css: '' };
    const before = JSON.stringify({ state: sync.state(), uncertain: sync.uncertain, inFlight: sync.inFlight });
    const readsBefore = storageAccesses, f = app(sync);
    f.run("exportSource('html'); exportSource('css')");
    assert.deepEqual(f.exported, [{ format: 'html', text: 'latest HTML 🎵' }, { format: 'css', text: '' }]);
    assert.equal(JSON.stringify({ state: sync.state(), uncertain: sync.uncertain, inFlight: sync.inFlight }), before);
    assert.equal(storageAccesses, readsBefore); assert.equal(writes, 0);
    assert.equal(f.elements.get('save-status'), undefined, 'export must not write save status');
});

test('export failures show an error while the pending draft remains untouched', () => {
    const pending = Object.freeze({ dirty: true, inFlight: Object.freeze({ code_html: 'pending' }), online: true });
    const f = app(pending, true);
    f.run("exportSource('html')");
    assert.match(f.elements.get('notice-text').textContent, /Не удалось начать скачивание/);
    assert.equal(pending.dirty, true); assert.equal(pending.inFlight.code_html, 'pending');
});

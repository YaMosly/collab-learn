const { test } = require('node:test');
const assert = require('node:assert/strict');
const { DocumentSync } = require('../web/static/js/document-sync.js');

const board = (revision = 0, html = '', css = '') => ({ revision, code_html: html, code_css: css });
function storage() {
    const data = new Map();
    return { getItem: key => data.get(key), setItem: (key, value) => data.set(key, value), removeItem: key => data.delete(key) };
}
function server(initial = board()) {
    let current = initial;
    const writes = [];
    return {
        writes, get current() { return { ...current }; },
        async read() { return { ...current }; },
        async write(request) {
            writes.push({ ...request });
            if (request.base_revision !== current.revision) return { conflict: true, board: { ...current } };
            current = board(current.revision + 1, request.code_html, request.code_css);
            return { conflict: false, board: { ...current } };
        },
    };
}
function client(api, options = {}) {
    return new DocumentSync(api.current, { read: () => api.read(), write: value => api.write(value),
        key: 'draft', storage: storage(), ...options });
}
const edit = (client, html, css = client.local.code_css) => client.edit({ code_html: html, code_css: css });
function deferred() {
    let resolve, reject;
    const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
    return { promise, resolve, reject };
}

test('deleting every character on a loaded board is saved', async () => {
    const api = server(board(0, 'saved text')), c = client(api);
    await c.reconnect(); edit(c, ''); await c.flush();
    assert.equal(api.current.code_html, ''); assert.equal(api.current.revision, 1); assert.equal(c.dirty, false);
});

test('returning to a previously sent value after a remote edit is saved', async () => {
    const api = server(), c = client(api);
    await c.reconnect(); edit(c, 'A'); await c.flush();
    const remote = await api.write({ base_revision: 1, code_html: 'B', code_css: '' });
    c.receive(remote.board); edit(c, 'A'); await c.flush();
    assert.equal(api.current.code_html, 'A'); assert.equal(api.current.revision, 3);
});

test('offline edits survive reload and save after reconnect', async () => {
    const api = server(), draft = storage(), c = client(api, { storage: draft });
    edit(c, 'offline'); await c.flush(); assert.equal(api.writes.length, 0);
    const restored = client(api, { storage: draft });
    assert.equal(restored.local.code_html, 'offline');
    await restored.reconnect(); assert.equal(api.current.code_html, 'offline'); assert.equal(draft.getItem('draft'), undefined);
});

test('concurrent HTML and CSS changes merge with conditional retries', async () => {
    const api = server(), a = client(api), b = client(api);
    await Promise.all([a.reconnect(), b.reconnect()]);
    edit(a, '<h1>A</h1>'); edit(b, '', 'body { color: red; }');
    await Promise.all([a.flush(), b.flush()]);
    assert.deepEqual(api.current, board(2, '<h1>A</h1>', 'body { color: red; }'));
    assert.deepEqual(b.state().conflicts, []);
});

test('same-field conflict retains local text and exposes the server text', async () => {
    const api = server(), a = client(api), b = client(api);
    await Promise.all([a.reconnect(), b.reconnect()]); edit(a, 'A'); edit(b, 'B');
    await a.flush(); await b.flush();
    assert.equal(api.current.code_html, 'A'); assert.equal(b.local.code_html, 'B');
    assert.equal(b.state().remote.code_html, 'A'); assert.deepEqual(b.state().conflicts, ['code_html']);
    await b.resolve('local'); assert.equal(api.current.code_html, 'B'); assert.equal(api.current.revision, 2);
});

test('accepting a server conflict preserves unrelated unsaved local CSS', async () => {
    const api = server(), c = client(api);
    await c.reconnect(); edit(c, 'local', 'p { color: blue; }');
    const remote = await api.write({ base_revision: 0, code_html: 'remote', code_css: '' });
    c.receive(remote.board); await c.resolve('server');
    assert.deepEqual(api.current, board(2, 'remote', 'p { color: blue; }'));
});

test('typing during a pending save is coalesced and not mistaken for an acknowledged version', async () => {
    const api = server(), response = deferred();
    let first = true;
    const c = client(api, { write: async request => {
        const result = await api.write(request);
        if (first) { first = false; await response.promise; }
        return result;
    } });
    await c.reconnect(); edit(c, 'A'); const saving = c.flush();
    edit(c, 'AB'); assert.equal(c.state().busy, true);
    response.resolve(); await saving;
    assert.equal(api.current.code_html, 'AB'); assert.equal(api.current.revision, 2); assert.equal(c.dirty, false);
});

test('an early WebSocket echo does not turn further local typing into a conflict', async () => {
    const api = server(), response = deferred(); let first = true;
    const c = client(api, { write: async request => {
        const result = await api.write(request);
        if (first) { first = false; await response.promise; }
        return result;
    } });
    await c.reconnect(); edit(c, 'A'); const saving = c.flush();
    await Promise.resolve(); c.receive(api.current); edit(c, 'AB');
    response.resolve(); await saving;
    assert.equal(api.current.code_html, 'AB'); assert.deepEqual(c.state().conflicts, []);
});

test('a newer remote revision received during a save is merged after its acknowledgement', async () => {
    const api = server(), response = deferred(); let first = true;
    const c = client(api, { write: async request => {
        const result = await api.write(request);
        if (first) { first = false; await response.promise; }
        return result;
    } });
    await c.reconnect(); edit(c, 'A'); const saving = c.flush();
    await Promise.resolve();
    await api.write({ base_revision: 1, code_html: 'A', code_css: 'remote CSS' });
    c.receive(api.current); edit(c, 'AB'); response.resolve(); await saving;
    assert.deepEqual(api.current, board(3, 'AB', 'remote CSS'));
});

test('undoing to the original text during a pending save survives reload after commit', async () => {
    const api = server(), draft = storage(), response = deferred();
    const c = client(api, { storage: draft, write: async request => { const saved = await api.write(request); await response.promise; return saved; } });
    await c.reconnect(); edit(c, 'A'); const saving = c.flush(); edit(c, '');
    assert.ok(draft.getItem('draft'));
    const restored = client(api, { storage: draft });
    assert.equal(restored.local.code_html, ''); await restored.reconnect();
    assert.equal(api.current.code_html, '');
    response.resolve(); await saving;
});

test('a failed save keeps a recoverable draft and never reports clean', async () => {
    const api = server(), draft = storage();
    const c = client(api, { storage: draft, write: async () => { throw new Error('network error'); } });
    await c.reconnect(); edit(c, 'keep me'); await c.flush();
    assert.equal(c.online, false); assert.equal(c.dirty, true); assert.ok(draft.getItem('draft'));
    assert.equal(c.state().error, 'network error');
});

test('older reconnect completions cannot revive a disconnected session', async () => {
    const api = server(), read = deferred(), c = client(api, { read: () => read.promise });
    edit(c, 'draft'); const reconnecting = c.reconnect(); c.disconnect();
    read.resolve(board()); await reconnecting;
    assert.equal(c.online, false); assert.equal(api.writes.length, 0); assert.equal(c.local.code_html, 'draft');
});

test('older broadcasts cannot roll back a newer server revision', async () => {
    const c = client(server(board(2, 'new'))); c.receive(board(1, 'old'));
    assert.equal(c.local.code_html, 'new'); assert.equal(c.base.revision, 2);
});

test('conflicts survive reload even if the server revision has not changed', async () => {
    const api = server(), draft = storage(), c = client(api, { storage: draft });
    edit(c, 'local'); await api.write({ base_revision: 0, code_html: 'remote', code_css: '' });
    c.receive(api.current);
    const restored = client(api, { storage: draft }); await restored.reconnect();
    assert.deepEqual(restored.state().conflicts, ['code_html']); assert.equal(api.writes.length, 1);
});

test('storage failures are visible and do not discard the in-memory edit', () => {
    const c = client(server(), { storage: { getItem() {}, setItem() { throw new Error('quota'); } } });
    edit(c, 'local'); assert.equal(c.local.code_html, 'local'); assert.ok(c.state().draftWarning);
});

test('non-advancing conflicts stop instead of retrying forever', async () => {
    const api = server(), c = client(api, { write: async () => ({ conflict: true, board: board() }) });
    await c.reconnect(); edit(c, 'local'); await c.flush();
    assert.equal(c.online, false); assert.ok(c.state().error); assert.equal(c.dirty, true);
});

test('a lost acknowledgement plus undo survives a failed request and reload', async () => {
    const api = server(), draft = storage(), response = deferred();
    const c = client(api, { storage: draft, write: async request => {
        await api.write(request); await response.promise; throw new Error('response lost');
    } });
    await c.reconnect(); edit(c, 'A'); const saving = c.flush(); edit(c, '');
    response.resolve(); await saving;
    assert.equal(c.dirty, true); assert.ok(draft.getItem('draft'));
    const restored = client(api, { storage: draft }); await restored.reconnect();
    assert.equal(api.current.code_html, ''); assert.equal(restored.dirty, false);
});

test('a failed write that never reached the server still retries the retained text', async () => {
    const api = server(), draft = storage(), c = client(api, { storage: draft,
        write: async () => { throw new Error('request lost'); } });
    await c.reconnect(); edit(c, 'A'); await c.flush();
    const restored = client(api, { storage: draft }); await restored.reconnect();
    assert.equal(api.current.code_html, 'A'); assert.equal(restored.dirty, false);
});

test('editing after a rejected write keeps the retry action available', async () => {
    const api = server(), c = client(api, { write: async () => { throw new Error('document too large'); } });
    await c.reconnect(); edit(c, 'large'); await c.flush(); edit(c, 'smaller');
    assert.equal(c.online, false); assert.equal(c.state().error, 'document too large');
});

test('a lost response merges a newer edit with another writer’s CSS after reload', async () => {
    const api = server(), draft = storage(), response = deferred();
    const c = client(api, { storage: draft, write: async request => {
        await api.write(request); await response.promise; throw new Error('response lost');
    } });
    await c.reconnect(); edit(c, 'A'); const saving = c.flush(); edit(c, 'AB');
    await api.write({ base_revision: 1, code_html: 'A', code_css: 'remote CSS' });
    response.resolve(); await saving;
    const restored = client(api, { storage: draft }); await restored.reconnect();
    assert.deepEqual(api.current, board(3, 'AB', 'remote CSS')); assert.deepEqual(restored.state().conflicts, []);
});

test('periodic refresh recovers a missing notification without losing local HTML', async () => {
    const api = server(), c = client(api);
    await c.reconnect(); edit(c, 'local HTML');
    await api.write({ base_revision: 0, code_html: '', code_css: 'missed CSS' });
    await c.refresh();
    assert.deepEqual(api.current, board(2, 'local HTML', 'missed CSS'));
    assert.equal(c.online, true); assert.deepEqual(c.state().conflicts, []);
});

test('refresh keeps both versions when a missed update conflicts in the same field', async () => {
    const api = server(), c = client(api);
    await c.reconnect(); edit(c, 'local');
    await api.write({ base_revision: 0, code_html: 'remote', code_css: '' });
    await c.refresh();
    assert.equal(c.local.code_html, 'local'); assert.equal(api.current.code_html, 'remote');
    assert.deepEqual(c.state().conflicts, ['code_html']);
});

test('a stale refresh response cannot overwrite a disconnected draft', async () => {
    const api = server(), pending = deferred(); let initial = true;
    const c = client(api, { read: () => { if (initial) { initial = false; return api.read(); } return pending.promise; } });
    await c.reconnect(); const refreshing = c.refresh(); c.disconnect(); edit(c, 'offline draft');
    pending.resolve(board(1, 'stale network result')); await refreshing;
    assert.equal(c.online, false); assert.equal(c.local.code_html, 'offline draft'); assert.equal(c.base.revision, 0);
});

test('refresh failures keep the draft and request reconnect', async () => {
    const api = server(); let initial = true, failures = 0;
    const c = client(api, { read: async () => { if (initial) { initial = false; return api.read(); } throw new Error('network'); }, failed: () => { failures++; } });
    await c.reconnect(); edit(c, 'draft'); await c.refresh();
    assert.equal(c.online, false); assert.equal(c.local.code_html, 'draft'); assert.equal(failures, 1);
});

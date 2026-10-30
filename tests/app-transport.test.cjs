const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const { readFileSync } = require('node:fs');
const { join } = require('node:path');
const { DocumentSync } = require('../web/static/js/document-sync.js');

test('the request deadline covers a stalled response body, not just headers', async () => {
    let deadline, cleared = false, bodyStarted = false;
    let enteredBody;
    const bodyEntered = new Promise(resolve => { enteredBody = resolve; });
    const context = vm.createContext({
        document: { addEventListener() {} }, AbortController,
        setTimeout(callback) { deadline = callback; return 1; },
        clearTimeout() { cleared = true; },
        async fetch(_url, { signal }) {
            return { ok: true, status: 200, json() {
                bodyStarted = true;
                enteredBody();
                return new Promise((_resolve, reject) => signal.addEventListener('abort', () => reject(new Error('aborted'))));
            } };
        },
    });
    vm.runInContext(readFileSync(join(__dirname, '../web/static/js/app.js'), 'utf8'), context);
    const response = vm.runInContext("fetchBoardResult('/api/boards/id')", context);
    await bodyEntered;
    assert.equal(bodyStarted, true); assert.equal(cleared, false);
    deadline(); await assert.rejects(response, /aborted/); assert.equal(cleared, true);
});

test('remote setValue events are suppressed synchronously without suppressing the next local edit', () => {
    const elements = new Map(), editors = [], drafts = new Map();
    const context = vm.createContext({
        document: { addEventListener() {}, getElementById(id) {
            if (!elements.has(id)) elements.set(id, {});
            return elements.get(id);
        } },
        window: { sessionStorage: { getItem: key => drafts.get(key), setItem: (key, value) => drafts.set(key, value), removeItem: key => drafts.delete(key) } },
        CollabDocumentSync: DocumentSync,
        CodeMirror: { fromTextArea() {
            const editor = { value: '', changed: null, getValue() { return this.value; },
                setValue(value) { this.value = value; this.changed?.(); },
                getCursor() { return {}; }, setCursor() {}, on(_event, callback) { this.changed = callback; } };
            editors.push(editor); return editor;
        } },
        setTimeout() { return 1; }, clearTimeout() {},
    });
    vm.runInContext(readFileSync(join(__dirname, '../web/static/js/app.js'), 'utf8'), context);
    vm.runInContext("initializeEditors({revision:0, code_html:'A', code_css:''})", context);
    vm.runInContext("handleWebSocketMessage({type:'update',data:{revision:1,code_html:'B',code_css:''}})", context);
    assert.equal(editors[0].value, 'B');
    assert.equal(vm.runInContext('documentSync.dirty', context), false);
    editors[0].setValue('C');
    assert.equal(vm.runInContext('documentSync.dirty', context), true);
    assert.equal(vm.runInContext('documentSync.local.code_html', context), 'C');
});

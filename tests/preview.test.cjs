const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { join } = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');

const root = join(__dirname, '..');
const markup = readFileSync(join(root, 'web/static/index.html'), 'utf8');
const client = readFileSync(join(root, 'web/static/js/app.js'), 'utf8');

function preview(html = '', css = '') {
    const frame = {};
    const context = vm.createContext({
        document: {
            addEventListener() {},
            getElementById(id) {
                assert.equal(id, 'preview-frame');
                return frame;
            },
        },
        html, css,
    });
    vm.runInContext(client, context);
    vm.runInContext(`
        htmlEditor = { getValue: () => html };
        cssEditor = { getValue: () => css };
        updatePreview();
    `, context);
    return { frame, context };
}

test('the user-content frame has a full sandbox and no referrer', () => {
    const iframe = markup.match(/<iframe\b[^>]*\bid="preview-frame"[^>]*>/i)?.[0];
    assert.ok(iframe, 'The preview frame must exist');
    // In particular, neither allow-scripts nor allow-same-origin is permitted.
    assert.match(iframe, /\bsandbox=""/);
    assert.match(iframe, /\breferrerpolicy="no-referrer"/);
    assert.match(iframe, /\btitle="[^"]+"/);
});

test('HTML and CSS are rendered as the document of the same isolated frame', () => {
    const { frame } = preview('<h1>Hello</h1>', 'h1 { color: red; }');
    assert.match(frame.srcdoc, /<!DOCTYPE html>/);
    assert.match(frame.srcdoc, /<style>h1 \{ color: red; \}<\/style>/);
    assert.match(frame.srcdoc, /<body><h1>Hello<\/h1><\/body>/);
    assert.equal(frame.src, undefined);
});

test('consecutive updates replace the prior preview without allocating Blob URLs', () => {
    // Blob and URL deliberately are not available in the context.
    const { frame, context } = preview('<p>First</p>', '');
    context.html = '<p>Second</p>';
    context.css = 'p { color: blue; }';
    vm.runInContext('updatePreview()', context);
    assert.match(frame.srcdoc, /<p>Second<\/p>/);
    assert.match(frame.srcdoc, /color: blue/);
    assert.doesNotMatch(frame.srcdoc, /First/);
});

test('an empty board still produces a complete preview document', () => {
    const { frame } = preview();
    assert.match(frame.srcdoc, /<style><\/style>/);
    assert.match(frame.srcdoc, /<body><\/body>/);
    assert.doesNotMatch(frame.srcdoc, /undefined|null/);
});

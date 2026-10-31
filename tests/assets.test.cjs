const { test } = require('node:test');
const assert = require('node:assert/strict');
const { createHash } = require('node:crypto');
const { readFileSync, statSync } = require('node:fs');
const { resolve, dirname } = require('node:path');
const vm = require('node:vm');

const root = resolve(__dirname, '../web/static');
const vendor = resolve(root, 'vendor/codemirror-5.65.2');
const html = readFileSync(resolve(root, 'index.html'), 'utf8');
const scripts = [...html.matchAll(/<script\b[^>]*\bsrc="([^"]+)"/g)].map(match => match[1]);
const styles = [...html.matchAll(/<link\b[^>]*\brel="stylesheet"[^>]*\bhref="([^"]+)"/g)].map(match => match[1]);
const manifest = JSON.parse(readFileSync(resolve(vendor, 'manifest.json'), 'utf8'));
const fileFor = url => resolve(root, '.' + url.replace(/^\/static/, ''));

test('editor scripts and styles are complete local resources with no remote CSS imports', () => {
    assert.ok(scripts.length > 10 && styles.length > 3);
    for (const url of [...scripts, ...styles]) {
        assert.ok(url.startsWith('/static/'), 'external dependency: ' + url);
        assert.equal(statSync(fileFor(url)).isFile(), true, 'missing dependency: ' + url);
    }
    for (const url of styles) {
        const css = readFileSync(fileFor(url), 'utf8');
        assert.doesNotMatch(css, /@import\b|url\(\s*['"]?(?:https?:)?\/\//i, 'network CSS dependency: ' + url);
    }
});

test('upstream assets match the pinned manifest and retain their license', () => {
    assert.equal(manifest.package, 'codemirror'); assert.equal(manifest.version, '5.65.2');
    for (const [file, digest] of Object.entries(manifest.files)) {
        const bytes = readFileSync(resolve(vendor, file));
        assert.equal(createHash('sha256').update(bytes).digest('hex'), digest, 'modified vendor file: ' + file);
    }
    assert.match(readFileSync(resolve(vendor, 'LICENSE'), 'utf8'), /Permission is hereby granted/);
    assert.match(readFileSync(resolve(vendor, 'lib/codemirror.js'), 'utf8'), /version = "5\.65\.2"/);
});

test('vendor JavaScript parses and browser script order covers each addon dependency', () => {
    const loaded = new Set();
    for (const url of scripts) {
        const file = fileFor(url), source = readFileSync(file, 'utf8');
        new vm.Script(source, { filename: file });
        if (file.startsWith(vendor)) {
            for (const match of source.matchAll(/require\("(\.[^"]+)"\)/g)) {
                const dependency = resolve(dirname(file), match[1] + '.js');
                assert.ok(loaded.has(dependency), url + ' must load after ' + match[1]);
            }
        }
        loaded.add(file);
    }
    for (const file of Object.keys(manifest.files).filter(file => file.endsWith('.js'))) {
        assert.ok(loaded.has(resolve(vendor, file)), 'unused or missing script: ' + file);
    }
});

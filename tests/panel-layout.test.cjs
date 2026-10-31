const { test } = require('node:test');
const assert = require('node:assert/strict');
const { PanelLayout, fitShares, resizePair } = require('../web/static/js/panel-layout.js');

function fixture(narrow = false) {
    const document = { activeElement: null }, elements = new Map(), frames = new Map();
    let sequence = 0, width = narrow ? 360 : 1440;
    class Element {
        constructor(id) { this.id = id; this.attrs = {}; this.style = {}; this.hidden = false; this.handlers = new Map(); this.ownerDocument = document; this.capture = new Set();
            this.classes = new Set(); this.classList = { add: name => this.classes.add(name), remove: name => this.classes.delete(name) }; }
        addEventListener(type, fn) { if (!this.handlers.has(type)) this.handlers.set(type, new Set()); this.handlers.get(type).add(fn); }
        removeEventListener(type, fn) { this.handlers.get(type)?.delete(fn); }
        fire(type, extra = {}) { const event = { preventDefault() {}, ...extra }; for (const fn of this.handlers.get(type) || []) fn(event); }
        setAttribute(key, value) { this.attrs[key] = value; }
        focus() { document.activeElement = this; }
        contains(element) { return element === this || element?.parent === this; }
        getBoundingClientRect() { return { width: this.id.endsWith('splitter') ? 10 : width }; }
        setPointerCapture(id) { this.capture.add(id); }
        hasPointerCapture(id) { return this.capture.has(id); }
        releasePointerCapture(id) { this.capture.delete(id); }
    }
    for (const id of ['editor-container', 'editor-panels', 'editor-tabs', 'splitter-help', 'html-tab', 'css-tab', 'preview-tab', 'html-panel', 'css-panel', 'preview-panel', 'html-splitter', 'css-splitter']) elements.set(id, new Element(id));
    const root = elements.get('editor-container'); root.querySelector = selector => elements.get(selector.slice(1));
    const media = new Element('media'); media.matches = narrow;
    const window = new Element('window');
    window.matchMedia = () => media;
    window.requestAnimationFrame = fn => { frames.set(++sequence, fn); return sequence; };
    window.cancelAnimationFrame = id => frames.delete(id);
    window.ResizeObserver = class { constructor(fn) { this.callback = fn; } observe() {} disconnect() { this.disconnected = true; } };
    document.defaultView = window;
    const editors = Object.fromEntries(['html', 'css'].map(name => [name, { text: 'long '.repeat(100), cursor: { line: 12, ch: 7 }, refreshes: 0,
        getScrollInfo() { return { left: 8, top: 123 }; }, refresh() { this.refreshes++; }, scrollTo(left, top) { assert.equal(left, 8); assert.equal(top, 123); } }]));
    const layout = new PanelLayout({ root, editors });
    return { layout, editors, document, media, elements, window, frames,
        flush() { const callbacks = [...frames.values()]; frames.clear(); callbacks.forEach(fn => fn()); },
        mode(value) { media.matches = value; width = value ? 360 : 1440; media.fire('change'); },
        width(value) { width = value; window.fire('resize'); } };
}

test('resizing clamps neighboring panels, preserves their total and fits a smaller desktop', () => {
    const start = [.3, .4, .3];
    for (const delta of [-10, -.05, .05, 10]) {
        const result = resizePair(start, 0, delta, .15);
        assert.ok(result[0] >= .15 && result[1] >= .15 - 1e-12); assert.equal(result[2], .3);
        assert.ok(Math.abs(result.reduce((a, b) => a + b) - 1) < 1e-12);
    }
    const fitted = fitShares([.05, .7, .25], .2);
    assert.ok(fitted.every(value => value >= .2)); assert.ok(Math.abs(fitted.reduce((a, b) => a + b) - 1) < 1e-12);
});

test('mobile tab keys expose one panel and preserve editor text, cursor and scroll', () => {
    const f = fixture(true), original = JSON.stringify(Object.values(f.editors).map(e => [e.text, e.cursor]));
    f.flush(); assert.equal(f.elements.get('css-panel').hidden, true);
    f.elements.get('html-tab').fire('keydown', { key: 'ArrowRight' }); f.flush();
    assert.equal(f.document.activeElement.id, 'css-tab'); assert.equal(f.elements.get('css-panel').hidden, false);
    assert.equal(f.elements.get('html-panel').hidden, true); assert.equal(f.elements.get('css-panel').attrs.role, 'tabpanel');
    assert.equal(f.elements.get('css-tab').attrs['aria-selected'], 'true'); assert.equal(f.elements.get('html-tab').tabIndex, -1);
    assert.equal(f.elements.get('html-splitter').hidden, true);
    assert.equal(f.elements.get('splitter-help').hidden, true);
    f.elements.get('css-tab').fire('keydown', { key: 'End' }); f.flush(); assert.equal(f.layout.active, 2);
    f.elements.get('preview-tab').fire('keydown', { key: 'Home' }); f.flush(); assert.equal(f.layout.active, 0);
    f.elements.get('html-tab').fire('keydown', { key: 'ArrowLeft' }); f.flush(); assert.equal(f.layout.active, 2);
    f.elements.get('preview-tab').fire('keydown', { key: 'ArrowRight' }); f.flush(); assert.equal(f.layout.active, 0);
    assert.equal(JSON.stringify(Object.values(f.editors).map(e => [e.text, e.cursor])), original);
});

test('desktop splitters support arrows and limits with a valid accessible range', () => {
    const f = fixture(), handle = f.elements.get('html-splitter'); f.flush();
    const third = f.layout.shares[2]; handle.fire('keydown', { key: 'ArrowRight' }); f.flush();
    assert.ok(f.layout.shares[0] > 1/3); assert.equal(f.layout.shares[2], third);
    for (const key of ['Home', 'End']) {
        handle.fire('keydown', { key }); f.flush();
        const a = handle.attrs;
        assert.ok(+a['aria-valuenow'] >= +a['aria-valuemin'] && +a['aria-valuenow'] <= +a['aria-valuemax']);
    }
    f.width(1030); f.flush(); assert.ok(f.layout.shares.every(value => value >= 240 / 1010 - 1e-12));
    handle.fire('dblclick'); assert.deepEqual(f.layout.shares, [1/3, 1/3, 1/3]);
});

test('drag cancellation, viewport mode changes and teardown restore pointer handling', () => {
    const f = fixture(), handle = f.elements.get('css-splitter'); f.flush();
    handle.fire('pointerdown', { button: 0, pointerId: 1, clientX: 800 });
    handle.fire('pointerdown', { button: 0, pointerId: 2, clientX: 850, isPrimary: false });
    assert.equal(f.layout.drag.id, 1);
    handle.fire('pointermove', { pointerId: 1, clientX: 900 }); assert.ok(f.layout.shares[1] > 1/3);
    handle.fire('pointercancel', { pointerId: 1 }); assert.equal(handle.hasPointerCapture(1), false);
    assert.equal(f.elements.get('editor-panels').classes.has('is-resizing'), false);
    f.document.activeElement = { parent: f.elements.get('css-panel') }; f.mode(true); f.flush();
    assert.equal(f.layout.active, 1); assert.equal(f.elements.get('css-panel').hidden, false);
    f.elements.get('css-tab').focus(); f.mode(false); f.flush(); assert.equal(f.document.activeElement.id, 'css-panel');
    f.layout.destroy(); assert.equal(f.layout.observer.disconnected, true);
    assert.ok([...handle.handlers.values()].every(set => set.size === 0)); assert.equal(f.frames.size, 0);
});

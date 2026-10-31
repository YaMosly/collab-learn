const { test } = require('node:test');
const assert = require('node:assert/strict');
const { BoardHistory } = require('../web/static/js/board-history.js');
const vm = require('node:vm');
const { readFileSync } = require('node:fs');
const { join } = require('node:path');

const id = n => `00000000-0000-4000-8000-${String(n).padStart(12, '0')}`;
function memory(initial) {
    let value = initial;
    return { getItem() { return value; }, setItem(_key, next) { value = next; }, removeItem() { value = null; } };
}

test('history stores only twenty deduplicated links and opening one moves it first', () => {
    const store = memory(), history = new BoardHistory(store);
    for (let n = 0; n < 25; n++) assert.equal(history.remember(id(n), n), true);
    assert.equal(history.list().length, 20);
    assert.equal(history.list()[0].id, id(24));
    history.remember(id(10), 30);
    assert.equal(history.list()[0].id, id(10));
    assert.equal(history.list().filter(e => e.id === id(10)).length, 1);
    history.clear(); assert.deepEqual(history.list(), []);
});

test('malformed or unavailable local storage cannot inject links or prevent opening a board', () => {
    for (const value of ['broken', '{}', 'x'.repeat(17000)]) {
        const history = new BoardHistory(memory(value)); assert.deepEqual(history.list(), []);
    }
    const history = new BoardHistory(memory(JSON.stringify([
        { id: '<img src=x onerror=alert(1)>', openedAt: 1 }, { id: id(1), openedAt: 2, code_html: 'never retain code' },
        { id: id(1), openedAt: 3 }, { id: id(2), openedAt: 'bad' }, null,
    ])));
    assert.deepEqual(history.list(), [{ id: id(1), openedAt: 2 }]);
    assert.equal(history.remember('../api/boards'), false);
    for (const storage of [null, { getItem() { throw Error('blocked'); }, setItem() { throw Error('quota'); }, removeItem() { throw Error('blocked'); } }]) {
        const blocked = new BoardHistory(storage);
        assert.deepEqual(blocked.list(), []); assert.equal(blocked.remember(id(1)), false);
        assert.equal(blocked.clear(), false); assert.equal(blocked.available, false);
    }
});

function appContext(extra = {}) {
    const context = vm.createContext({ document: { addEventListener() {} }, URLSearchParams, ...extra });
    vm.runInContext(readFileSync(join(__dirname, '../web/static/js/retry-policy.js'), 'utf8'), context);
    vm.runInContext(readFileSync(join(__dirname, '../web/static/js/app.js'), 'utf8'), context);
    return context;
}

test('the home page reads local history without requesting the global board directory', () => {
    let fetched = false;
    const elements = new Map([['board-message', {}], ['boards-container', {}]]);
    const context = appContext({ document: { addEventListener() {}, getElementById: id => elements.get(id) },
        fetch() { fetched = true; throw Error('global listing forbidden'); } });
    vm.runInContext('boardHistory = { available: true, list: () => [] }; loadBoardsList()', context);
    assert.equal(fetched, false);
    assert.match(elements.get('boards-container').textContent, /доски/);
});

test('legacy links are upgraded to fragments and rate limiting stays retryable', () => {
    let upgraded;
    const context = appContext({ window: { location: { search: `?board=${id(1)}`, hash: '' },
        history: { replaceState(_state, _title, path) { upgraded = path; } } } });
    assert.equal(vm.runInContext('getBoardIdFromURL()', context), id(1));
    assert.equal(upgraded, `/#board=${id(1)}`);
    assert.equal(vm.runInContext('permanentStatus(429)', context), false);
    assert.equal(vm.runInContext('permanentStatus(408)', context), false);
    assert.equal(vm.runInContext('permanentStatus(403)', context), true);
});

test('a failed create reports the error and never redirects to an undefined board', async () => {
    const elements = new Map();
    const context = appContext({ document: { addEventListener() {}, getElementById(id) {
        if (!elements.has(id)) elements.set(id, { dataset: {} }); return elements.get(id);
    } }, window: { location: { href: '/' } },
        fetch: async () => ({ ok: false, status: 429 }), AbortController, setTimeout, clearTimeout });
    await vm.runInContext('createNewBoard()', context);
    assert.equal(elements.get('create-board').disabled, false);
    assert.match(elements.get('notice-text').textContent, /Создание доски не подтверждено/);
    assert.equal(context.window.location.href, '/');
});

test('fragment navigation reloads the board and a canceled dirty navigation restores its URL', () => {
    let reloaded = 0, restored, accept = false;
    const context = appContext({ window: { location: { search: '', hash: `#board=${id(2)}`, reload() { reloaded++; } },
        confirm() { return accept; }, history: { replaceState(_state, _title, path) { restored = path; } } } });
    vm.runInContext(`currentBoardId = '${id(1)}'; documentSync = { dirty: true }; handleHashNavigation()`, context);
    assert.equal(reloaded, 0); assert.equal(restored, `/#board=${id(1)}`);
    accept = true;
    vm.runInContext('handleHashNavigation()', context);
    assert.equal(reloaded, 1); assert.equal(vm.runInContext('navigationConfirmed', context), true);
});

const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const { readFileSync } = require('node:fs');
const { join } = require('node:path');
const { RetryPolicy, retryAfterMs } = require('../web/static/js/retry-policy.js');

test('Retry-After accepts seconds and HTTP-date, including clock skew, and rejects shorthand', () => {
    const now = Date.parse('Sat, 31 Oct 2026 12:00:00 GMT');
    assert.equal(retryAfterMs('30', now), 30000);
    assert.equal(retryAfterMs('Sat, 31 Oct 2026 12:00:20 GMT', now), 20000);
    assert.equal(retryAfterMs('Sat, 31 Oct 2026 12:00:20 GMT', now + 3600000, 'Sat, 31 Oct 2026 12:00:00 GMT'), 20000);
    assert.equal(retryAfterMs('Sat, 31 Oct 2026 12:00:20 GMT', now, '0'), 20000);
    for (const value of [null, '', '-1', '1.5', 'NaN', 'Infinity', '9999999999999999999999', 'tomorrow', '2026-11-01']) {
        assert.equal(retryAfterMs(value, now), 0, String(value));
    }
});

test('backoff is bounded with jitter and cannot shorten an existing server cooldown', () => {
    let now = 0;
    const policy = new RetryPolicy({ now: () => now, random: () => 0.5 });
    assert.equal(policy.nextDelay(), 1100); assert.equal(policy.nextDelay(), 2200);
    policy.hold('90'); policy.reset();
    assert.equal(policy.nextDelay(), 90000);
    now += 10000; policy.hold('2'); assert.equal(policy.remaining(), 80000);
    now += 80000;
    for (let i = 0; i < 20; i++) assert.ok(policy.nextDelay() <= 36000);
});

function fixture(fetch = async () => ({ ok: true, status: 200, json: async () => ({}) })) {
    let now = 0, sequence = 0;
    const timers = new Map(), elements = new Map(), sockets = [];
    const getElementById = id => {
        if (!elements.has(id)) elements.set(id, { dataset: {}, addEventListener() {}, focus() {}, select() {} });
        return elements.get(id);
    };
    class Clock extends Date { static now() { return now; } }
    class Socket {
        static CONNECTING = 0; static OPEN = 1;
        constructor(url) { this.url = url; this.readyState = 0; sockets.push(this); }
        close() { this.readyState = 3; this.onclose?.(); }
        async open() { this.readyState = 1; await this.onopen?.(); }
    }
    const sync = { online: false, reads: 0, refreshes: 0, disconnects: 0,
        async reconnect() { this.reads++; this.online = true; },
        async refresh() { this.refreshes++; }, disconnect() { this.online = false; this.disconnects++; } };
    const context = vm.createContext({ document: { addEventListener() {}, getElementById }, Date: Clock,
        window: { location: { protocol: 'http:', host: 'localhost:8080', origin: 'http://localhost:8080' } },
        WebSocket: Socket, AbortController, fetch, sync, console,
        setTimeout(callback, delay) { const id = ++sequence; timers.set(id, { callback, delay, due: now + delay }); return id; },
        clearTimeout(id) { timers.delete(id); } });
    vm.runInContext(readFileSync(join(__dirname, '../web/static/js/retry-policy.js'), 'utf8'), context);
    vm.runInContext(readFileSync(join(__dirname, '../web/static/js/app.js'), 'utf8'), context);
    vm.runInContext("currentBoardId = 'board'; documentSync = sync", context);
    return { context, elements, sockets, sync, timers, run: code => vm.runInContext(code, context),
        async tick(ms) {
            now += ms;
            const due = [...timers].filter(([, timer]) => timer.due <= now);
            for (const [id, timer] of due) { if (timers.delete(id)) timer.callback(); }
            await Promise.resolve();
        } };
}

test('a 429 pauses HTTP and manual WebSocket retries until the server window ends', async () => {
    let fetches = 0;
    const f = fixture(async () => { fetches++; return { ok: false, status: 429,
        headers: { get: key => key === 'Retry-After' ? '20' : null } }; });
    await f.run("fetchBoardResult('/api/boards/board')");
    assert.equal(fetches, 1);
    await assert.rejects(f.run("fetchBoardResult('/api/boards/board')"), /подождать/);
    f.run("connectWebSocket('board')");
    assert.equal(f.sockets.length, 0); assert.equal(f.elements.get('retry-save').disabled, true);
    await f.tick(19000); assert.equal(f.sockets.length, 0);
    await f.tick(1000); assert.equal(f.sockets.length, 1);
    assert.equal(fetches, 1); assert.equal(f.elements.get('retry-save').disabled, true, 'CONNECTING still disables the retry action');
    await f.sockets[0].open(); assert.equal(f.elements.get('retry-save').disabled, false);
});

test('long Retry-After values use bounded timers without opening an early connection', () => {
    const f = fixture(); f.run("retryPolicy.hold('86400'); connectWebSocket('board')");
    assert.equal(f.sockets.length, 0);
    assert.ok([...f.timers.values()].every(timer => timer.delay <= 60000));
});

test('a stalled socket times out, retries once, and ignores late events from the old socket', async () => {
    const f = fixture(); f.run("connectWebSocket('board')");
    const old = f.sockets[0];
    await f.tick(10000); assert.equal(f.sync.disconnects, 1); assert.equal(old.readyState, 3);
    await f.tick(1200); assert.equal(f.sockets.length, 2);
    await old.open(); assert.equal(f.sync.reads, 0);
    await f.sockets[1].open(); assert.equal(f.sync.reads, 1);
    await f.tick(10000); assert.equal(f.sockets.length, 2); assert.equal(f.sync.online, true);
});

test('visibility/online recovery refreshes an open socket and respects cooldowns and terminal errors', async () => {
    const f = fixture(); f.run("connectWebSocket('board')"); await f.sockets[0].open();
    f.run('resumeConnection()'); assert.equal(f.sync.refreshes, 1);
    f.run("retryPolicy.hold('20'); resumeConnection()"); await f.tick(20000);
    assert.equal(f.sockets.length, 1, 'a cooldown must not replace a healthy socket');
    f.run('retryBlocked = true; ws.close(); resumeConnection()'); await f.tick(60000);
    assert.equal(f.sockets.length, 1);
});

test('an in-progress authoritative read is not restarted by a visibility event', async () => {
    const f = fixture(); let finish;
    f.sync.reconnect = () => new Promise(resolve => { finish = resolve; });
    f.run("connectWebSocket('board')"); const opening = f.sockets[0].open();
    f.run('resumeConnection()'); assert.equal(f.sockets.length, 1);
    finish(); await opening;
});

test('failed clipboard access exposes a selectable link without a modal alert', async () => {
    const f = fixture(); f.context.navigator = {};
    await f.run('shareBoard()');
    assert.equal(f.elements.get('notice-link').hidden, false);
    assert.equal(f.elements.get('notice-link').value, 'http://localhost:8080/#board=board');
    assert.match(f.elements.get('notice-text').textContent, /скопируйте ссылку/);
});

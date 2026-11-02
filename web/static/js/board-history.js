(function (root) {
    'use strict';
    const key = 'collablearn:recent:v1';
    const validID = value => typeof value === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value);
    const cachedTitle = value => typeof value === 'string' && [...value].length <= 100 && !/[\u0000-\u001f\u007f-\u009f]/u.test(value) && value ? { title: value } : {};

    class BoardHistory {
        constructor(storage) { this.storage = storage; this.available = !!storage; }
        list() {
            try {
                if (!this.storage) return [];
                const raw = this.storage.getItem(key);
                if (!raw) return [];
                if (raw.length > 16384) throw new Error('History too large');
                const entries = JSON.parse(raw);
                if (!Array.isArray(entries)) return [];
                const seen = new Set();
                return entries.filter(entry => {
                    if (!entry || !validID(entry.id) || !Number.isFinite(entry.openedAt) || seen.has(entry.id.toLowerCase())) return false;
                    seen.add(entry.id.toLowerCase()); return true;
                }).slice(0, 20).map(entry => ({ id: entry.id.toLowerCase(), openedAt: entry.openedAt, ...cachedTitle(entry.title) }));
            } catch (_) { this.available = false; return []; }
        }
        remember(id, now = Date.now(), title) {
            if (!validID(id)) return false;
            const previous = this.list();
            const label = title === undefined ? previous.find(entry => entry.id === id.toLowerCase())?.title : title;
            const entries = [{ id: id.toLowerCase(), openedAt: now, ...cachedTitle(label) }, ...previous.filter(entry => entry.id !== id.toLowerCase())].slice(0, 20);
            try { this.storage.setItem(key, JSON.stringify(entries)); this.available = true; return true; }
            catch (_) { this.available = false; return false; }
        }
        updateTitle(id, title) {
            if (!validID(id)) return false;
            const previous = this.list(), current = previous.find(entry => entry.id === id.toLowerCase());
            const label = cachedTitle(title);
            if (!current || current.title === label.title) return this.available;
            const entries = previous.map(entry => entry.id === id.toLowerCase() ?
                { id: entry.id, openedAt: entry.openedAt, ...label } : entry);
            try { this.storage.setItem(key, JSON.stringify(entries)); this.available = true; return true; }
            catch (_) { this.available = false; return false; }
        }
        clear() {
            try { this.storage.removeItem(key); return true; }
            catch (_) { this.available = false; return false; }
        }
    }
    if (typeof module !== 'undefined' && module.exports) module.exports = { BoardHistory };
    else root.CollabBoardHistory = BoardHistory;
})(typeof globalThis !== 'undefined' ? globalThis : this);

(function (root) {
    'use strict';
    const key = 'collablearn:recent:v1';
    const validID = value => typeof value === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value);

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
                }).slice(0, 20).map(entry => ({ id: entry.id.toLowerCase(), openedAt: entry.openedAt }));
            } catch (_) { this.available = false; return []; }
        }
        remember(id, now = Date.now()) {
            if (!validID(id)) return false;
            const entries = [{ id: id.toLowerCase(), openedAt: now }, ...this.list().filter(entry => entry.id !== id.toLowerCase())].slice(0, 20);
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

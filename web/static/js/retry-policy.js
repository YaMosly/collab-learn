(function (root) {
    'use strict';

    function retryAfterMs(value, now = Date.now(), serverDate = null) {
        if (typeof value !== 'string' || !value.trim()) return 0;
        value = value.trim();
        if (/^\d+$/.test(value)) {
            const ms = Number(value) * 1000;
            return Number.isSafeInteger(ms) && ms <= Number.MAX_SAFE_INTEGER - now ? ms : 0;
        }
        // HTTP-date, not Date.parse's permissive numeric/date shorthand.
        if (!/^(Mon|Tue|Wed|Thu|Fri|Sat|Sun), \d{2} [A-Z][a-z]{2} \d{4} \d{2}:\d{2}:\d{2} GMT$/.test(value)) return 0;
        const until = Date.parse(value);
        const serverNow = typeof serverDate === 'string' && /^(Mon|Tue|Wed|Thu|Fri|Sat|Sun), \d{2} [A-Z][a-z]{2} \d{4} \d{2}:\d{2}:\d{2} GMT$/.test(serverDate)
            ? Date.parse(serverDate) : NaN;
        return Number.isFinite(until) ? Math.max(0, until - (Number.isFinite(serverNow) ? serverNow : now)) : 0;
    }

    class RetryPolicy {
        constructor({ now = () => Date.now(), random = () => Math.random() } = {}) {
            this.now = now; this.random = random; this.attempts = 0; this.until = 0;
        }
        hold(value, serverDate) {
            this.until = Math.max(this.until, this.now() + retryAfterMs(value, this.now(), serverDate));
            return this.remaining();
        }
        remaining() { return Math.max(0, this.until - this.now()); }
        nextDelay() {
            const delay = Math.min(30000, 1000 * 2 ** Math.min(this.attempts++, 5));
            return Math.max(this.remaining(), delay + this.random() * delay * .2);
        }
        reset() { this.attempts = 0; } // A successful socket never cancels an HTTP cooldown.
    }

    if (typeof module !== 'undefined' && module.exports) module.exports = { RetryPolicy, retryAfterMs };
    else root.CollabRetryPolicy = RetryPolicy;
})(typeof globalThis !== 'undefined' ? globalThis : this);

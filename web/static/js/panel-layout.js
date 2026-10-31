(function (global) {
    'use strict';
    const names = ['html', 'css', 'preview'];

    function fitShares(shares, minimum) {
        const free = Math.max(0, 1 - 3 * minimum);
        const excess = shares.map(value => Math.max(0, value - minimum));
        const total = excess.reduce((a, b) => a + b, 0);
        return excess.map(value => minimum + (total ? free * value / total : free / 3));
    }

    function resizePair(shares, index, delta, minimum) {
        const next = [...shares], pair = shares[index] + shares[index + 1];
        next[index] = Math.max(minimum, Math.min(pair - minimum, shares[index] + delta));
        next[index + 1] = pair - next[index];
        return next;
    }

    class PanelLayout {
        constructor({ root, editors, window = root.ownerDocument.defaultView }) {
            this.root = root; this.editors = editors; this.window = window;
            this.document = root.ownerDocument;
            this.grid = root.querySelector('#editor-panels');
            this.tabs = names.map(name => root.querySelector(`#${name}-tab`));
            this.panels = names.map(name => root.querySelector(`#${name}-panel`));
            this.handles = ['html', 'css'].map(name => root.querySelector(`#${name}-splitter`));
            this.tablist = root.querySelector('#editor-tabs');
            this.help = root.querySelector('#splitter-help');
            this.media = window.matchMedia('(max-width: 1024px)');
            this.active = 0; this.shares = [1 / 3, 1 / 3, 1 / 3];
            this.listeners = []; this.frame = null; this.drag = null; this.destroyed = false;
            this.tabs.forEach((tab, index) => {
                this.listen(tab, 'click', () => this.select(index));
                this.listen(tab, 'keydown', event => {
                    if (event.altKey || event.ctrlKey || event.metaKey) return;
                    let next;
                    if (event.key === 'ArrowRight') next = (index + 1) % 3;
                    if (event.key === 'ArrowLeft') next = (index + 2) % 3;
                    if (event.key === 'Home') next = 0;
                    if (event.key === 'End') next = 2;
                    if (next !== undefined) { event.preventDefault(); this.select(next, true); }
                });
            });
            this.handles.forEach((handle, index) => {
                this.listen(handle, 'keydown', event => {
                    if (event.altKey || event.ctrlKey || event.metaKey) return;
                    if (this.media.matches || !['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
                    event.preventDefault();
                    const delta = event.key === 'Home' ? -1 : event.key === 'End' ? 1 :
                        (event.key === 'ArrowLeft' ? -1 : 1) * (event.shiftKey ? .1 : .02);
                    this.shares = resizePair(this.shares, index, delta, this.minimum()); this.paint();
                });
                this.listen(handle, 'dblclick', () => { this.shares = [1 / 3, 1 / 3, 1 / 3]; this.paint(); });
                this.listen(handle, 'pointerdown', event => {
                    if (this.media.matches || event.button !== 0 || this.drag || event.isPrimary === false) return;
                    event.preventDefault(); handle.focus();
                    this.drag = { index, id: event.pointerId, x: event.clientX, shares: [...this.shares], width: this.available() };
                    handle.setPointerCapture(event.pointerId); this.grid.classList.add('is-resizing');
                });
                this.listen(handle, 'pointermove', event => {
                    const drag = this.drag;
                    if (!drag || drag.id !== event.pointerId || drag.index !== index) return;
                    this.shares = resizePair(drag.shares, index, (event.clientX - drag.x) / drag.width, this.minimum());
                    this.paint();
                });
                for (const type of ['pointerup', 'pointercancel', 'lostpointercapture']) {
                    this.listen(handle, type, event => { if (this.drag?.id === event.pointerId) this.cancelDrag(); });
                }
            });
            this.listen(this.media, 'change', () => this.applyMode());
            this.listen(window, 'resize', () => this.resize());
            if (window.ResizeObserver) {
                this.observer = new window.ResizeObserver(() => this.resize()); this.observer.observe(this.grid);
            }
            this.applyMode();
        }

        listen(target, type, callback) {
            target.addEventListener(type, callback); this.listeners.push([target, type, callback]);
        }
        available() {
            return Math.max(1, this.grid.getBoundingClientRect().width - this.handles.reduce((total, handle) => total + handle.getBoundingClientRect().width, 0));
        }
        minimum() { return Math.min(1 / 3, 240 / this.available()); }

        applyMode() {
            this.cancelDrag();
            const focused = this.document.activeElement;
            if (this.media.matches) {
                const index = this.panels.findIndex(panel => panel.contains(focused));
                if (index !== -1) this.active = index;
            }
            this.tablist.hidden = !this.media.matches;
            this.help.hidden = this.media.matches;
            this.handles.forEach(handle => { handle.hidden = this.media.matches; });
            this.select(this.active);
            if (this.handles.includes(focused) && this.media.matches) this.tabs[this.active].focus();
            if (this.tabs.includes(focused) && !this.media.matches) this.panels[this.active].focus();
            this.resize();
        }

        select(index, focus = false) {
            this.active = index;
            this.tabs.forEach((tab, i) => { tab.setAttribute('aria-selected', String(i === index)); tab.tabIndex = i === index ? 0 : -1; });
            this.panels.forEach((panel, i) => {
                panel.hidden = this.media.matches && i !== index;
                panel.setAttribute('role', this.media.matches ? 'tabpanel' : 'region');
                panel.setAttribute('aria-labelledby', `${names[i]}-${this.media.matches ? 'tab' : 'heading'}`);
                panel.tabIndex = this.media.matches ? 0 : -1;
            });
            if (focus) this.tabs[index].focus();
            this.refresh();
        }

        resize() {
            this.cancelDrag();
            if (!this.media.matches) { this.shares = fitShares(this.shares, this.minimum()); this.paint(); }
            else this.refresh();
        }

        paint() {
            this.grid.style.gridTemplateColumns = this.shares.map(value => `minmax(0, ${value}fr)`).join(' 10px ');
            const minimum = this.minimum();
            this.handles.forEach((handle, i) => {
                handle.setAttribute('aria-valuemin', String(Math.round(minimum * 100)));
                handle.setAttribute('aria-valuemax', String(Math.round((this.shares[i] + this.shares[i + 1] - minimum) * 100)));
                handle.setAttribute('aria-valuenow', String(Math.round(this.shares[i] * 100)));
                handle.setAttribute('aria-valuetext', `${names[i].toUpperCase()}: ${Math.round(this.shares[i] * 100)}%, ${names[i + 1].toUpperCase()}: ${Math.round(this.shares[i + 1] * 100)}%`);
            });
            this.refresh();
        }

        refresh() {
            if (this.frame !== null || this.destroyed) return;
            this.frame = this.window.requestAnimationFrame(() => {
                this.frame = null;
                if (this.destroyed) return;
                for (const [name, editor] of Object.entries(this.editors)) {
                    if (this.panels[names.indexOf(name)].hidden) continue;
                    const scroll = editor.getScrollInfo(); editor.refresh(); editor.scrollTo(scroll.left, scroll.top);
                }
            });
        }

        cancelDrag() {
            const drag = this.drag; this.drag = null; this.grid.classList.remove('is-resizing');
            if (drag && this.handles[drag.index].hasPointerCapture(drag.id)) this.handles[drag.index].releasePointerCapture(drag.id);
        }

        destroy() {
            this.destroyed = true; this.cancelDrag(); this.observer?.disconnect();
            if (this.frame !== null) this.window.cancelAnimationFrame(this.frame);
            for (const [target, type, callback] of this.listeners) target.removeEventListener(type, callback);
        }
    }

    if (typeof module !== 'undefined' && module.exports) module.exports = { PanelLayout, resizePair, fitShares };
    else global.CollabPanelLayout = PanelLayout;
})(typeof globalThis !== 'undefined' ? globalThis : this);

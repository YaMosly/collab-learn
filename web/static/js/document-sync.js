// Versioned whole-document saves. Different fields merge; same-field conflicts
// keep the local text and require an explicit choice instead of last-writer-wins.
(function (root) {
    const fields = ['code_html', 'code_css'];
    const copy = value => ({ code_html: value.code_html, code_css: value.code_css });
    const equal = (a, b) => fields.every(field => a[field] === b[field]);
    const valid = value => value && Number.isSafeInteger(value.revision) && value.revision >= 0 &&
        fields.every(field => typeof value[field] === 'string');

    class DocumentSync {
        constructor(board, options) {
            if (!valid(board)) throw new Error('Invalid document revision; reload the editor');
            this.options = options;
            this.base = { ...board };
            this.local = copy(board);
            this.conflicts = new Set();
            this.online = false;
            this.inFlight = null;
            this.uncertain = null;
            this.deferred = null;
            this.generation = 0;
            this.error = '';
            this.draftWarning = '';
            try {
                const draft = options.storage?.getItem(options.key);
                if (draft) {
                    const saved = JSON.parse(draft);
                    if (!valid(saved.base) || !fields.every(field => typeof saved.local?.[field] === 'string')) {
                        throw new Error('Invalid draft');
                    }
                    this.base = saved.base;
                    this.local = copy(saved.local);
                    if (saved.uncertain && Number.isSafeInteger(saved.uncertain.base_revision) &&
                        fields.every(field => typeof saved.uncertain[field] === 'string')) this.uncertain = saved.uncertain;
                    this.conflicts = new Set((saved.conflicts || []).filter(field => fields.includes(field)));
                    if (board.revision < this.base.revision) {
                        this.base = { ...board };
                        fields.filter(field => this.local[field] !== board[field]).forEach(field => this.conflicts.add(field));
                    }
                    // The initial GET is authoritative; never restore an older
                    // server snapshot over edits retained in this tab's draft.
                    this.receive(board);
                }
            } catch (_) {
                this.draftWarning = 'Не удалось восстановить черновик этой вкладки';
            }
        }

        get dirty() { return !!this.uncertain || !equal(this.local, this.base); }

        state() {
            return { local: copy(this.local), remote: copy(this.base), revision: this.base.revision,
                conflicts: [...this.conflicts], online: this.online, busy: !!this.inFlight,
                dirty: this.dirty, error: this.error, draftWarning: this.draftWarning };
        }

        notify() {
            try {
                if (!this.options.storage) throw new Error('Storage unavailable');
                if (this.dirty || this.inFlight || this.conflicts.size) {
                    this.options.storage.setItem(this.options.key, JSON.stringify({
                        base: this.base, local: this.local, conflicts: [...this.conflicts],
                        // A lost response does not prove the write failed.
                        uncertain: this.inFlight || this.uncertain,
                    }));
                } else {
                    this.options.storage.removeItem(this.options.key);
                }
            } catch (_) {
                this.draftWarning = 'Черновик недоступен: не закрывайте вкладку до сохранения';
            }
            this.options.changed?.(this.state());
        }

        edit(document) {
            this.local = copy(document);
            for (const field of this.conflicts) {
                if (this.local[field] === this.base[field]) this.conflicts.delete(field);
            }
            // Keep the retry action visible after a terminal HTTP error until
            // the user reconnects; typing alone does not restore transport.
            if (this.online) this.error = '';
            this.notify();
        }

        receive(board) {
            if (!valid(board)) throw new Error('Invalid server document');
            if (board.revision <= this.base.revision) return;
            if (this.inFlight) {
                if (!this.deferred || board.revision > this.deferred.revision) this.deferred = board;
                return;
            }
            const reference = { ...this.base };
            for (const field of fields) {
                if (this.uncertain && board[field] === this.uncertain[field]) reference[field] = this.uncertain[field];
            }
            for (const field of fields) {
                const undoWhileSaving = this.uncertain && this.local[field] !== this.uncertain[field];
                if (this.local[field] === reference[field] && !undoWhileSaving) {
                    this.local[field] = board[field];
                } else if (board[field] !== reference[field] && this.local[field] !== board[field]) {
                    this.conflicts.add(field);
                }
                if (this.local[field] === board[field]) this.conflicts.delete(field);
            }
            this.base = { ...board };
            this.uncertain = null;
            this.notify();
        }

        disconnect() {
            ++this.generation;
            this.online = false;
            this.notify();
        }

        async reconnect() {
            const generation = ++this.generation;
            this.online = false;
            this.notify();
            try {
                const board = await this.options.read();
                if (generation !== this.generation) return;
                this.receive(board);
                this.online = true;
                this.error = '';
                this.notify();
                return this.flush();
            } catch (error) {
                if (generation !== this.generation) return;
                this.error = error.message || 'Нет связи с сервером';
                this.notify();
                this.options.failed?.(error);
            }
        }

        async refresh() {
            if (!this.online || this.refreshing) return;
            this.refreshing = true;
            const generation = this.generation;
            try {
                const board = await this.options.read();
                if (generation !== this.generation || !this.online) return;
                this.receive(board);
                return await this.flush();
            } catch (error) {
                if (generation !== this.generation) return;
                this.online = false;
                this.error = error.message || 'Нет связи с сервером';
                this.notify();
                this.options.failed?.(error);
            } finally { this.refreshing = false; }
        }

        async flush(attempt = 0) {
            if (!this.online || this.inFlight || !this.dirty || this.conflicts.size) return;
            const request = { base_revision: this.base.revision, ...copy(this.local) };
            this.inFlight = request;
            this.notify();
            let retry = false;
            try {
                const result = await this.options.write(request);
                if (!valid(result.board)) throw new Error('Invalid save response');
                if (result.conflict) {
                    if (result.board.revision <= request.base_revision || attempt >= 5) {
                        const error = new Error('Версия сервера изменилась: повторите соединение');
                        error.permanent = true;
                        throw error;
                    }
                    // Merge only after inFlight has been cleared below.
                    if (!this.deferred || result.board.revision >= this.deferred.revision) this.deferred = result.board;
                    retry = true;
                } else {
                    if (result.board.revision !== request.base_revision + 1 || !equal(result.board, request)) {
                        throw new Error('Сервер не подтвердил отправленную версию');
                    }
                    // An acknowledgement covers the submitted snapshot, not
                    // keystrokes made while this request was on the network.
                    if (result.board.revision > this.base.revision) {
                        for (const field of fields) {
                            if (this.local[field] === request[field]) this.local[field] = result.board[field];
                        }
                        this.base = { ...result.board };
                    }
                    this.error = '';
                    this.uncertain = null;
                    retry = true;
                }
            } catch (error) {
                this.uncertain = request;
                this.online = false;
                this.error = error.message || 'Сохранение не подтверждено';
                this.options.failed?.(error);
            } finally {
                this.inFlight = null;
                if (this.deferred) {
                    const board = this.deferred;
                    this.deferred = null;
                    this.receive(board);
                }
                this.notify();
            }
            if (retry) return this.flush(attempt + 1);
        }

        resolve(choice) {
            if (choice !== 'local' && choice !== 'server') throw new Error('Invalid conflict choice');
            if (choice === 'server') {
                for (const field of this.conflicts) this.local[field] = this.base[field];
            }
            this.conflicts.clear();
            this.error = '';
            this.notify();
            return this.flush();
        }
    }

    if (typeof module !== 'undefined' && module.exports) module.exports = { DocumentSync };
    else root.CollabDocumentSync = DocumentSync;
})(globalThis);

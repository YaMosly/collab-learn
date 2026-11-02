(function (root) {
    'use strict';
    const valid = board => board && typeof board.title === 'string' &&
        Number.isSafeInteger(board.title_revision) && board.title_revision >= 0;
    const normalize = value => value.replace(/^\p{White_Space}+|\p{White_Space}+$/gu, '');
    class BoardTitle {
        constructor(board, options) {
            if (!valid(board)) throw new Error('Некорректное название доски');
            this.options = options; this.title = board.title; this.revision = board.title_revision;
            this.input = board.title; this.busy = false; this.error = '';
        }
        get dirty() { return normalize(this.input) !== this.title; }
        notify() { this.options.changed?.(this); }
        edit(value) { this.input = value; this.error = ''; this.notify(); }
        receive(board) {
            if (!valid(board) || board.title_revision <= this.revision) return;
            const preserve = this.dirty || this.busy;
            this.title = board.title; this.revision = board.title_revision;
            if (!preserve) this.input = board.title;
            else if (!this.busy) this.error = 'Название уже изменили. Проверьте текущую версию перед сохранением.';
            this.notify();
        }
        reset() { if (!this.busy) { this.input = this.title; this.error = ''; this.notify(); } }
        async save() {
            if (this.busy) return;
            const title = normalize(this.input);
            if ([...title].length > 100 || /[\u0000-\u001f\u007f-\u009f]/u.test(title)) {
                this.error = 'До 100 символов, без управляющих знаков.'; this.notify(); return;
            }
            this.busy = true; this.error = ''; this.notify();
            try {
                const base = this.revision;
                const result = await this.options.write({ title, base_title_revision: base });
                if (!valid(result.board) || (!result.conflict && result.board.title_revision !== base + 1)) throw new Error('Сервер не подтвердил название.');
                this.receive(result.board);
                if (result.conflict && result.board.title !== title) {
                    this.error = 'Название уже изменили. Текущее показано выше. Повторите сохранение или отмените свою правку.';
                } else if (result.board.title !== title) {
                    throw new Error('Сервер не подтвердил название.');
                } else if (this.title === title) {
                    this.input = title;
                } else {
                    this.error = 'Название снова изменили. Проверьте текущую версию.';
                }
            } catch (error) {
                this.error = `Название не подтверждено. ${error.message || 'Повторите позже.'}`;
            } finally { this.busy = false; this.notify(); }
        }
    }
    if (typeof module !== 'undefined' && module.exports) module.exports = { BoardTitle };
    else root.CollabBoardTitle = BoardTitle;
})(globalThis);

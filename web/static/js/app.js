let ws = null;
let currentBoardId = null;
let htmlEditor = null;
let cssEditor = null;
let isUpdatingFromServer = false;
let documentSync = null;
let reconnectTimer = null;
const retryPolicy = new CollabRetryPolicy();
let socketTimer = null;
let cooldownTimer = null;
let creatingBoard = false;
let openingBoard = false;
let noticeRetry = null;
let noticeRetryLabel = 'Повторить';
let retryBlocked = false;
let boardHistory = null;
let navigationConfirmed = false;

document.addEventListener('DOMContentLoaded', () => {
    initializeApp();
});

function initializeApp() {
    let storage;
    try { storage = window.localStorage; } catch (_) { storage = null; }
    boardHistory = new CollabBoardHistory(storage);
    document.getElementById('create-board').addEventListener('click', createNewBoard);
    document.getElementById('refresh-preview').addEventListener('click', updatePreview);
    document.getElementById('share-btn').addEventListener('click', shareBoard);
    document.getElementById('export-html').addEventListener('click', () => exportSource('html'));
    document.getElementById('export-css').addEventListener('click', () => exportSource('css'));
    document.getElementById('clear-history').addEventListener('click', () => { boardHistory.clear(); loadBoardsList(); });
    document.getElementById('notice-retry').addEventListener('click', () => { if (!retryPolicy.remaining()) noticeRetry?.(); });
    document.getElementById('notice-dismiss').addEventListener('click', clearNotice);
    document.getElementById('keep-local').addEventListener('click', () => documentSync?.resolve('local'));
    document.getElementById('use-server').addEventListener('click', () => documentSync?.resolve('server'));
    document.getElementById('retry-save').addEventListener('click', () => {
        retryBlocked = false;
        connectWebSocket(currentBoardId);
    });
    window.addEventListener('beforeunload', event => {
        if (!navigationConfirmed && (documentSync?.dirty || documentSync?.inFlight)) { event.preventDefault(); event.returnValue = ''; }
    });
    window.addEventListener('hashchange', handleHashNavigation);
    window.addEventListener('online', resumeConnection);
    document.addEventListener('visibilitychange', () => { if (!document.hidden) resumeConnection(); });

    const boardId = getBoardIdFromURL();
    if (boardId) {
        loadBoard(boardId);
    } else {
        loadBoardsList();
    }
}

function getBoardIdFromURL() {
    const fragment = new URLSearchParams(window.location.hash.slice(1)).get('board');
    const legacy = new URLSearchParams(window.location.search).get('board');
    const id = fragment || legacy;
    if (legacy && id) window.history.replaceState(null, '', `/#board=${encodeURIComponent(id)}`);
    return id;
}

function handleHashNavigation() {
    if (getBoardIdFromURL() === currentBoardId) return;
    if ((documentSync?.dirty || documentSync?.inFlight) &&
        !window.confirm('Есть неподтверждённые изменения. Черновик останется в этой вкладке. Перейти на другую доску?')) {
        window.history.replaceState(null, '', `/#board=${encodeURIComponent(currentBoardId)}`);
        return;
    }
    navigationConfirmed = true;
    window.location.reload();
}

async function createNewBoard() {
    if (creatingBoard || openingBoard) return;
    creatingBoard = true;
    showNotice('Создаём доску…', { kind: 'info' });
    updateRetryControls();
    try {
        const board = await fetchBoardJSON('/api/boards', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' }
        });
        boardHistory.remember(board.id);
        // A document navigation also works when another board is already open;
        // initializeApp converts the compatibility query to a shareable fragment.
        window.location.href = `/?board=${encodeURIComponent(board.id)}`;
    } catch (error) {
        showNotice(`Создание доски не подтверждено. ${error.message}`, { retry: createNewBoard, label: 'Создать ещё раз' });
    } finally { creatingBoard = false; updateRetryControls(); }
}

function loadBoardsList() {
    const boards = boardHistory.list();
    document.getElementById('board-message').textContent = boardHistory.available ? '' :
        'История браузера недоступна. Сохраните ссылку на доску самостоятельно.';
    const container = document.getElementById('boards-container');
    container.innerHTML = '';
    if (boards.length === 0) {
        container.textContent = 'Здесь появятся доски, которые вы откроете. Создайте новую или перейдите по ссылке.';
        return;
    }
    boards.forEach(board => container.appendChild(createBoardElement(board)));
}

function createBoardElement(board) {
    const div = document.createElement('div');
    div.className = 'board-item';
    const title = document.createElement('h3');
    title.textContent = `Board ${board.id.substring(0, 8)}`;
    const opened = document.createElement('p');
    opened.textContent = `Открыта: ${new Date(board.openedAt).toLocaleString()}`;
    div.append(title, opened);
    div.addEventListener('click', () => {
        window.location.href = `/?board=${encodeURIComponent(board.id)}`;
    });
    return div;
}

async function loadBoard(boardId) {
    if (openingBoard || currentBoardId) return;
    openingBoard = true;
    showNotice('Открываем доску…', { kind: 'info' });
    updateRetryControls();
    try {
        const board = await fetchBoardJSON(`/api/boards/${encodeURIComponent(boardId)}`);
        if (typeof CodeMirror === 'undefined') {
            const error = new Error('Редактор не загрузился. Проверьте подключение к сети и перезагрузите страницу.');
            error.reload = true; throw error;
        }
        if (!board || !Number.isSafeInteger(board.revision) || board.revision < 0 ||
            typeof board.code_html !== 'string' || typeof board.code_css !== 'string') throw new Error('Сервер вернул некорректный документ.');
        currentBoardId = boardId;
        boardHistory.remember(boardId);
        
        document.getElementById('boards-list').style.display = 'none';
        document.getElementById('editor-container').style.display = 'grid';
        document.getElementById('sync-status').hidden = false;
        
        initializeEditors(board);
        clearNotice();
        connectWebSocket(boardId);
        updatePreview();
    } catch (error) {
        currentBoardId = null;
        showNotice(`Не удалось открыть доску. ${error.message} Ссылка остаётся в адресной строке.`,
            { retry: error.reload ? () => window.location.reload() : error.permanent ? null : () => loadBoard(boardId),
                label: error.reload ? 'Перезагрузить страницу' : 'Повторить' });
        loadBoardsList();
    } finally { openingBoard = false; updateRetryControls(); }
}

function initializeEditors(board) {
    const editorConfig = {
        theme: 'material-ocean',
        lineNumbers: true,
        lineWrapping: true,
        autoCloseBrackets: true,
        autoCloseTags: true,
        matchBrackets: true,
        styleActiveLine: true,
        extraKeys: {
            'Ctrl-Space': 'autocomplete',
            'Cmd-Space': 'autocomplete',
            'Ctrl-/': 'toggleComment',
            'Cmd-/': 'toggleComment',
            'Tab': function(cm) {
                if (cm.somethingSelected()) {
                    cm.indentSelection('add');
                } else {
                    cm.replaceSelection('  ', 'end');
                }
            }
        },
        foldGutter: true,
        gutters: ['CodeMirror-linenumbers', 'CodeMirror-foldgutter'],
        matchTags: {bothTags: true},
        hintOptions: {
            completeSingle: false,
            closeCharacters: /[\s()\[\]{};:>,]/,
            closeOnUnfocus: true
        }
    };
    
    htmlEditor = CodeMirror.fromTextArea(document.getElementById('html-editor'), {
        ...editorConfig,
        mode: 'htmlmixed'
    });
    
    cssEditor = CodeMirror.fromTextArea(document.getElementById('css-editor'), {
        ...editorConfig,
        mode: 'css'
    });
    
    htmlEditor.setValue(board.code_html || '');
    cssEditor.setValue(board.code_css || '');
    
    let storage;
    try { storage = window.sessionStorage; } catch (_) { storage = null; }
    documentSync = new CollabDocumentSync(board, {
        storage, key: `collablearn:draft:${currentBoardId}`,
        read: () => fetchBoardJSON(`/api/boards/${encodeURIComponent(currentBoardId)}`),
        write: async update => {
            const response = await fetchBoardResult(`/api/boards/${encodeURIComponent(currentBoardId)}`, {
                method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(update),
            });
            if (!response.ok && response.status !== 409) {
                throw responseError(response);
            }
            return { conflict: response.status === 409, board: response.board };
        },
        changed: renderDocumentState,
        failed: error => { retryBlocked = !!error.permanent; ws?.close(); updateRetryControls(); },
    });
    documentSync.notify();

    let updateTimeout;
    const handleChange = (editor) => {
        if (isUpdatingFromServer) return;
        documentSync.edit({ code_html: htmlEditor.getValue(), code_css: cssEditor.getValue() });
        
        clearTimeout(updateTimeout);
        updateTimeout = setTimeout(() => {
            sendUpdate();
            updatePreview();
        }, 500);
    };
    
    htmlEditor.on('change', () => handleChange(htmlEditor));
    cssEditor.on('change', () => handleChange(cssEditor));
}

function connectWebSocket(boardId) {
    clearTimeout(reconnectTimer);
    if (!boardId || !documentSync || retryBlocked) return;
    if (retryPolicy.remaining()) { scheduleReconnect(boardId, retryPolicy.remaining()); return; }
    clearTimeout(socketTimer);
    if (ws) { ws.onclose = null; ws.close(); }
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/api/boards/${encodeURIComponent(boardId)}/ws`;
    
    updateConnectionStatus(false);
    const socket = new WebSocket(wsUrl);
    ws = socket;
    // Some proxies leave CONNECTING pending without an error event.
    socketTimer = setTimeout(() => {
        if (ws !== socket) return;
        ws = null;
        socket.onclose = null;
        socket.close();
        documentSync.disconnect();
        updateConnectionStatus(false);
        updateUserCount(0, false);
        scheduleReconnect(boardId);
    }, 10000);
    
    socket.onopen = async () => {
        if (ws !== socket) return;
        clearTimeout(socketTimer);
        updateUserCount(0, false);
        socket.synchronizing = true;
        try { await documentSync.reconnect(); } finally { socket.synchronizing = false; updateRetryControls(); }
        if (ws !== socket) return;
        updateConnectionStatus(documentSync.online);
        if (documentSync.online) retryPolicy.reset();
    };
    
    socket.onclose = () => {
        if (ws !== socket) return;
        clearTimeout(socketTimer);
        ws = null;
        updateConnectionStatus(false);
        updateUserCount(0, false);
        documentSync.disconnect();
        if (!retryBlocked) scheduleReconnect(boardId);
    };
    
    socket.onmessage = (event) => {
        if (ws !== socket) return;
        try { handleWebSocketMessage(JSON.parse(event.data)); }
        catch (error) { console.error('Invalid update:', error); socket.close(); }
    };
    
    socket.onerror = (error) => {
        console.error('WebSocket error:', error);
    };
    updateRetryControls();
}

function scheduleReconnect(boardId, delay = retryPolicy.nextDelay()) {
    clearTimeout(reconnectTimer);
    // Long Retry-After values are checked in chunks to avoid timer overflow.
    reconnectTimer = setTimeout(() => {
        if (currentBoardId === boardId && !retryBlocked) connectWebSocket(boardId);
    }, Math.min(delay, 60000));
    updateRetryControls();
}

function resumeConnection() {
    if (!currentBoardId || retryBlocked) return;
    if (retryPolicy.remaining()) {
        if (!ws || !documentSync.online) scheduleReconnect(currentBoardId, retryPolicy.remaining());
        return;
    }
    if (ws?.synchronizing) return;
    if (ws?.readyState === WebSocket.OPEN && documentSync.online) documentSync.refresh();
    else if (!ws || ws.readyState !== WebSocket.CONNECTING) connectWebSocket(currentBoardId);
}

function handleWebSocketMessage(message) {
    switch (message.type) {
        case 'update':
            documentSync.receive(message.data);
            break;
            
        case 'connections':
            updateUserCount(message.data.count, message.data.available !== false);
            break;
        case 'resync':
            documentSync.refresh();
            break;
    }
}

function sendUpdate() {
    return documentSync?.flush();
}

async function fetchBoardResult(url, options = {}) {
    if (retryPolicy.remaining()) throw new Error(`Сервер просит подождать ${Math.ceil(retryPolicy.remaining() / 1000)} с.`);
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 15000);
    try {
        const response = await fetch(url, { ...options, cache: 'no-store', signal: controller.signal });
        if (response.status === 429 || response.status === 503) {
            retryPolicy.hold(response.headers?.get('Retry-After'), response.headers?.get('Date'));
            updateRetryControls();
        }
        // Keep the deadline through the body read, not just response headers.
        const board = response.ok || response.status === 409 ? await response.json() : null;
        return { ok: response.ok, status: response.status, board };
    }
    catch (error) {
        if (error.name === 'AbortError') throw new Error('Сервер не ответил за 15 секунд. Повторите позже.');
        if (error instanceof TypeError) throw new Error('Нет связи с сервером. Проверьте подключение к сети.');
        throw error;
    } finally { clearTimeout(timeout); }
}

function permanentStatus(status) { return status >= 400 && status < 500 && status !== 408 && status !== 429; }

function responseError(response) {
    const messages = { 403: 'Доступ запрещён. Проверьте адрес сайта или обратитесь к владельцу сервиса.',
        404: 'Доска не найдена. Проверьте ссылку.', 413: 'Документ больше 512 КБ. Сократите код и повторите сохранение.',
        429: 'Слишком много запросов. Дождитесь окончания паузы и повторите.', 503: 'Сервис временно недоступен. Повторите позже.' };
    const error = new Error(messages[response.status] || `Запрос не подтверждён (${response.status}).`);
    error.permanent = permanentStatus(response.status);
    return error;
}

async function fetchBoardJSON(url, options) {
    const response = await fetchBoardResult(url, options);
    if (!response.ok) {
        throw responseError(response);
    }
    return response.board;
}

function renderDocumentState(state) {
    isUpdatingFromServer = true;
    try {
        for (const [editor, field] of [[htmlEditor, 'code_html'], [cssEditor, 'code_css']]) {
            if (editor.getValue() !== state.local[field]) {
                const cursor = editor.getCursor();
                editor.setValue(state.local[field]);
                editor.setCursor(cursor);
            }
        }
    } finally { isUpdatingFromServer = false; }
    updatePreview();
    const status = document.getElementById('save-status');
    status.textContent = state.error || (state.conflicts.length ? 'Конфликт: обе версии сохранены в этой вкладке' :
        !state.online ? 'Нет связи · черновик в этой вкладке' : state.dirty || state.busy ? 'Сохраняется…' : 'Сохранено');
    document.getElementById('draft-warning').textContent = state.draftWarning;
    const panel = document.getElementById('sync-conflict');
    panel.hidden = state.conflicts.length === 0;
    document.getElementById('server-version').textContent = `HTML\n${state.remote.code_html}\n\nCSS\n${state.remote.code_css}`;
    document.getElementById('retry-save').hidden = !state.error;
    updateRetryControls();
}

function updatePreview() {
    const html = htmlEditor ? htmlEditor.getValue() : '';
    const css = cssEditor ? cssEditor.getValue() : '';
    
    const previewContent = `
        <!DOCTYPE html>
        <html>
        <head>
            <style>${css}</style>
        </head>
        <body>${html}</body>
        </html>
    `;
    
    const preview = document.getElementById('preview-frame');
    // The sandboxed frame renders HTML/CSS without executing board scripts.
    // srcdoc replaces the previous document without retaining object URLs.
    preview.srcdoc = previewContent;
}

function updateConnectionStatus(connected) {
    const status = document.getElementById('connection-status');
    if (connected) {
        status.textContent = 'Connected';
        status.className = 'status connected';
    } else {
        status.textContent = 'Disconnected';
        status.className = 'status disconnected';
    }
}

function updateUserCount(count, available = true) {
    const userCount = document.getElementById('user-count');
    userCount.textContent = available ? `${count} user${count !== 1 ? 's' : ''}` : 'Участники: нет данных';
}

function clearNotice() {
    noticeRetry = null;
    document.getElementById('app-notice').hidden = true;
    document.getElementById('notice-link').value = '';
}

function showNotice(message, { retry = null, label = 'Повторить', kind = 'error', link = '' } = {}) {
    noticeRetry = retry; noticeRetryLabel = label;
    const panel = document.getElementById('app-notice');
    panel.hidden = false; panel.dataset.kind = kind;
    document.getElementById('notice-text').textContent = message;
    const input = document.getElementById('notice-link');
    input.hidden = !link; input.value = link;
    updateRetryControls();
}

function updateRetryControls() {
    clearTimeout(cooldownTimer);
    const remaining = retryPolicy.remaining();
    const label = `Повторить через ${Math.ceil(remaining / 1000)} с`;
    document.getElementById('create-board').disabled = creatingBoard || openingBoard || remaining > 0;
    const retry = document.getElementById('notice-retry');
    retry.hidden = !noticeRetry;
    retry.disabled = creatingBoard || openingBoard || remaining > 0;
    retry.textContent = remaining ? label : noticeRetryLabel;
    const save = document.getElementById('retry-save');
    save.disabled = remaining > 0 || !!ws && (ws.readyState === 0 || ws.synchronizing);
    save.textContent = remaining ? label : 'Повторить соединение';
    if (remaining > 0) cooldownTimer = setTimeout(updateRetryControls, Math.min(1000, remaining));
}

async function shareBoard() {
    if (!currentBoardId) return;
    const url = `${window.location.origin}/#board=${encodeURIComponent(currentBoardId)}`;
    try {
        await navigator.clipboard.writeText(url);
        showNotice('Ссылка скопирована. Любой получивший её может читать и редактировать доску.', { kind: 'success' });
    } catch (_) {
        showNotice('Автоматическое копирование недоступно. Выделите и скопируйте ссылку.', { link: url });
        const input = document.getElementById('notice-link'); input.focus(); input.select();
    }
}

function exportSource(format) {
    const editor = format === 'html' ? htmlEditor : format === 'css' ? cssEditor : null;
    if (!editor) return;
    try {
        const filename = CollabSourceExport.download(format, editor.getValue());
        showNotice(`Локальная версия передана браузеру для скачивания: ${filename}. Проверьте список загрузок.`, { kind: 'success' });
    } catch (_) {
        showNotice('Не удалось начать скачивание. Повторите попытку или скопируйте код из редактора.');
    }
}

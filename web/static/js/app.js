let ws = null;
let currentBoardId = null;
let htmlEditor = null;
let cssEditor = null;
let isUpdatingFromServer = false;
let documentSync = null;
let reconnectTimer = null;
let reconnectAttempts = 0;
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
    document.getElementById('clear-history').addEventListener('click', () => { boardHistory.clear(); loadBoardsList(); });
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
    const button = document.getElementById('create-board');
    button.disabled = true;
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
        alert(`Не удалось создать доску. ${error.message}`);
    } finally { button.disabled = false; }
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
    try {
        const board = await fetchBoardJSON(`/api/boards/${encodeURIComponent(boardId)}`);
        currentBoardId = boardId;
        boardHistory.remember(boardId);
        
        document.getElementById('boards-list').style.display = 'none';
        document.getElementById('editor-container').style.display = 'grid';
        document.getElementById('sync-status').hidden = false;
        
        initializeEditors(board);
        connectWebSocket(boardId);
        updatePreview();
    } catch (error) {
        console.error('Error loading board:', error);
        alert(`Не удалось открыть доску. ${error.message} Ссылка сохранена в адресной строке — можно повторить позже.`);
        loadBoardsList();
    }
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
                const error = new Error(response.status === 413 ? 'Документ больше 512 КБ. Сократите код и повторите сохранение.' :
                    `Сохранение не подтверждено (${response.status})`);
                error.permanent = permanentStatus(response.status);
                throw error;
            }
            return { conflict: response.status === 409, board: response.board };
        },
        changed: renderDocumentState,
        failed: error => { retryBlocked = !!error.permanent; ws?.close(); },
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
    if (ws) { ws.onclose = null; ws.close(); }
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/api/boards/${encodeURIComponent(boardId)}/ws`;
    
    const socket = new WebSocket(wsUrl);
    ws = socket;
    
    socket.onopen = async () => {
        if (ws !== socket) return;
        updateConnectionStatus(true);
        updateUserCount(0, false);
        await documentSync.reconnect();
        if (documentSync.online) reconnectAttempts = 0;
    };
    
    socket.onclose = () => {
        if (ws !== socket) return;
        updateConnectionStatus(false);
        updateUserCount(0, false);
        documentSync.disconnect();
        if (retryBlocked) return;
        const delay = Math.min(30000, 1000 * 2 ** Math.min(reconnectAttempts++, 5));
        reconnectTimer = setTimeout(() => {
            if (currentBoardId === boardId) {
                connectWebSocket(boardId);
            }
        }, delay + Math.random() * delay * .2);
    };
    
    socket.onmessage = (event) => {
        if (ws !== socket) return;
        try { handleWebSocketMessage(JSON.parse(event.data)); }
        catch (error) { console.error('Invalid update:', error); socket.close(); }
    };
    
    socket.onerror = (error) => {
        console.error('WebSocket error:', error);
    };
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
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 15000);
    try {
        const response = await fetch(url, { ...options, cache: 'no-store', signal: controller.signal });
        // Keep the deadline through the body read, not just response headers.
        const board = response.ok || response.status === 409 ? await response.json() : null;
        return { ok: response.ok, status: response.status, board };
    }
    finally { clearTimeout(timeout); }
}

function permanentStatus(status) { return status >= 400 && status < 500 && status !== 408 && status !== 429; }

async function fetchBoardJSON(url, options) {
    const response = await fetchBoardResult(url, options);
    if (!response.ok) {
        const error = new Error(response.status === 429 ? 'Слишком много запросов. Подождите минуту и повторите.' : `Сервер недоступен (${response.status})`);
        error.permanent = permanentStatus(response.status);
        throw error;
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

function shareBoard() {
    const url = window.location.href;
    navigator.clipboard.writeText(url).then(() => {
        alert('Board URL copied to clipboard!');
    }).catch(err => {
        console.error('Error copying to clipboard:', err);
    });
}

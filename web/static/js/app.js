let ws = null;
let currentBoardId = null;
let htmlEditor = null;
let cssEditor = null;
let isUpdatingFromServer = false;
let lastSentHTML = '';
let lastSentCSS = '';

document.addEventListener('DOMContentLoaded', () => {
    initializeApp();
});

function initializeApp() {
    document.getElementById('create-board').addEventListener('click', createNewBoard);
    document.getElementById('refresh-preview').addEventListener('click', updatePreview);
    document.getElementById('share-btn').addEventListener('click', shareBoard);

    const boardId = getBoardIdFromURL();
    if (boardId) {
        loadBoard(boardId);
    } else {
        loadBoardsList();
    }
}

function getBoardIdFromURL() {
    const params = new URLSearchParams(window.location.search);
    return params.get('board');
}

async function createNewBoard() {
    try {
        const response = await fetch('/api/boards', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' }
        });
        const board = await response.json();
        window.location.href = `/?board=${board.id}`;
    } catch (error) {
        console.error('Error creating board:', error);
    }
}

async function loadBoardsList() {
    try {
        const response = await fetch('/api/boards');
        const boards = await response.json();
        
        const container = document.getElementById('boards-container');
        container.innerHTML = '';
        
        if (!boards || boards.length === 0) {
            container.innerHTML = '<p>No boards available. Create a new one!</p>';
            return;
        }
        
        boards.forEach(board => {
            const boardElement = createBoardElement(board);
            container.appendChild(boardElement);
        });
    } catch (error) {
        console.error('Error loading boards:', error);
    }
}

function createBoardElement(board) {
    const div = document.createElement('div');
    div.className = 'board-item';
    div.innerHTML = `
        <h3>Board ${board.id.substring(0, 8)}</h3>
        <p>Updated: ${new Date(board.updated_at).toLocaleString()}</p>
    `;
    div.addEventListener('click', () => {
        window.location.href = `/?board=${board.id}`;
    });
    return div;
}

async function loadBoard(boardId) {
    try {
        const response = await fetch(`/api/boards/${boardId}`);
        if (!response.ok) {
            throw new Error('Board not found');
        }
        
        const board = await response.json();
        currentBoardId = boardId;
        
        document.getElementById('boards-list').style.display = 'none';
        document.getElementById('editor-container').style.display = 'grid';
        
        initializeEditors(board);
        connectWebSocket(boardId);
        updatePreview();
    } catch (error) {
        console.error('Error loading board:', error);
        alert('Board not found');
        window.location.href = '/';
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
    
    let updateTimeout;
    const handleChange = (editor) => {
        if (isUpdatingFromServer) return;
        
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
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/api/boards/${boardId}/ws`;
    
    ws = new WebSocket(wsUrl);
    
    ws.onopen = () => {
        updateConnectionStatus(true);
    };
    
    ws.onclose = () => {
        updateConnectionStatus(false);
        setTimeout(() => {
            if (currentBoardId === boardId) {
                connectWebSocket(boardId);
            }
        }, 3000);
    };
    
    ws.onmessage = (event) => {
        const message = JSON.parse(event.data);
        handleWebSocketMessage(message);
    };
    
    ws.onerror = (error) => {
        console.error('WebSocket error:', error);
    };
}

function handleWebSocketMessage(message) {
    switch (message.type) {
        case 'update':
            isUpdatingFromServer = true;
            if (message.data.code_html !== undefined && htmlEditor.getValue() !== message.data.code_html) {
                const htmlCursor = htmlEditor.getCursor();
                htmlEditor.setValue(message.data.code_html);
                htmlEditor.setCursor(htmlCursor);
            }
            if (message.data.code_css !== undefined && cssEditor.getValue() !== message.data.code_css) {
                const cssCursor = cssEditor.getCursor();
                cssEditor.setValue(message.data.code_css);
                cssEditor.setCursor(cssCursor);
            }
            updatePreview();
            setTimeout(() => {
                isUpdatingFromServer = false;
            }, 100);
            break;
            
        case 'connections':
            updateUserCount(message.data.count);
            break;
    }
}

function sendUpdate() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    
    const currentHTML = htmlEditor.getValue();
    const currentCSS = cssEditor.getValue();
    
    if (currentHTML === lastSentHTML && currentCSS === lastSentCSS) {
        return;
    }
    
    lastSentHTML = currentHTML;
    lastSentCSS = currentCSS;
    
    const update = {
        type: 'update',
        data: {
            code_html: currentHTML,
            code_css: currentCSS
        }
    };
    
    ws.send(JSON.stringify(update));
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
    const blob = new Blob([previewContent], { type: 'text/html' });
    preview.src = URL.createObjectURL(blob);
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

function updateUserCount(count) {
    const userCount = document.getElementById('user-count');
    userCount.textContent = `${count} user${count !== 1 ? 's' : ''}`;
}

function shareBoard() {
    const url = window.location.href;
    navigator.clipboard.writeText(url).then(() => {
        alert('Board URL copied to clipboard!');
    }).catch(err => {
        console.error('Error copying to clipboard:', err);
    });
}
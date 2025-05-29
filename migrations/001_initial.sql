CREATE TABLE IF NOT EXISTS boards (
    id VARCHAR(36) PRIMARY KEY,
    code_html TEXT NOT NULL DEFAULT '',
    code_css TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions (
    id VARCHAR(36) PRIMARY KEY,
    board_id VARCHAR(36) NOT NULL,
    user_token VARCHAR(36) NOT NULL,
    connected_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (board_id) REFERENCES boards(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_sessions_board_id ON sessions(board_id);
CREATE INDEX IF NOT EXISTS idx_boards_updated_at ON boards(updated_at);
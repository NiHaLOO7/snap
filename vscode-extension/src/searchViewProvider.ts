import * as vscode from 'vscode';

export type SearchMode = 'file' | 'content' | 'checkpoint';

export class SearchViewProvider implements vscode.WebviewViewProvider {
    public static readonly viewType = 'snapSearch';

    private _view?: vscode.WebviewView;
    private _onSearch: (query: string, opts: { caseSensitive: boolean; mode: SearchMode; regex: boolean }) => void;
    private _onOpenFile: (filePath: string, snapshotId: number, line?: number) => void;
    private _onDiffFile: (filePath: string, snapshotId: number) => void;
    private _onOpenCheckpoint: (snapshotId: number) => void;
    private _onRestoreFile: (filePath: string, snapshotId: number) => void;

    constructor(
        private readonly _extensionUri: vscode.Uri,
        callbacks: {
            onSearch: (query: string, opts: { caseSensitive: boolean; mode: SearchMode; regex: boolean }) => void;
            onOpenFile: (filePath: string, snapshotId: number, line?: number) => void;
            onDiffFile: (filePath: string, snapshotId: number) => void;
            onOpenCheckpoint: (snapshotId: number) => void;
            onRestoreFile: (filePath: string, snapshotId: number) => void;
        }
    ) {
        this._onSearch = callbacks.onSearch;
        this._onOpenFile = callbacks.onOpenFile;
        this._onDiffFile = callbacks.onDiffFile;
        this._onOpenCheckpoint = callbacks.onOpenCheckpoint;
        this._onRestoreFile = callbacks.onRestoreFile;
    }

    resolveWebviewView(webviewView: vscode.WebviewView) {
        this._view = webviewView;
        webviewView.webview.options = {
            enableScripts: true,
            localResourceRoots: [vscode.Uri.joinPath(this._extensionUri, 'node_modules', '@vscode', 'codicons', 'dist')],
        };
        webviewView.webview.html = this._getHtml(webviewView.webview);

        webviewView.webview.onDidReceiveMessage(msg => {
            switch (msg.type) {
                case 'search':
                    this._onSearch(msg.query, msg.opts);
                    break;
                case 'openFile':
                    this._onOpenFile(msg.filePath, msg.snapshotId, msg.line);
                    break;
                case 'diffFile':
                    this._onDiffFile(msg.filePath, msg.snapshotId);
                    break;
                case 'openCheckpoint':
                    this._onOpenCheckpoint(msg.snapshotId);
                    break;
                case 'restoreFile':
                    this._onRestoreFile(msg.filePath, msg.snapshotId);
                    break;
            }
        });
    }

    setResults(data: { type: 'file' | 'content' | 'checkpoint'; query: string; results: any[] }) {
        this._view?.webview.postMessage({ cmd: 'results', searchType: data.type, query: data.query, results: data.results });
    }

    setLoading(loading: boolean) {
        this._view?.webview.postMessage({ cmd: 'loading', loading });
    }

    clear() {
        this._view?.webview.postMessage({ cmd: 'clear' });
    }

    private _getHtml(webview: vscode.Webview): string {
        const codiconUri = webview.asWebviewUri(vscode.Uri.joinPath(this._extensionUri, 'node_modules', '@vscode', 'codicons', 'dist', 'codicon.css'));
        return /*html*/ `<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1.0">
<link href="${codiconUri}" rel="stylesheet" />
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
    font-family: var(--vscode-font-family);
    font-size: var(--vscode-font-size);
    color: var(--vscode-foreground);
    background: var(--vscode-sideBar-background);
    overflow-x: hidden;
}

.search-box {
    display: flex;
    align-items: center;
    padding: 4px 8px;
    gap: 0;
    background: var(--vscode-input-background);
    border: 1px solid var(--vscode-input-border, var(--vscode-widget-border, transparent));
    border-radius: 2px;
    margin: 8px 8px 0 8px;
}
.search-box:focus-within {
    border-color: var(--vscode-focusBorder);
}
.search-box input {
    flex: 1;
    background: transparent;
    border: none;
    outline: none;
    color: var(--vscode-input-foreground);
    font-family: var(--vscode-font-family);
    font-size: var(--vscode-font-size);
    padding: 3px 4px;
    min-width: 0;
}
.search-box input::placeholder {
    color: var(--vscode-input-placeholderForeground);
}

.toggles {
    display: flex;
    gap: 1px;
    flex-shrink: 0;
}
.toggle-btn {
    width: 26px;
    height: 22px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 3px;
    color: var(--vscode-foreground);
    opacity: 0.5;
    cursor: pointer;
    font-size: 11px;
    font-weight: 600;
    font-family: var(--vscode-editor-font-family, monospace);
    transition: opacity 0.1s;
}
.toggle-btn:hover {
    background: var(--vscode-toolbar-hoverBackground);
    opacity: 0.8;
}
.toggle-btn.active {
    opacity: 1;
    background: var(--vscode-inputOption-activeBackground, rgba(0, 100, 200, 0.3));
    border-color: var(--vscode-inputOption-activeBorder, var(--vscode-focusBorder));
    color: var(--vscode-inputOption-activeForeground, var(--vscode-foreground));
}

.mode-bar {
    display: flex;
    margin: 4px 8px 4px 8px;
    gap: 0;
    border-radius: 3px;
    overflow: hidden;
    border: 1px solid var(--vscode-widget-border, var(--vscode-input-border, rgba(128,128,128,0.3)));
}
.mode-btn {
    flex: 1;
    padding: 3px 0;
    text-align: center;
    font-size: 11px;
    cursor: pointer;
    background: transparent;
    border: none;
    color: var(--vscode-descriptionForeground);
    transition: all 0.1s;
}
.mode-btn:hover {
    background: var(--vscode-toolbar-hoverBackground);
}
.mode-btn.active {
    background: var(--vscode-inputOption-activeBackground, rgba(0, 100, 200, 0.3));
    color: var(--vscode-foreground);
    font-weight: 500;
}
.mode-btn + .mode-btn {
    border-left: 1px solid var(--vscode-widget-border, var(--vscode-input-border, rgba(128,128,128,0.3)));
}

.summary {
    padding: 4px 12px;
    font-size: 11px;
    color: var(--vscode-descriptionForeground);
    display: flex;
    align-items: center;
    gap: 6px;
}
.loading-dot {
    display: inline-block;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    border: 1.5px solid var(--vscode-progressBar-background, #0078d4);
    border-top-color: transparent;
    animation: spin 0.8s linear infinite;
}
@keyframes spin { to { transform: rotate(360deg); } }

.group-bar {
    display: flex;
    justify-content: flex-end;
    padding: 0 8px;
}
.group-bar.hidden { display: none; }

.results { padding: 0 0 8px 0; }

/* ── Checkpoint group (top-level collapsible) ── */
.checkpoint-group { margin: 0; }
.checkpoint-header {
    display: flex;
    align-items: center;
    padding: 4px 8px;
    cursor: pointer;
    gap: 5px;
    user-select: none;
    height: 22px;
}
.checkpoint-header:hover { background: var(--vscode-list-hoverBackground); }
.checkpoint-header .arrow {
    font-size: 10px;
    width: 14px;
    text-align: center;
    flex-shrink: 0;
    transition: transform 0.1s;
}
.checkpoint-header .arrow.collapsed { transform: rotate(-90deg); }
.codicon {
    font-family: 'codicon';
    font-size: 16px;
    width: 16px;
    flex-shrink: 0;
    text-align: center;
    line-height: 1;
}
.cp-icon { flex-shrink: 0; width: 16px; text-align: center; }
.cp-icon.pinned { color: var(--vscode-textLink-foreground, #3794ff); }
.cp-icon.latest { color: var(--vscode-charts-green, #89d185); }
.cp-icon.auto { color: var(--vscode-descriptionForeground); }
.cp-icon.normal { color: var(--vscode-foreground); }
.checkpoint-label {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    flex: 1;
    font-weight: 600;
}
.checkpoint-meta {
    flex-shrink: 0;
    color: var(--vscode-descriptionForeground);
    font-size: 0.9em;
    white-space: nowrap;
}
.checkpoint-desc {
    padding: 0 8px 2px 40px;
    font-size: 11px;
    color: var(--vscode-descriptionForeground);
    font-style: italic;
}
.checkpoint-children {
    overflow: hidden;
}
.checkpoint-children.collapsed { max-height: 0 !important; }

/* ── Folder row inside checkpoint ── */
.folder-row {
    display: flex;
    align-items: center;
    padding: 2px 8px 2px 0;
    cursor: pointer;
    gap: 4px;
    height: 22px;
    user-select: none;
}
.folder-row:hover { background: var(--vscode-list-hoverBackground); }
.folder-row .arrow {
    font-size: 10px;
    width: 14px;
    text-align: center;
    flex-shrink: 0;
    transition: transform 0.1s;
}
.folder-row .arrow.collapsed { transform: rotate(-90deg); }
.folder-icon { flex-shrink: 0; width: 16px; text-align: center; color: var(--vscode-icon-foreground); }
.folder-name { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; flex: 1; }
.folder-children { overflow: hidden; }
.folder-children.collapsed { max-height: 0 !important; }

/* ── File row inside checkpoint/folder ── */
.file-row {
    display: flex;
    align-items: center;
    padding: 2px 8px 2px 0;
    cursor: pointer;
    gap: 4px;
    height: 22px;
}
.file-row:hover { background: var(--vscode-list-hoverBackground); }
.file-icon { flex-shrink: 0; width: 16px; text-align: center; color: var(--vscode-icon-foreground); }
.file-name {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    flex: 1;
}
.file-dir {
    color: var(--vscode-descriptionForeground);
    font-size: 0.9em;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    flex: 1;
}

/* Status badges (M/D) */
.status-badge {
    flex-shrink: 0;
    font-size: 11px;
    font-weight: 600;
    min-width: 14px;
    text-align: center;
}
.status-badge.modified { color: var(--vscode-list-warningForeground, #cca700); }
.status-badge.deleted { color: var(--vscode-list-errorForeground, #f48771); }

/* ── Content search: file subgroup inside checkpoint ── */
.file-subgroup { margin: 0; }
.file-subheader {
    display: flex;
    align-items: center;
    padding: 2px 8px 2px 28px;
    cursor: pointer;
    gap: 4px;
    user-select: none;
}
.file-subheader:hover { background: var(--vscode-list-hoverBackground); }
.file-subheader .arrow {
    font-size: 10px;
    width: 14px;
    text-align: center;
    flex-shrink: 0;
    transition: transform 0.1s;
}
.file-subheader .arrow.collapsed { transform: rotate(-90deg); }
.match-badge {
    flex-shrink: 0;
    margin-left: auto;
    background: var(--vscode-badge-background);
    color: var(--vscode-badge-foreground);
    border-radius: 8px;
    padding: 0 6px;
    font-size: 10px;
    line-height: 18px;
}

.match-lines { overflow: hidden; }
.match-lines.collapsed { max-height: 0 !important; }
.match-line {
    display: flex;
    padding: 1px 8px 1px 48px;
    cursor: pointer;
    font-family: var(--vscode-editor-font-family, monospace);
    font-size: 12px;
    line-height: 20px;
    gap: 6px;
}
.match-line:hover { background: var(--vscode-list-hoverBackground); }
.line-num {
    color: var(--vscode-editorLineNumber-foreground, var(--vscode-descriptionForeground));
    min-width: 28px;
    text-align: right;
    flex-shrink: 0;
}
.line-content {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
}
.highlight {
    background: var(--vscode-editor-findMatchHighlightBackground, rgba(234, 92, 0, 0.33));
    border-radius: 2px;
}

/* ── Action buttons (show on hover) ── */
.actions {
    display: flex;
    gap: 2px;
    flex-shrink: 0;
    margin-left: auto;
    opacity: 0;
}
.checkpoint-header:hover .actions,
.file-row:hover .actions,
.folder-row:hover .actions,
.file-subheader:hover .actions,
.match-line:hover .actions {
    opacity: 1;
}
.action-btn {
    width: 20px;
    height: 20px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: none;
    color: var(--vscode-foreground);
    cursor: pointer;
    border-radius: 3px;
    font-size: 13px;
    opacity: 0.7;
}
.action-btn:hover {
    background: var(--vscode-toolbar-hoverBackground);
    opacity: 1;
}

/* ── Checkpoint tags (flat mode) ── */
.cp-tags {
    display: flex;
    gap: 3px;
    flex-shrink: 0;
    margin-left: auto;
    margin-right: 4px;
}
.cp-tag {
    font-size: 10px;
    padding: 0 4px;
    border-radius: 3px;
    background: var(--vscode-badge-background);
    color: var(--vscode-badge-foreground);
    line-height: 16px;
    white-space: nowrap;
    cursor: default;
}

.empty-state {
    padding: 16px 12px;
    text-align: center;
    color: var(--vscode-descriptionForeground);
    font-size: 12px;
}
</style>
</head>
<body>

<div class="search-box">
    <input type="text" id="searchInput" placeholder="Search file names" spellcheck="false" />
    <div class="toggles">
        <button class="toggle-btn" id="toggleCase" title="Match Case">Aa</button>
        <button class="toggle-btn" id="toggleRegex" title="Use Regular Expression">.*</button>
    </div>
</div>

<div class="mode-bar">
    <button class="mode-btn active" data-mode="file">Files</button>
    <button class="mode-btn" data-mode="content">Content</button>
    <button class="mode-btn" data-mode="checkpoint">Checkpoints</button>
</div>

<div id="groupBar" class="group-bar">
    <button class="toggle-btn" id="toggleGroup" title="Group results by checkpoint"><span class="codicon codicon-list-tree"></span></button>
</div>

<div id="summary" class="summary"></div>
<div id="results" class="results"></div>

<script>
const vscode = acquireVsCodeApi();
const input = document.getElementById('searchInput');
const summary = document.getElementById('summary');
const results = document.getElementById('results');
const toggleCase = document.getElementById('toggleCase');
const toggleRegex = document.getElementById('toggleRegex');
const modeBtns = document.querySelectorAll('.mode-btn');

const toggleGroup = document.getElementById('toggleGroup');
const groupBar = document.getElementById('groupBar');
let caseSensitive = false;
let regexMode = false;
let grouped = true;
let currentMode = 'file';
let debounceTimer = null;
let lastQuery = '';
let lastData = null;

const placeholders = {
    file: 'Search file names',
    content: 'Search file contents',
    checkpoint: 'Search checkpoints by message'
};

toggleCase.addEventListener('click', () => {
    caseSensitive = !caseSensitive;
    toggleCase.classList.toggle('active', caseSensitive);
    doSearch();
});

toggleRegex.addEventListener('click', () => {
    regexMode = !regexMode;
    toggleRegex.classList.toggle('active', regexMode);
    doSearch();
});

toggleGroup.classList.add('active');
toggleGroup.addEventListener('click', () => {
    grouped = !grouped;
    toggleGroup.classList.toggle('active', grouped);
    if (lastData) { renderResults(lastData); }
});

modeBtns.forEach(btn => {
    btn.addEventListener('click', () => {
        modeBtns.forEach(b => b.classList.remove('active'));
        btn.classList.add('active');
        currentMode = btn.dataset.mode;
        input.placeholder = placeholders[currentMode];
        groupBar.classList.toggle('hidden', currentMode !== 'file');
        doSearch();
    });
});

input.addEventListener('input', () => {
    clearTimeout(debounceTimer);
    debounceTimer = setTimeout(doSearch, 300);
});

input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
        clearTimeout(debounceTimer);
        doSearch();
    }
    if (e.key === 'Escape') {
        input.value = '';
        results.innerHTML = '';
        summary.textContent = '';
    }
});

function doSearch() {
    const query = input.value.trim();
    if (!query) {
        results.innerHTML = '';
        summary.textContent = '';
        return;
    }
    lastQuery = query;
    summary.innerHTML = '<span class="loading-dot"></span> Searching...';
    vscode.postMessage({
        type: 'search',
        query,
        opts: { caseSensitive, mode: currentMode, regex: regexMode }
    });
}

window.addEventListener('message', (event) => {
    const msg = event.data;
    if (msg.cmd === 'results') {
        renderResults({ type: msg.searchType, query: msg.query, results: msg.results });
    } else if (msg.cmd === 'clear') {
        results.innerHTML = '';
        summary.textContent = '';
        input.value = '';
    } else if (msg.cmd === 'loading') {
        if (msg.loading) {
            summary.innerHTML = '<span class="loading-dot"></span> Searching...';
        }
    }
});

function renderResults(data) {
    lastData = data;
    results.innerHTML = '';

    if (!data.results || data.results.length === 0) {
        summary.textContent = 'No results found';
        results.innerHTML = '<div class="empty-state">No matches for "' + escapeHtml(data.query) + '"</div>';
        return;
    }

    if (data.type === 'checkpoint') {
        renderCheckpointResults(data);
    } else if (data.type === 'file') {
        grouped ? renderFileResults(data) : renderFileResultsFlat(data);
    } else {
        renderContentResults(data);
    }
}

/* ── Build folder tree from file list (like snapProvider) ── */
function buildFolderTree(files) {
    const root = { files: [], subfolders: new Map() };
    for (const filePath of files) {
        const parts = filePath.split('/');
        let current = root;
        for (let i = 0; i < parts.length - 1; i++) {
            if (!current.subfolders.has(parts[i])) {
                current.subfolders.set(parts[i], { files: [], subfolders: new Map() });
            }
            current = current.subfolders.get(parts[i]);
        }
        current.files.push(filePath);
    }
    return root;
}

function collectAllFiles(tree) {
    const result = [...tree.files];
    for (const sub of tree.subfolders.values()) {
        result.push(...collectAllFiles(sub));
    }
    return result;
}

/* Compute folder status from children file statuses */
function computeFolderStatus(files, fileStatuses) {
    let allDeleted = true;
    let hasChanges = false;
    for (const f of files) {
        const s = fileStatuses[f];
        if (!s) { allDeleted = false; }
        else { hasChanges = true; if (s !== 'deleted') allDeleted = false; }
    }
    if (allDeleted && files.length > 0) return 'deleted';
    if (hasChanges) return 'modified';
    return 'same';
}

/* Render compacted folder tree nodes recursively (matches snapProvider) */
function renderFolderTree(tree, prefix, depth, snapshotId, fileStatuses, container) {
    const indent = 22 + depth * 16;

    // Folders first
    for (const [name, subtree] of tree.subfolders) {
        let displayName = name;
        let currentPath = prefix ? prefix + '/' + name : name;
        let current = subtree;

        // Compact single-child folders (src/utils → src/utils)
        while (current.files.length === 0 && current.subfolders.size === 1) {
            const entry = current.subfolders.entries().next().value;
            displayName = displayName + '/' + entry[0];
            currentPath = currentPath + '/' + entry[0];
            current = entry[1];
        }

        const allFiles = collectAllFiles(current);
        const folderStatus = computeFolderStatus(allFiles, fileStatuses);

        const folderRow = document.createElement('div');
        folderRow.className = 'folder-row';
        folderRow.style.paddingLeft = indent + 'px';

        let statusHtml = '';
        if (folderStatus === 'modified') { statusHtml = '<span class="status-badge modified">M</span>'; }
        else if (folderStatus === 'deleted') { statusHtml = '<span class="status-badge deleted">D</span>'; }

        folderRow.innerHTML =
            '<span class="arrow">▾</span>' +
            '<span class="folder-icon codicon codicon-folder"></span>' +
            '<span class="folder-name">' + escapeHtml(displayName) + '</span>' +
            statusHtml;

        const folderChildren = document.createElement('div');
        folderChildren.className = 'folder-children';

        const folderArrow = folderRow.querySelector('.arrow');
        folderRow.addEventListener('click', () => {
            const collapsed = folderChildren.classList.toggle('collapsed');
            folderArrow.classList.toggle('collapsed', collapsed);
        });

        container.appendChild(folderRow);
        container.appendChild(folderChildren);

        renderFolderTree(current, currentPath, depth + 1, snapshotId, fileStatuses, folderChildren);
    }

    // Then files
    for (const filePath of tree.files) {
        const fileName = filePath.substring(filePath.lastIndexOf('/') + 1);
        const status = fileStatuses[filePath];

        const row = document.createElement('div');
        row.className = 'file-row';
        row.style.paddingLeft = indent + 'px';

        let statusHtml = '';
        if (status === 'modified') { statusHtml = '<span class="status-badge modified">M</span>'; }
        else if (status === 'deleted') { statusHtml = '<span class="status-badge deleted">D</span>'; }

        let nameColor = '';
        if (status === 'deleted') { nameColor = ' style="color: var(--vscode-list-errorForeground, #f48771)"'; }
        else if (status === 'modified') { nameColor = ' style="color: var(--vscode-list-warningForeground, #cca700)"'; }

        row.innerHTML =
            '<span class="file-icon codicon codicon-file"></span>' +
            '<span class="file-name"' + nameColor + '>' + escapeHtml(fileName) + '</span>' +
            '<div class="actions">' +
                '<button class="action-btn" title="Diff with Current" data-action="diff"><span class="codicon codicon-diff"></span></button>' +
                '<button class="action-btn" title="Restore File" data-action="restore"><span class="codicon codicon-discard"></span></button>' +
            '</div>' +
            statusHtml;

        row.addEventListener('click', (e) => {
            if (e.target.closest('.action-btn')) return;
            vscode.postMessage({ type: 'openFile', filePath, snapshotId });
        });
        row.querySelector('[data-action="diff"]').addEventListener('click', () => {
            vscode.postMessage({ type: 'diffFile', filePath, snapshotId });
        });
        row.querySelector('[data-action="restore"]').addEventListener('click', () => {
            vscode.postMessage({ type: 'restoreFile', filePath, snapshotId });
        });

        container.appendChild(row);
    }
}

/* ── Checkpoint search: exact native Checkpoints tree style ── */
function renderCheckpointResults(data) {
    summary.textContent = data.results.length + ' checkpoint' + (data.results.length !== 1 ? 's' : '') + ' found';
    for (const r of data.results) {
        const group = document.createElement('div');
        group.className = 'checkpoint-group';

        let iconHtml;
        if (r.pinned) {
            iconHtml = '<span class="cp-icon pinned codicon codicon-pinned"></span>';
        } else if (r.isLatest) {
            iconHtml = '<span class="cp-icon latest codicon codicon-circle-large-filled"></span>';
        } else if (r.autoSave) {
            iconHtml = '<span class="cp-icon auto codicon codicon-circle-small"></span>';
        } else {
            iconHtml = '<span class="cp-icon normal codicon codicon-circle-large-outline"></span>';
        }

        const header = document.createElement('div');
        header.className = 'checkpoint-header';
        const labelText = '#' + r.id + ' — ' + r.message;

        header.innerHTML =
            '<span class="arrow collapsed">▾</span>' +
            iconHtml +
            '<span class="checkpoint-label">' + highlightMatch(labelText, data.query) + '</span>' +
            '<span class="checkpoint-meta">' + escapeHtml(r.time) + ' • ' + r.fileCount + ' files</span>' +
            '<div class="actions">' +
                '<button class="action-btn" title="Restore to this Checkpoint" data-action="restore-cp"><span class="codicon codicon-discard"></span></button>' +
            '</div>';

        const children = document.createElement('div');
        children.className = 'checkpoint-children collapsed';

        if (r.files && r.files.length > 0) {
            const folderTree = buildFolderTree(r.files.slice().sort());
            const fileStatuses = r.fileStatuses || {};
            renderFolderTree(folderTree, '', 0, r.id, fileStatuses, children);
        }

        const arrow = header.querySelector('.arrow');
        header.addEventListener('click', (e) => {
            if (e.target.closest('.action-btn')) return;
            const collapsed = children.classList.toggle('collapsed');
            arrow.classList.toggle('collapsed', collapsed);
        });
        const restoreCpBtn = header.querySelector('[data-action="restore-cp"]');
        if (restoreCpBtn) {
            restoreCpBtn.addEventListener('click', () => {
                vscode.postMessage({ type: 'openCheckpoint', snapshotId: r.id });
            });
        }

        group.appendChild(header);
        if (r.description) {
            const desc = document.createElement('div');
            desc.className = 'checkpoint-desc';
            desc.textContent = r.description;
            group.appendChild(desc);
        }
        group.appendChild(children);
        results.appendChild(group);
    }
}

/* ── File search: grouped by checkpoint, files with path ── */
function renderFileResults(data) {
    // Group by checkpoint
    const cpMap = new Map();
    const cpOrder = [];
    for (const r of data.results) {
        if (!cpMap.has(r.snapshot_id)) {
            cpMap.set(r.snapshot_id, { meta: r.checkpoint_meta || null, files: [] });
            cpOrder.push(r.snapshot_id);
        }
        cpMap.get(r.snapshot_id).files.push(r);
    }

    let totalFiles = data.results.length;
    summary.textContent = totalFiles + ' file' + (totalFiles !== 1 ? 's' : '') + ' in ' + cpOrder.length + ' checkpoint' + (cpOrder.length !== 1 ? 's' : '');

    for (const snapId of cpOrder) {
        const entry = cpMap.get(snapId);
        const meta = entry.meta;

        const group = document.createElement('div');
        group.className = 'checkpoint-group';

        const header = document.createElement('div');
        header.className = 'checkpoint-header';
        const iconHtml = meta && meta.pinned ? '<span class="cp-icon pinned codicon codicon-pinned"></span>' : '<span class="cp-icon normal codicon codicon-circle-large-outline"></span>';
        const labelText = meta ? '#' + snapId + ' — ' + meta.message : '#' + snapId;
        const metaHtml = meta ? escapeHtml(meta.time) + ' • ' + meta.fileCount + ' files' : '';
        header.innerHTML =
            '<span class="arrow">▾</span>' +
            iconHtml +
            '<span class="checkpoint-label">' + escapeHtml(labelText) + '</span>' +
            '<span class="checkpoint-meta">' + metaHtml + '</span>';

        const children = document.createElement('div');
        children.className = 'checkpoint-children';

        for (const r of entry.files) {
            const parts = splitPath(r.path);
            const row = document.createElement('div');
            row.className = 'file-row';
            row.style.paddingLeft = '28px';
            row.innerHTML =
                '<span class="file-icon codicon codicon-file"></span>' +
                '<span class="file-name">' + highlightMatch(parts.name, data.query) + '</span>' +
                (parts.dir ? '<span class="file-dir">' + escapeHtml(parts.dir) + '</span>' : '') +
                '<div class="actions">' +
                    '<button class="action-btn" title="Diff with Current" data-action="diff"><span class="codicon codicon-diff"></span></button>' +
                    '<button class="action-btn" title="Restore File" data-action="restore"><span class="codicon codicon-discard"></span></button>' +
                '</div>';
            row.addEventListener('click', (e) => {
                if (e.target.closest('.action-btn')) return;
                vscode.postMessage({ type: 'openFile', filePath: r.path, snapshotId: snapId });
            });
            row.querySelector('[data-action="diff"]').addEventListener('click', () => {
                vscode.postMessage({ type: 'diffFile', filePath: r.path, snapshotId: snapId });
            });
            row.querySelector('[data-action="restore"]').addEventListener('click', () => {
                vscode.postMessage({ type: 'restoreFile', filePath: r.path, snapshotId: snapId });
            });
            children.appendChild(row);
        }

        const arrow = header.querySelector('.arrow');
        header.addEventListener('click', (e) => {
            if (e.target.closest('.action-btn')) return;
            const collapsed = children.classList.toggle('collapsed');
            arrow.classList.toggle('collapsed', collapsed);
        });

        group.appendChild(header);
        group.appendChild(children);
        results.appendChild(group);
    }
}

/* ── Content search: checkpoint → file (with path) → matched lines (highlighted) ── */
function renderContentResults(data) {
    // Group: checkpoint → file → lines
    const cpMap = new Map();
    const cpOrder = [];
    for (const r of data.results) {
        if (!cpMap.has(r.snapshot_id)) {
            cpMap.set(r.snapshot_id, { meta: r.checkpoint_meta || null, files: new Map(), fileOrder: [] });
            cpOrder.push(r.snapshot_id);
        }
        const cpEntry = cpMap.get(r.snapshot_id);
        if (!cpEntry.files.has(r.path)) {
            cpEntry.files.set(r.path, []);
            cpEntry.fileOrder.push(r.path);
        }
        cpEntry.files.get(r.path).push(r);
    }

    const totalMatches = data.results.length;
    let totalFiles = 0;
    for (const [, cp] of cpMap) { totalFiles += cp.files.size; }
    summary.textContent = totalMatches + ' result' + (totalMatches !== 1 ? 's' : '') + ' in ' + totalFiles + ' file' + (totalFiles !== 1 ? 's' : '') + ' across ' + cpOrder.length + ' checkpoint' + (cpOrder.length !== 1 ? 's' : '');

    for (const snapId of cpOrder) {
        const cpEntry = cpMap.get(snapId);
        const meta = cpEntry.meta;

        const group = document.createElement('div');
        group.className = 'checkpoint-group';

        // Checkpoint header
        const cpHeader = document.createElement('div');
        cpHeader.className = 'checkpoint-header';
        const iconHtml = meta && meta.pinned ? '<span class="cp-icon pinned codicon codicon-pinned"></span>' : '<span class="cp-icon normal codicon codicon-circle-large-outline"></span>';
        const labelText = meta ? '#' + snapId + ' — ' + meta.message : '#' + snapId;
        const metaHtml = meta ? escapeHtml(meta.time) + ' • ' + meta.fileCount + ' files' : '';
        cpHeader.innerHTML =
            '<span class="arrow">▾</span>' +
            iconHtml +
            '<span class="checkpoint-label">' + escapeHtml(labelText) + '</span>' +
            '<span class="checkpoint-meta">' + metaHtml + '</span>';

        const cpChildren = document.createElement('div');
        cpChildren.className = 'checkpoint-children';

        for (const filePath of cpEntry.fileOrder) {
            const matches = cpEntry.files.get(filePath);
            const parts = splitPath(filePath);

            const fileGroup = document.createElement('div');
            fileGroup.className = 'file-subgroup';

            // File subheader
            const fileHeader = document.createElement('div');
            fileHeader.className = 'file-subheader';
            fileHeader.innerHTML =
                '<span class="arrow">▾</span>' +
                '<span class="file-name">' + escapeHtml(parts.name) + '</span>' +
                (parts.dir ? '<span class="file-dir">' + escapeHtml(parts.dir) + '</span>' : '') +
                '<span class="match-badge">' + matches.length + '</span>' +
                '<div class="actions">' +
                    '<button class="action-btn" title="Diff with Current" data-action="diff"><span class="codicon codicon-diff"></span></button>' +
                    '<button class="action-btn" title="Restore File" data-action="restore"><span class="codicon codicon-discard"></span></button>' +
                '</div>';

            const linesEl = document.createElement('div');
            linesEl.className = 'match-lines';

            for (const m of matches) {
                const line = document.createElement('div');
                line.className = 'match-line';
                line.innerHTML =
                    '<span class="line-num">' + m.line + '</span>' +
                    '<span class="line-content">' + highlightContent(m.content.trim(), data.query) + '</span>';
                line.addEventListener('click', () => {
                    vscode.postMessage({ type: 'openFile', filePath: m.path, snapshotId: snapId, line: m.line });
                });
                linesEl.appendChild(line);
            }

            const fileArrow = fileHeader.querySelector('.arrow');
            fileHeader.addEventListener('click', (e) => {
                if (e.target.closest('.action-btn')) return;
                const collapsed = linesEl.classList.toggle('collapsed');
                fileArrow.classList.toggle('collapsed', collapsed);
            });
            fileHeader.querySelector('[data-action="diff"]').addEventListener('click', () => {
                vscode.postMessage({ type: 'diffFile', filePath, snapshotId: snapId });
            });
            fileHeader.querySelector('[data-action="restore"]').addEventListener('click', () => {
                vscode.postMessage({ type: 'restoreFile', filePath, snapshotId: snapId });
            });

            fileGroup.appendChild(fileHeader);
            fileGroup.appendChild(linesEl);
            cpChildren.appendChild(fileGroup);
        }

        const cpArrow = cpHeader.querySelector('.arrow');
        cpHeader.addEventListener('click', (e) => {
            if (e.target.closest('.action-btn')) return;
            const collapsed = cpChildren.classList.toggle('collapsed');
            cpArrow.classList.toggle('collapsed', collapsed);
        });

        group.appendChild(cpHeader);
        group.appendChild(cpChildren);
        results.appendChild(group);
    }
}

/* ── File search flat — dedup by (path + hash), group same-version checkpoints ── */
function renderFileResultsFlat(data) {
    // Group by (path + hash) — each unique file version is one entry
    const versionMap = new Map();
    const versionOrder = [];
    for (const r of data.results) {
        const key = r.path + '\0' + (r.hash || '');
        if (!versionMap.has(key)) {
            versionMap.set(key, { path: r.path, hash: r.hash || '', snapIds: [], latestSnapId: r.snapshot_id, score: r.score });
            versionOrder.push(key);
        }
        const entry = versionMap.get(key);
        entry.snapIds.push(r.snapshot_id);
        if (r.snapshot_id > entry.latestSnapId) { entry.latestSnapId = r.snapshot_id; }
        if (r.score > entry.score) { entry.score = r.score; }
    }

    const versions = versionOrder.map(k => versionMap.get(k));
    versions.sort((a, b) => b.score - a.score);

    const totalVersions = versions.length;
    const uniquePaths = new Set(versions.map(v => v.path)).size;
    summary.textContent = uniquePaths + ' file' + (uniquePaths !== 1 ? 's' : '') + ', ' + totalVersions + ' version' + (totalVersions !== 1 ? 's' : '');

    for (const v of versions) {
        const parts = splitPath(v.path);
        const uniqueIds = [...new Set(v.snapIds)].sort((a, b) => b - a);
        const cpTagsHtml = uniqueIds.map(id => {
            const meta = data.results.find(x => x.snapshot_id === id && x.checkpoint_meta);
            const msg = meta && meta.checkpoint_meta ? meta.checkpoint_meta.message : '';
            const tooltip = msg ? '#' + id + ' — ' + escapeHtml(msg) : '#' + id;
            return '<span class="cp-tag" title="' + tooltip + '">#' + id + '</span>';
        }).join('');

        const row = document.createElement('div');
        row.className = 'file-row';
        row.style.paddingLeft = '8px';
        row.innerHTML =
            '<span class="file-icon codicon codicon-file"></span>' +
            '<span class="file-name">' + highlightMatch(parts.name, data.query) + '</span>' +
            (parts.dir ? '<span class="file-dir">' + escapeHtml(parts.dir) + '</span>' : '') +
            '<span class="cp-tags">' + cpTagsHtml + '</span>' +
            '<div class="actions">' +
                '<button class="action-btn" title="Diff with Current" data-action="diff"><span class="codicon codicon-diff"></span></button>' +
                '<button class="action-btn" title="Restore File" data-action="restore"><span class="codicon codicon-discard"></span></button>' +
            '</div>';
        row.addEventListener('click', (e) => {
            if (e.target.closest('.action-btn')) return;
            vscode.postMessage({ type: 'openFile', filePath: v.path, snapshotId: v.latestSnapId });
        });
        row.querySelector('[data-action="diff"]').addEventListener('click', () => {
            vscode.postMessage({ type: 'diffFile', filePath: v.path, snapshotId: v.latestSnapId });
        });
        row.querySelector('[data-action="restore"]').addEventListener('click', () => {
            vscode.postMessage({ type: 'restoreFile', filePath: v.path, snapshotId: v.latestSnapId });
        });
        results.appendChild(row);
    }
}

/* ── Content search flat (no grouping) — dedup by (path, line), show all checkpoint names ── */
function renderContentResultsFlat(data) {
    // Collect all checkpoint IDs per file path
    const pathSnapIds = new Map();
    for (const r of data.results) {
        if (!pathSnapIds.has(r.path)) { pathSnapIds.set(r.path, new Set()); }
        pathSnapIds.get(r.path).add(r.snapshot_id);
    }

    const lineMap = new Map();
    for (const r of data.results) {
        const key = r.path + ':' + r.line;
        if (!lineMap.has(key) || r.snapshot_id > lineMap.get(key).snapshot_id) {
            lineMap.set(key, r);
        }
    }

    // Group deduped results by file
    const fileMap = new Map();
    const fileOrder = [];
    for (const [, r] of lineMap) {
        if (!fileMap.has(r.path)) {
            fileMap.set(r.path, []);
            fileOrder.push(r.path);
        }
        fileMap.get(r.path).push(r);
    }

    let totalMatches = lineMap.size;
    summary.textContent = totalMatches + ' result' + (totalMatches !== 1 ? 's' : '') + ' in ' + fileOrder.length + ' file' + (fileOrder.length !== 1 ? 's' : '');

    for (const filePath of fileOrder) {
        const matches = fileMap.get(filePath);
        matches.sort((a, b) => a.line - b.line);
        const parts = splitPath(filePath);

        const snapIds = [...(pathSnapIds.get(filePath) || [])].sort((a, b) => b - a);
        const cpTagsHtml = snapIds.map(id => {
            const meta = data.results.find(x => x.snapshot_id === id && x.checkpoint_meta);
            const msg = meta && meta.checkpoint_meta ? meta.checkpoint_meta.message : '';
            const tooltip = msg ? '#' + id + ' — ' + escapeHtml(msg) : '#' + id;
            return '<span class="cp-tag" title="' + tooltip + '">#' + id + '</span>';
        }).join('');

        const fileGroup = document.createElement('div');
        fileGroup.className = 'file-subgroup';

        const fileHeader = document.createElement('div');
        fileHeader.className = 'file-subheader';
        fileHeader.style.paddingLeft = '8px';
        fileHeader.innerHTML =
            '<span class="arrow">▾</span>' +
            '<span class="file-icon codicon codicon-file"></span>' +
            '<span class="file-name">' + escapeHtml(parts.name) + '</span>' +
            (parts.dir ? '<span class="file-dir">' + escapeHtml(parts.dir) + '</span>' : '') +
            '<span class="match-badge">' + matches.length + '</span>' +
            '<span class="cp-tags">' + cpTagsHtml + '</span>' +
            '<div class="actions">' +
                '<button class="action-btn" title="Diff with Current" data-action="diff"><span class="codicon codicon-diff"></span></button>' +
                '<button class="action-btn" title="Restore File" data-action="restore"><span class="codicon codicon-discard"></span></button>' +
            '</div>';

        const linesEl = document.createElement('div');
        linesEl.className = 'match-lines';

        for (const m of matches) {
            const line = document.createElement('div');
            line.className = 'match-line';
            line.style.paddingLeft = '32px';
            line.innerHTML =
                '<span class="line-num">' + m.line + '</span>' +
                '<span class="line-content">' + highlightContent(m.content.trim(), data.query) + '</span>';
            line.addEventListener('click', () => {
                vscode.postMessage({ type: 'openFile', filePath: m.path, snapshotId: m.snapshot_id, line: m.line });
            });
            linesEl.appendChild(line);
        }

        const fileArrow = fileHeader.querySelector('.arrow');
        fileHeader.addEventListener('click', (e) => {
            if (e.target.closest('.action-btn')) return;
            const collapsed = linesEl.classList.toggle('collapsed');
            fileArrow.classList.toggle('collapsed', collapsed);
        });
        fileHeader.querySelector('[data-action="diff"]').addEventListener('click', () => {
            vscode.postMessage({ type: 'diffFile', filePath, snapshotId: matches[0].snapshot_id });
        });
        fileHeader.querySelector('[data-action="restore"]').addEventListener('click', () => {
            vscode.postMessage({ type: 'restoreFile', filePath, snapshotId: matches[0].snapshot_id });
        });

        fileGroup.appendChild(fileHeader);
        fileGroup.appendChild(linesEl);
        results.appendChild(fileGroup);
    }
}

function splitPath(p) {
    const idx = p.lastIndexOf('/');
    if (idx === -1) return { name: p, dir: '' };
    return { name: p.substring(idx + 1), dir: p.substring(0, idx) };
}

function escapeHtml(s) {
    if (!s) return '';
    return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

function highlightMatch(text, query) {
    if (!text || !query) return escapeHtml(text);
    const escaped = escapeHtml(text);
    const queryEscaped = escapeHtml(query);
    try {
        const re = new RegExp('(' + queryEscaped.replace(/[.*+?^\${}()|[\\]\\\\]/g, '\\\\$&') + ')', 'gi');
        return escaped.replace(re, '<span class="highlight">$1</span>');
    } catch {
        return escaped;
    }
}

function highlightContent(text, query) {
    if (!text || !query) return escapeHtml(text);
    const escaped = escapeHtml(text);
    const queryEscaped = escapeHtml(query);
    try {
        const re = new RegExp('(' + queryEscaped.replace(/[.*+?^\${}()|[\\]\\\\]/g, '\\\\$&') + ')', caseSensitive ? 'g' : 'gi');
        return escaped.replace(re, '<span class="highlight">$1</span>');
    } catch {
        return escaped;
    }
}
</script>
</body>
</html>`;
    }
}

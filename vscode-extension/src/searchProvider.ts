import * as vscode from 'vscode';
import * as path from 'path';

export interface SearchFileResult {
    snapshot_id: number;
    path: string;
    score: number;
}

export interface SearchContentResult {
    snapshot_id: number;
    path: string;
    line: number;
    content: string;
}

export type SearchMode = 'filename' | 'content';

// ── File search result items ──

export class SearchFileItem extends vscode.TreeItem {
    constructor(
        public readonly filePath: string,
        public readonly snapshotId: number,
        public readonly score: number,
    ) {
        super(path.basename(filePath), vscode.TreeItemCollapsibleState.None);
        this.description = path.dirname(filePath) === '.' ? '' : path.dirname(filePath);
        this.tooltip = `${filePath} [#${snapshotId}] (score: ${score})`;
        this.iconPath = new vscode.ThemeIcon('file');
        this.contextValue = 'searchFileResult';
        this.command = {
            command: 'snap.openSearchFile',
            title: 'Open File',
            arguments: [this],
        };
    }
}

// ── Content search result items ──

export class SearchContentFileItem extends vscode.TreeItem {
    constructor(
        public readonly filePath: string,
        public readonly snapshotId: number,
        public readonly matchCount: number,
    ) {
        super(path.basename(filePath), vscode.TreeItemCollapsibleState.Expanded);
        const dir = path.dirname(filePath);
        this.description = `${dir === '.' ? '' : dir + ' '}— ${matchCount} match${matchCount > 1 ? 'es' : ''} [#${snapshotId}]`;
        this.iconPath = new vscode.ThemeIcon('file');
        this.contextValue = 'searchContentFile';
    }
}

export class SearchContentLineItem extends vscode.TreeItem {
    constructor(
        public readonly filePath: string,
        public readonly snapshotId: number,
        public readonly lineNumber: number,
        public readonly content: string,
    ) {
        super(`L${lineNumber}: ${content.trim()}`, vscode.TreeItemCollapsibleState.None);
        this.tooltip = `${filePath}:${lineNumber}`;
        this.iconPath = new vscode.ThemeIcon('symbol-text');
        this.contextValue = 'searchContentLine';
        this.command = {
            command: 'snap.openSearchLine',
            title: 'Open at Line',
            arguments: [this],
        };
    }
}

export class SearchSummaryItem extends vscode.TreeItem {
    constructor(query: string, totalMatches: number, totalFiles: number) {
        const label = `"${query}" — ${totalMatches} match${totalMatches !== 1 ? 'es' : ''} in ${totalFiles} file${totalFiles !== 1 ? 's' : ''}`;
        super(label, vscode.TreeItemCollapsibleState.None);
        this.iconPath = new vscode.ThemeIcon('search');
        this.contextValue = 'searchSummary';
    }
}

// ── Search TreeDataProvider ──

export class SearchProvider implements vscode.TreeDataProvider<vscode.TreeItem> {
    private _onDidChangeTreeData = new vscode.EventEmitter<vscode.TreeItem | undefined>();
    readonly onDidChangeTreeData = this._onDidChangeTreeData.event;

    private mode: SearchMode = 'filename';
    private lastQuery = '';
    private fileResults: SearchFileResult[] = [];
    private contentResults: SearchContentResult[] = [];

    constructor(private workspaceRoot: string) {}

    getMode(): SearchMode {
        return this.mode;
    }

    setMode(mode: SearchMode) {
        this.mode = mode;
    }

    setFileResults(query: string, results: SearchFileResult[]) {
        this.lastQuery = query;
        this.fileResults = results.map(r => ({
            snapshot_id: r.snapshot_id,
            path: r.path,
            score: r.score,
        }));
        this.contentResults = [];
        this._onDidChangeTreeData.fire(undefined);
    }

    setContentResults(query: string, results: SearchContentResult[]) {
        this.lastQuery = query;
        this.contentResults = results;
        this.fileResults = [];
        this._onDidChangeTreeData.fire(undefined);
    }

    clear() {
        this.lastQuery = '';
        this.fileResults = [];
        this.contentResults = [];
        this._onDidChangeTreeData.fire(undefined);
    }

    refresh() {
        this._onDidChangeTreeData.fire(undefined);
    }

    getTreeItem(element: vscode.TreeItem): vscode.TreeItem {
        return element;
    }

    getChildren(element?: vscode.TreeItem): vscode.TreeItem[] {
        if (!this.lastQuery) {
            return [new NoResultsItem('Type a query to search snapshots')];
        }

        // Content search — grouped by file
        if (this.contentResults.length > 0) {
            if (!element) {
                // Build file groups
                const groups = new Map<string, { snapId: number; matches: SearchContentResult[] }>();
                const order: string[] = [];

                for (const r of this.contentResults) {
                    const existing = groups.get(r.path);
                    if (existing) {
                        existing.matches.push(r);
                    } else {
                        groups.set(r.path, { snapId: r.snapshot_id, matches: [r] });
                        order.push(r.path);
                    }
                }

                const totalMatches = this.contentResults.length;
                const items: vscode.TreeItem[] = [
                    new SearchSummaryItem(this.lastQuery, totalMatches, groups.size),
                ];

                for (const p of order) {
                    const g = groups.get(p)!;
                    items.push(new SearchContentFileItem(p, g.snapId, g.matches.length));
                }

                return items;
            }

            if (element instanceof SearchContentFileItem) {
                const group = this.contentResults.filter(r => r.path === element.filePath);
                return group.map(r => new SearchContentLineItem(r.path, r.snapshot_id, r.line, r.content));
            }

            return [];
        }

        // File search — flat list
        if (this.fileResults.length > 0) {
            if (!element) {
                const items: vscode.TreeItem[] = [
                    new SearchSummaryItem(this.lastQuery, this.fileResults.length, this.fileResults.length),
                ];

                for (const r of this.fileResults) {
                    items.push(new SearchFileItem(r.path, r.snapshot_id, r.score));
                }

                return items;
            }
            return [];
        }

        return [new NoResultsItem(`No results for "${this.lastQuery}"`)];
    }
}

class NoResultsItem extends vscode.TreeItem {
    constructor(message: string) {
        super(message, vscode.TreeItemCollapsibleState.None);
        this.iconPath = new vscode.ThemeIcon('info');
    }
}

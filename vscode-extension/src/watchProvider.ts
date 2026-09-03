import * as vscode from 'vscode';
import { getWatchlist } from './snapCli';

class WatchItem extends vscode.TreeItem {
    constructor(public readonly filePath: string) {
        super(filePath, vscode.TreeItemCollapsibleState.None);
        this.iconPath = new vscode.ThemeIcon('eye');
        this.tooltip = `${filePath} — auto-checkpointed on every change`;
        this.contextValue = 'watchedFile';
    }
}

export class WatchProvider implements vscode.TreeDataProvider<WatchItem> {
    private _onDidChangeTreeData = new vscode.EventEmitter<WatchItem | undefined | null>();
    readonly onDidChangeTreeData = this._onDidChangeTreeData.event;

    constructor(private workspaceRoot: string) {}

    refresh(): void {
        this._onDidChangeTreeData.fire(undefined);
    }

    getTreeItem(element: WatchItem): vscode.TreeItem {
        return element;
    }

    async getChildren(): Promise<WatchItem[]> {
        const files = await getWatchlist(this.workspaceRoot);
        return files.map(f => new WatchItem(f));
    }
}

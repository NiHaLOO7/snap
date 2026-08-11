import * as vscode from 'vscode';
import { getStatus } from './snapCli';

class ChangeItem extends vscode.TreeItem {
    constructor(change: string) {
        const symbol = change.charAt(0);
        const fileName = change.substring(2).trim();

        super(fileName, vscode.TreeItemCollapsibleState.None);

        switch (symbol) {
            case '+':
                this.iconPath = new vscode.ThemeIcon('diff-added', new vscode.ThemeColor('gitDecoration.addedResourceForeground'));
                this.description = 'added';
                break;
            case '~':
                this.iconPath = new vscode.ThemeIcon('diff-modified', new vscode.ThemeColor('gitDecoration.modifiedResourceForeground'));
                this.description = 'modified';
                break;
            case '-':
                this.iconPath = new vscode.ThemeIcon('diff-removed', new vscode.ThemeColor('gitDecoration.deletedResourceForeground'));
                this.description = 'deleted';
                break;
        }
    }
}

export class ChangesProvider implements vscode.TreeDataProvider<ChangeItem> {
    private _onDidChangeTreeData = new vscode.EventEmitter<ChangeItem | undefined | null>();
    readonly onDidChangeTreeData = this._onDidChangeTreeData.event;

    constructor(private workspaceRoot: string) {}

    refresh(): void {
        this._onDidChangeTreeData.fire(undefined);
    }

    getTreeItem(element: ChangeItem): vscode.TreeItem {
        return element;
    }

    async getChildren(): Promise<ChangeItem[]> {
        const changes = await getStatus(this.workspaceRoot);
        return changes.map(change => new ChangeItem(change));
    }
}

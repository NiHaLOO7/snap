import * as vscode from 'vscode';
import { getSnapshots, SnapshotInfo } from './snapCli';

export class CategoryItem extends vscode.TreeItem {
    constructor(
        public readonly label: string,
        public readonly category: 'user' | 'auto',
    ) {
        super(label, vscode.TreeItemCollapsibleState.Expanded);
        this.contextValue = 'category';
        this.iconPath = category === 'user'
            ? new vscode.ThemeIcon('bookmark')
            : new vscode.ThemeIcon('history');
    }
}

export class SnapshotItem extends vscode.TreeItem {
    constructor(
        public readonly snapshotId: number,
        public readonly snapshot: SnapshotInfo,
        public readonly isLatest: boolean,
    ) {
        super(
            `#${snapshot.id} — ${snapshot.message}`,
            vscode.TreeItemCollapsibleState.Collapsed
        );

        const date = new Date(snapshot.timestamp);
        const timeStr = date.toLocaleString('en-US', {
            month: 'short',
            day: 'numeric',
            hour: '2-digit',
            minute: '2-digit',
        });

        const descPart = snapshot.description ? `\n${snapshot.description}` : '';
        this.description = `${timeStr} • ${snapshot.fileCount} files`;
        this.tooltip = `Snapshot #${snapshot.id}\n${snapshot.message}${descPart}\n${timeStr}\n${snapshot.fileCount} files${snapshot.autoSave ? '\n[auto-save]' : ''}`;

        if (isLatest) {
            this.iconPath = new vscode.ThemeIcon('circle-large-filled');
        } else if (snapshot.autoSave) {
            this.iconPath = new vscode.ThemeIcon('circle-small');
        } else {
            this.iconPath = new vscode.ThemeIcon('circle-large-outline');
        }

        this.contextValue = 'snapshot';
    }
}

export class FileItem extends vscode.TreeItem {
    constructor(
        public readonly filePath: string,
        public readonly snapshotId: number,
        public readonly workspaceRoot: string,
    ) {
        super(filePath, vscode.TreeItemCollapsibleState.None);

        this.iconPath = new vscode.ThemeIcon('file');
        this.tooltip = `Click to view content at snapshot #${snapshotId}`;
        this.contextValue = 'snapshotFile';

        this.command = {
            command: 'snap.showFile',
            title: 'Show File',
            arguments: [this],
        };
    }
}

type TreeNode = CategoryItem | SnapshotItem | FileItem;

export class SnapProvider implements vscode.TreeDataProvider<TreeNode> {
    private _onDidChangeTreeData = new vscode.EventEmitter<TreeNode | undefined | null>();
    readonly onDidChangeTreeData = this._onDidChangeTreeData.event;

    constructor(private workspaceRoot: string) {}

    refresh(): void {
        this._onDidChangeTreeData.fire(undefined);
    }

    getTreeItem(element: TreeNode): vscode.TreeItem {
        return element;
    }

    async getChildren(element?: TreeNode): Promise<TreeNode[]> {
        if (!element) {
            const snapshots = await getSnapshots(this.workspaceRoot);
            const hasUser = snapshots.some(s => !s.autoSave);
            const hasAuto = snapshots.some(s => s.autoSave);

            const categories: TreeNode[] = [];
            if (hasUser) {
                categories.push(new CategoryItem('Checkpoints', 'user'));
            }
            if (hasAuto) {
                categories.push(new CategoryItem('Auto-saves', 'auto'));
            }
            return categories;
        }

        if (element instanceof CategoryItem) {
            const snapshots = await getSnapshots(this.workspaceRoot);
            const filtered = element.category === 'user'
                ? snapshots.filter(s => !s.autoSave)
                : snapshots.filter(s => s.autoSave);

            return filtered
                .reverse()
                .map((snap, index) => new SnapshotItem(snap.id, snap, index === 0));
        }

        if (element instanceof SnapshotItem) {
            const files = element.snapshot.files || [];
            return files.map(f => new FileItem(f, element.snapshotId, this.workspaceRoot));
        }

        return [];
    }
}

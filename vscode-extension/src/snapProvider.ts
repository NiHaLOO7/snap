import * as vscode from 'vscode';
import * as fs from 'fs';
import * as crypto from 'crypto';
import { getSnapshots, SnapshotInfo } from './snapCli';
import * as path from 'path';

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

export class FolderItem extends vscode.TreeItem {
    constructor(
        public readonly folderName: string,
        public readonly folderPath: string,
        public readonly snapshotId: number,
        public readonly files: string[],
        public readonly workspaceRoot: string,
        public readonly snapshotTree: Record<string, string>,
    ) {
        super(folderName, vscode.TreeItemCollapsibleState.Collapsed);
        this.tooltip = `${folderPath} (${files.length} files)`;
        this.contextValue = 'snapshotFolder';

        // Determine folder status based on contained files
        const status = getFolderStatus(files, workspaceRoot, snapshotTree);
        this.resourceUri = vscode.Uri.parse(`snap-tree://folder/${snapshotId}/${folderPath}?status=${status}`);

        if (status === 'deleted') {
            this.iconPath = new vscode.ThemeIcon('folder', new vscode.ThemeColor('list.errorForeground'));
        } else if (status === 'modified') {
            this.iconPath = new vscode.ThemeIcon('folder', new vscode.ThemeColor('list.warningForeground'));
        } else {
            this.iconPath = new vscode.ThemeIcon('folder');
        }
    }
}

export class FileItem extends vscode.TreeItem {
    constructor(
        public readonly filePath: string,
        public readonly snapshotId: number,
        public readonly workspaceRoot: string,
        public readonly snapshotHash: string,
    ) {
        const fileName = path.basename(filePath);
        super(fileName, vscode.TreeItemCollapsibleState.None);

        this.tooltip = `${filePath}\nClick to diff with current • Right-click for more`;
        this.contextValue = 'snapshotFile';

        // Determine file status
        const status = getFileStatus(filePath, workspaceRoot, snapshotHash);
        this.resourceUri = vscode.Uri.parse(`snap-tree://file/${snapshotId}/${filePath}?status=${status}`);

        if (status === 'deleted') {
            this.iconPath = new vscode.ThemeIcon('file', new vscode.ThemeColor('list.errorForeground'));
            this.description = 'D';
        } else if (status === 'modified') {
            this.iconPath = new vscode.ThemeIcon('file', new vscode.ThemeColor('list.warningForeground'));
            this.description = 'M';
        } else {
            this.iconPath = new vscode.ThemeIcon('file');
            this.description = '';
        }

        this.command = {
            command: 'snap.diffFile',
            title: 'Diff File',
            arguments: [this],
        };
    }
}

function getFileStatus(filePath: string, workspaceRoot: string, snapshotHash: string): 'same' | 'modified' | 'deleted' {
    const fullPath = path.join(workspaceRoot, filePath);
    if (!fs.existsSync(fullPath)) {
        return 'deleted';
    }

    try {
        const data = fs.readFileSync(fullPath);
        const currentHash = crypto.createHash('sha256').update(data).digest('hex');
        if (currentHash !== snapshotHash) {
            return 'modified';
        }
    } catch {
        return 'deleted';
    }

    return 'same';
}

function getFolderStatus(files: string[], workspaceRoot: string, snapshotTree: Record<string, string>): 'same' | 'modified' | 'deleted' {
    let allDeleted = true;
    let hasChanges = false;

    for (const filePath of files) {
        const hash = snapshotTree[filePath] || '';
        const status = getFileStatus(filePath, workspaceRoot, hash);
        if (status !== 'deleted') {
            allDeleted = false;
        }
        if (status !== 'same') {
            hasChanges = true;
        }
    }

    if (allDeleted) {
        return 'deleted';
    }
    if (hasChanges) {
        return 'modified';
    }
    return 'same';
}

type TreeNode = CategoryItem | SnapshotItem | FolderItem | FileItem;

interface FolderTree {
    files: string[];
    subfolders: Map<string, FolderTree>;
}

function buildFolderTree(files: string[]): FolderTree {
    const root: FolderTree = { files: [], subfolders: new Map() };

    for (const filePath of files) {
        const parts = filePath.split('/');
        let current = root;

        for (let i = 0; i < parts.length - 1; i++) {
            const folder = parts[i];
            if (!current.subfolders.has(folder)) {
                current.subfolders.set(folder, { files: [], subfolders: new Map() });
            }
            current = current.subfolders.get(folder)!;
        }

        current.files.push(filePath);
    }

    return root;
}

function collectAllFiles(tree: FolderTree): string[] {
    const result: string[] = [...tree.files];
    for (const sub of tree.subfolders.values()) {
        result.push(...collectAllFiles(sub));
    }
    return result;
}

function getCompactedChildren(tree: FolderTree, prefix: string, snapshotId: number, workspaceRoot: string, snapshotTree: Record<string, string>): TreeNode[] {
    const nodes: TreeNode[] = [];

    for (const [name, subtree] of tree.subfolders) {
        let displayName = name;
        let currentPath = prefix ? `${prefix}/${name}` : name;
        let current = subtree;

        while (current.files.length === 0 && current.subfolders.size === 1) {
            const [childName, childTree] = current.subfolders.entries().next().value!;
            displayName = `${displayName}/${childName}`;
            currentPath = `${currentPath}/${childName}`;
            current = childTree;
        }

        const allFiles = collectAllFiles(current);
        nodes.push(new FolderItem(displayName, currentPath, snapshotId, allFiles, workspaceRoot, snapshotTree));
    }

    for (const filePath of tree.files) {
        const hash = snapshotTree[filePath] || '';
        nodes.push(new FileItem(filePath, snapshotId, workspaceRoot, hash));
    }

    return nodes;
}

export class SnapDecorationProvider implements vscode.FileDecorationProvider {
    private _onDidChangeFileDecorations = new vscode.EventEmitter<vscode.Uri | vscode.Uri[] | undefined>();
    readonly onDidChangeFileDecorations = this._onDidChangeFileDecorations.event;

    provideFileDecoration(uri: vscode.Uri): vscode.FileDecoration | undefined {
        if (uri.scheme !== 'snap-tree') {
            return undefined;
        }

        const params = new URLSearchParams(uri.query);
        const status = params.get('status');

        if (status === 'deleted') {
            return {
                badge: 'D',
                color: new vscode.ThemeColor('list.errorForeground'),
                tooltip: 'Deleted from current directory',
            };
        }

        if (status === 'modified') {
            return {
                badge: 'M',
                color: new vscode.ThemeColor('list.warningForeground'),
                tooltip: 'Modified since this snapshot',
            };
        }

        return undefined;
    }

    refresh(): void {
        this._onDidChangeFileDecorations.fire(undefined);
    }
}

export class SnapProvider implements vscode.TreeDataProvider<TreeNode> {
    private _onDidChangeTreeData = new vscode.EventEmitter<TreeNode | undefined | null>();
    readonly onDidChangeTreeData = this._onDidChangeTreeData.event;
    private parentMap = new Map<TreeNode, TreeNode | undefined>();

    constructor(private workspaceRoot: string) {}

    refresh(): void {
        this.parentMap.clear();
        this._onDidChangeTreeData.fire(undefined);
    }

    getTreeItem(element: TreeNode): vscode.TreeItem {
        return element;
    }

    getParent(element: TreeNode): TreeNode | undefined {
        return this.parentMap.get(element);
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
            categories.forEach(c => this.parentMap.set(c, undefined));
            return categories;
        }

        if (element instanceof CategoryItem) {
            const snapshots = await getSnapshots(this.workspaceRoot);
            const filtered = element.category === 'user'
                ? snapshots.filter(s => !s.autoSave)
                : snapshots.filter(s => s.autoSave);

            const items = filtered
                .reverse()
                .map((snap, index) => new SnapshotItem(snap.id, snap, index === 0));
            items.forEach(i => this.parentMap.set(i, element));
            return items;
        }

        if (element instanceof SnapshotItem) {
            const files = element.snapshot.files || [];
            const snapshotTree = element.snapshot.tree || {};
            const tree = buildFolderTree(files);
            const children = getCompactedChildren(tree, '', element.snapshotId, this.workspaceRoot, snapshotTree);
            children.forEach(c => this.parentMap.set(c, element));
            return children;
        }

        if (element instanceof FolderItem) {
            const folderPrefix = element.folderPath;
            const directFiles: string[] = [];
            const subfolders = new Map<string, string[]>();

            for (const filePath of element.files) {
                const relative = filePath.substring(folderPrefix.length + 1);
                const slashIdx = relative.indexOf('/');

                if (slashIdx === -1) {
                    directFiles.push(filePath);
                } else {
                    const subName = relative.substring(0, slashIdx);
                    if (!subfolders.has(subName)) {
                        subfolders.set(subName, []);
                    }
                    subfolders.get(subName)!.push(filePath);
                }
            }

            const nodes: TreeNode[] = [];

            for (const [name, files] of subfolders) {
                let displayName = name;
                let currentPath = `${folderPrefix}/${name}`;
                let currentFiles = files;

                while (true) {
                    const prefix2 = currentPath;
                    const directInThis: string[] = [];
                    const subsInThis = new Map<string, string[]>();

                    for (const f of currentFiles) {
                        const rel = f.substring(prefix2.length + 1);
                        const idx = rel.indexOf('/');
                        if (idx === -1) {
                            directInThis.push(f);
                        } else {
                            const sub = rel.substring(0, idx);
                            if (!subsInThis.has(sub)) {
                                subsInThis.set(sub, []);
                            }
                            subsInThis.get(sub)!.push(f);
                        }
                    }

                    if (directInThis.length === 0 && subsInThis.size === 1) {
                        const [childName, childFiles] = subsInThis.entries().next().value!;
                        displayName = `${displayName}/${childName}`;
                        currentPath = `${currentPath}/${childName}`;
                        currentFiles = childFiles;
                    } else {
                        break;
                    }
                }

                nodes.push(new FolderItem(displayName, currentPath, element.snapshotId, currentFiles, element.workspaceRoot, element.snapshotTree));
            }

            for (const filePath of directFiles) {
                const hash = element.snapshotTree[filePath] || '';
                nodes.push(new FileItem(filePath, element.snapshotId, element.workspaceRoot, hash));
            }

            nodes.forEach(n => this.parentMap.set(n, element));
            return nodes;
        }

        return [];
    }
}

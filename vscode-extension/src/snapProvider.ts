import * as vscode from 'vscode';
import * as fs from 'fs';
import * as crypto from 'crypto';
import { getSnapshots, SnapshotInfo } from './snapCli';
import * as path from 'path';

export class CategoryItem extends vscode.TreeItem {
    constructor(
        public readonly label: string,
        public readonly category: 'pinned' | 'user' | 'auto',
    ) {
        super(label, vscode.TreeItemCollapsibleState.Expanded);
        this.contextValue = 'category';
        if (category === 'pinned') {
            this.iconPath = new vscode.ThemeIcon('pinned');
        } else if (category === 'user') {
            this.iconPath = new vscode.ThemeIcon('bookmark');
        } else {
            this.iconPath = new vscode.ThemeIcon('history');
        }
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

        if (snapshot.pinned) {
            this.iconPath = new vscode.ThemeIcon('pinned');
        } else if (isLatest) {
            this.iconPath = new vscode.ThemeIcon('circle-large-filled');
        } else if (snapshot.autoSave) {
            this.iconPath = new vscode.ThemeIcon('circle-small');
        } else {
            this.iconPath = new vscode.ThemeIcon('circle-large-outline');
        }

        this.contextValue = snapshot.pinned ? 'snapshotPinned' : 'snapshot';
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
        this.iconPath = new vscode.ThemeIcon('folder');

        // Encode folder info in resourceUri for decoration provider
        const encodedFiles = encodeURIComponent(JSON.stringify(files));
        this.resourceUri = vscode.Uri.parse(`snap-tree://folder/${snapshotId}/${folderPath}?files=${encodedFiles}`);
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

        this.iconPath = new vscode.ThemeIcon('file');
        this.tooltip = `${filePath}\nClick to diff with current • Right-click for more`;
        this.contextValue = 'snapshotFile';

        // Encode hash in resourceUri for decoration provider to compute live status
        this.resourceUri = vscode.Uri.parse(`snap-tree://file/${snapshotId}/${filePath}?hash=${snapshotHash}`);

        this.command = {
            command: 'snap.diffFile',
            title: 'Diff File',
            arguments: [this],
        };
    }
}

function encodeURIComponent(str: string): string {
    return globalThis.encodeURIComponent(str);
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
    private workspaceRoot: string = '';
    private snapshotTrees = new Map<number, Record<string, string>>();

    setWorkspaceRoot(root: string): void {
        this.workspaceRoot = root;
    }

    updateSnapshotTrees(snapshots: SnapshotInfo[]): void {
        this.snapshotTrees.clear();
        for (const snap of snapshots) {
            this.snapshotTrees.set(snap.id, snap.tree);
        }
    }

    provideFileDecoration(uri: vscode.Uri): vscode.FileDecoration | undefined {
        if (uri.scheme !== 'snap-tree') {
            return undefined;
        }

        if (uri.authority === 'file') {
            const params = new URLSearchParams(uri.query);
            const snapshotHash = params.get('hash') || '';
            // Path format: /snapshotId/file/path
            const pathParts = uri.path.substring(1); // remove leading /
            const slashIdx = pathParts.indexOf('/');
            const filePath = pathParts.substring(slashIdx + 1);

            const status = this.computeFileStatus(filePath, snapshotHash);

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

        if (uri.authority === 'folder') {
            const params = new URLSearchParams(uri.query);
            let files: string[] = [];
            try {
                files = JSON.parse(decodeURIComponent(params.get('files') || '[]'));
            } catch {
                return undefined;
            }

            const pathParts = uri.path.substring(1);
            const slashIdx = pathParts.indexOf('/');
            const snapshotIdStr = pathParts.substring(0, slashIdx);
            const snapshotId = parseInt(snapshotIdStr, 10);
            const tree = this.snapshotTrees.get(snapshotId) || {};

            const status = this.computeFolderStatus(files, tree);

            if (status === 'deleted') {
                return {
                    badge: 'D',
                    color: new vscode.ThemeColor('list.errorForeground'),
                    tooltip: 'All files deleted from current directory',
                };
            }
            if (status === 'modified') {
                return {
                    badge: 'M',
                    color: new vscode.ThemeColor('list.warningForeground'),
                    tooltip: 'Contains modified or deleted files',
                };
            }
            return undefined;
        }

        return undefined;
    }

    private computeFileStatus(filePath: string, snapshotHash: string): 'same' | 'modified' | 'deleted' {
        const fullPath = path.join(this.workspaceRoot, filePath);
        try {
            if (!fs.existsSync(fullPath)) {
                return 'deleted';
            }
            const data = fs.readFileSync(fullPath);
            const currentHash = crypto.createHash('sha256').update(data).digest('hex');
            return currentHash !== snapshotHash ? 'modified' : 'same';
        } catch {
            return 'deleted';
        }
    }

    private computeFolderStatus(files: string[], tree: Record<string, string>): 'same' | 'modified' | 'deleted' {
        let allDeleted = true;
        let hasChanges = false;

        for (const filePath of files) {
            const hash = tree[filePath] || '';
            const status = this.computeFileStatus(filePath, hash);
            if (status !== 'deleted') {
                allDeleted = false;
            }
            if (status !== 'same') {
                hasChanges = true;
            }
        }

        if (allDeleted && files.length > 0) {
            return 'deleted';
        }
        return hasChanges ? 'modified' : 'same';
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
            const hasPinned = snapshots.some(s => s.pinned);
            const hasUser = snapshots.some(s => !s.autoSave && !s.pinned);
            const hasAuto = snapshots.some(s => s.autoSave && !s.pinned);

            const categories: TreeNode[] = [];
            if (hasPinned) {
                categories.push(new CategoryItem('Pinned', 'pinned'));
            }
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
            let filtered: SnapshotInfo[];

            if (element.category === 'pinned') {
                filtered = snapshots.filter(s => s.pinned);
            } else if (element.category === 'user') {
                filtered = snapshots.filter(s => !s.autoSave && !s.pinned);
            } else {
                filtered = snapshots.filter(s => s.autoSave && !s.pinned);
            }

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

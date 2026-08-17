import * as vscode from 'vscode';
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
    ) {
        super(folderName, vscode.TreeItemCollapsibleState.Collapsed);
        this.iconPath = new vscode.ThemeIcon('folder');
        this.tooltip = `${folderPath} (${files.length} files)`;
        this.contextValue = 'snapshotFolder';
    }
}

export class FileItem extends vscode.TreeItem {
    constructor(
        public readonly filePath: string,
        public readonly snapshotId: number,
        public readonly workspaceRoot: string,
    ) {
        const fileName = path.basename(filePath);
        super(fileName, vscode.TreeItemCollapsibleState.None);

        this.iconPath = new vscode.ThemeIcon('file');
        this.tooltip = `${filePath}\nClick to diff with current • Right-click for more`;
        this.description = '';
        this.contextValue = 'snapshotFile';

        this.command = {
            command: 'snap.diffFile',
            title: 'Diff File',
            arguments: [this],
        };
    }
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

// Collapse single-child folders: a/b/c with only one subfolder becomes "a/b/c"
function getCompactedChildren(tree: FolderTree, prefix: string, snapshotId: number, workspaceRoot: string): TreeNode[] {
    const nodes: TreeNode[] = [];

    for (const [name, subtree] of tree.subfolders) {
        let displayName = name;
        let currentPath = prefix ? `${prefix}/${name}` : name;
        let current = subtree;

        // Compact: if folder has only subfolders (no direct files) and exactly 1 subfolder, merge
        while (current.files.length === 0 && current.subfolders.size === 1) {
            const [childName, childTree] = current.subfolders.entries().next().value!;
            displayName = `${displayName}/${childName}`;
            currentPath = `${currentPath}/${childName}`;
            current = childTree;
        }

        const allFiles = collectAllFiles(current);
        nodes.push(new FolderItem(displayName, currentPath, snapshotId, allFiles, workspaceRoot));
    }

    // Direct files in this folder
    for (const filePath of tree.files) {
        nodes.push(new FileItem(filePath, snapshotId, workspaceRoot));
    }

    return nodes;
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
            const tree = buildFolderTree(files);
            const children = getCompactedChildren(tree, '', element.snapshotId, this.workspaceRoot);
            children.forEach(c => this.parentMap.set(c, element));
            return children;
        }

        if (element instanceof FolderItem) {
            // Build subtree for this folder's files
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

            // Sub-folders (with compaction)
            for (const [name, files] of subfolders) {
                let displayName = name;
                let currentPath = `${folderPrefix}/${name}`;
                let currentFiles = files;

                // Compact single-child folders
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

                nodes.push(new FolderItem(displayName, currentPath, element.snapshotId, currentFiles, element.workspaceRoot));
            }

            // Direct files
            for (const filePath of directFiles) {
                nodes.push(new FileItem(filePath, element.snapshotId, element.workspaceRoot));
            }

            nodes.forEach(n => this.parentMap.set(n, element));
            return nodes;
        }

        return [];
    }
}

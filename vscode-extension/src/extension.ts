import * as vscode from 'vscode';
import * as path from 'path';
import { SnapProvider, SnapDecorationProvider, SnapshotItem, FileItem } from './snapProvider';
import { ChangesProvider } from './changesProvider';
import { execSnap } from './snapCli';

class SnapContentProvider implements vscode.TextDocumentContentProvider {
    private contentMap = new Map<string, string>();

    setContent(uri: string, content: string) {
        this.contentMap.set(uri, content);
    }

    provideTextDocumentContent(uri: vscode.Uri): string {
        return this.contentMap.get(uri.toString()) || '';
    }
}

export function activate(context: vscode.ExtensionContext) {
    const workspaceRoot = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
    if (!workspaceRoot) {
        return;
    }

    const snapProvider = new SnapProvider(workspaceRoot);
    const changesProvider = new ChangesProvider(workspaceRoot);
    const contentProvider = new SnapContentProvider();
    const decorationProvider = new SnapDecorationProvider();

    const treeView = vscode.window.createTreeView('snapTimeline', {
        treeDataProvider: snapProvider,
    });
    context.subscriptions.push(treeView);
    context.subscriptions.push(vscode.window.registerFileDecorationProvider(decorationProvider));
    vscode.window.registerTreeDataProvider('snapChanges', changesProvider);

    context.subscriptions.push(
        vscode.workspace.registerTextDocumentContentProvider('snap', contentProvider)
    );

    context.subscriptions.push(
        vscode.commands.registerCommand('snap.init', async () => {
            const result = await execSnap(workspaceRoot, ['init']);
            if (result.success) {
                vscode.window.showInformationMessage('Snap initialized!');
                snapProvider.refresh();
            } else {
                vscode.window.showErrorMessage(`Init failed: ${result.error}`);
            }
        }),

        vscode.commands.registerCommand('snap.save', async () => {
            const message = await vscode.window.showInputBox({
                prompt: 'Checkpoint message',
                placeHolder: 'e.g., before auth refactor',
            });

            if (message === undefined) {
                return;
            }

            const msg = message || 'snapshot';

            const description = await vscode.window.showInputBox({
                prompt: 'Description (optional — press Enter to skip)',
                placeHolder: 'e.g., JWT working, refresh token pending',
            });

            const args = ['save', msg];
            if (description) {
                args.push('-d', description);
            }

            const result = await execSnap(workspaceRoot, args);
            if (result.success) {
                vscode.window.showInformationMessage(`Saved: "${msg}"`);
                snapProvider.refresh();
                changesProvider.refresh();
            } else {
                vscode.window.showErrorMessage(`Save failed: ${result.error}`);
            }
        }),

        vscode.commands.registerCommand('snap.restore', async (item?: SnapshotItem) => {
            let id: string | undefined;

            if (item) {
                id = item.snapshotId.toString();
            } else {
                id = await vscode.window.showInputBox({
                    prompt: 'Snapshot ID to restore',
                    placeHolder: 'e.g., 3',
                });
            }

            if (!id) {
                return;
            }

            const confirm = await vscode.window.showWarningMessage(
                `Restore to snapshot #${id}? Current state will be auto-saved first.`,
                'Restore',
                'Cancel'
            );

            if (confirm !== 'Restore') {
                return;
            }

            const result = await execSnap(workspaceRoot, ['restore', id]);
            if (result.success) {
                vscode.window.showInformationMessage(`Restored to #${id}`);
                snapProvider.refresh();
                changesProvider.refresh();
                decorationProvider.refresh();
            } else {
                vscode.window.showErrorMessage(`Restore failed: ${result.error}`);
            }
        }),

        vscode.commands.registerCommand('snap.showFile', async (item: FileItem) => {
            const result = await execSnap(workspaceRoot, ['show', item.snapshotId.toString(), item.filePath]);
            if (result.success) {
                let content = result.output;
                const headerEnd = content.indexOf('\n\n');
                if (headerEnd !== -1) {
                    content = content.substring(headerEnd + 2);
                }

                const uri = vscode.Uri.parse(`snap://snapshot/${item.snapshotId}/${item.filePath}`);
                contentProvider.setContent(uri.toString(), content);

                const doc = await vscode.workspace.openTextDocument(uri);
                await vscode.window.showTextDocument(doc, { preview: true, preserveFocus: false });

                // Make document readonly after opening
                await vscode.commands.executeCommand('workbench.action.files.setActiveEditorReadonlyInSession');
            } else {
                vscode.window.showErrorMessage(`Show failed: ${result.error}`);
            }
        }),

        vscode.commands.registerCommand('snap.diffFile', async (item: FileItem) => {
            const result = await execSnap(workspaceRoot, ['show', item.snapshotId.toString(), item.filePath]);
            if (!result.success) {
                vscode.window.showErrorMessage(`Failed: ${result.error}`);
                return;
            }

            let snapshotContent = result.output;
            const headerEnd = snapshotContent.indexOf('\n\n');
            if (headerEnd !== -1) {
                snapshotContent = snapshotContent.substring(headerEnd + 2);
            }

            const snapshotUri = vscode.Uri.parse(
                `snap://diff/${item.snapshotId}/${item.filePath}?ts=${Date.now()}`
            );
            contentProvider.setContent(snapshotUri.toString(), snapshotContent);

            const currentFilePath = path.join(workspaceRoot, item.filePath);
            const currentUri = vscode.Uri.file(currentFilePath);

            await vscode.commands.executeCommand(
                'vscode.diff',
                snapshotUri,
                currentUri,
                `#${item.snapshotId} ↔ Current: ${path.basename(item.filePath)}`,
                {
                    renderSideBySide: true,
                }
            );
        }),

        vscode.commands.registerCommand('snap.restoreFile', async (item: FileItem) => {
            const confirm = await vscode.window.showWarningMessage(
                `Restore ${item.filePath} from snapshot #${item.snapshotId}? This will overwrite the current file.`,
                'Restore',
                'Cancel'
            );

            if (confirm !== 'Restore') {
                return;
            }

            const result = await execSnap(workspaceRoot, ['restore-file', item.snapshotId.toString(), item.filePath]);
            if (result.success) {
                vscode.window.showInformationMessage(`Restored ${item.filePath} from #${item.snapshotId}`);
                snapProvider.refresh();
                decorationProvider.refresh();
            } else {
                vscode.window.showErrorMessage(`Restore failed: ${result.error}`);
            }
        }),

        vscode.commands.registerCommand('snap.saveFile', async (_uri?: vscode.Uri, uris?: vscode.Uri[]) => {
            let filePaths: string[] = [];

            if (uris && uris.length > 0) {
                // Called from explorer context menu with multi-select
                filePaths = uris.map(u => path.relative(workspaceRoot, u.fsPath));
            } else if (_uri) {
                // Single file from explorer
                filePaths = [path.relative(workspaceRoot, _uri.fsPath)];
            } else {
                // Called from editor context menu or command palette
                const editor = vscode.window.activeTextEditor;
                if (!editor) {
                    vscode.window.showErrorMessage('No active file to save');
                    return;
                }
                filePaths = [path.relative(workspaceRoot, editor.document.uri.fsPath)];
            }

            const label = filePaths.length === 1 ? filePaths[0] : `${filePaths.length} files`;
            const message = await vscode.window.showInputBox({
                prompt: `Save checkpoint for ${label}`,
                placeHolder: 'e.g., before changes',
            });

            if (message === undefined) {
                return;
            }

            const args = ['save-file', ...filePaths, '-m', message || 'file checkpoint'];
            const result = await execSnap(workspaceRoot, args);
            if (result.success) {
                vscode.window.showInformationMessage(`Saved checkpoint: ${label}`);
                snapProvider.refresh();
            } else {
                vscode.window.showErrorMessage(`Save failed: ${result.error}`);
            }
        }),

        vscode.commands.registerCommand('snap.delete', async (item?: SnapshotItem) => {
            let id: string | undefined;

            if (item) {
                id = item.snapshotId.toString();
            } else {
                id = await vscode.window.showInputBox({
                    prompt: 'Snapshot ID to delete',
                    placeHolder: 'e.g., 3',
                });
            }

            if (!id) {
                return;
            }

            const confirm = await vscode.window.showWarningMessage(
                `Delete snapshot #${id}?`,
                'Delete',
                'Cancel'
            );

            if (confirm !== 'Delete') {
                return;
            }

            const result = await execSnap(workspaceRoot, ['delete', id]);
            if (result.success) {
                vscode.window.showInformationMessage(`Deleted #${id}`);
                snapProvider.refresh();
            } else {
                vscode.window.showErrorMessage(`Delete failed: ${result.error}`);
            }
        }),

        vscode.commands.registerCommand('snap.refresh', () => {
            snapProvider.refresh();
            changesProvider.refresh();
            decorationProvider.refresh();
        })
    );

    const watcher = vscode.workspace.createFileSystemWatcher('**/*', false, false, false);
    let debounceTimer: NodeJS.Timeout | undefined;

    const debouncedRefresh = () => {
        if (debounceTimer) {
            clearTimeout(debounceTimer);
        }
        debounceTimer = setTimeout(() => {
            changesProvider.refresh();
            snapProvider.refresh();
            decorationProvider.refresh();
        }, 2000);
    };

    watcher.onDidChange(debouncedRefresh);
    watcher.onDidCreate(debouncedRefresh);
    watcher.onDidDelete(debouncedRefresh);
    context.subscriptions.push(watcher);
}

export function deactivate() {}

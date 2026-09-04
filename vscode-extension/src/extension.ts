import * as vscode from 'vscode';
import * as path from 'path';
import * as fs from 'fs';
import * as crypto from 'crypto';
import { SnapProvider, SnapDecorationProvider, SnapshotItem, FileItem } from './snapProvider';
import { ChangesProvider } from './changesProvider';
import { RecordingTimelineProvider, TimelineChangeItem } from './recordingProvider';
import { WatchProvider } from './watchProvider';
import { SearchViewProvider } from './searchViewProvider';
import { execSnap, isRecording, clearTimeline, searchFiles, grepContent } from './snapCli';

function computeFileStatus(workspaceRoot: string, filePath: string, snapshotHash: string): 'same' | 'modified' | 'deleted' {
    const fullPath = path.join(workspaceRoot, filePath);
    try {
        if (!fs.existsSync(fullPath)) { return 'deleted'; }
        const data = fs.readFileSync(fullPath);
        const currentHash = crypto.createHash('sha256').update(data).digest('hex');
        return currentHash !== snapshotHash ? 'modified' : 'same';
    } catch { return 'deleted'; }
}

function computeFileStatuses(workspaceRoot: string, tree: Record<string, string>): Record<string, string> {
    const statuses: Record<string, string> = {};
    for (const [filePath, hash] of Object.entries(tree)) {
        const s = computeFileStatus(workspaceRoot, filePath, hash);
        if (s !== 'same') { statuses[filePath] = s; }
    }
    return statuses;
}

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
    const recordingProvider = new RecordingTimelineProvider(workspaceRoot);
    const watchProvider = new WatchProvider(workspaceRoot);
    const contentProvider = new SnapContentProvider();
    const decorationProvider = new SnapDecorationProvider();
    decorationProvider.setWorkspaceRoot(workspaceRoot);

    // Status bar — recording toggle
    const recordingStatusBar = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 50);
    recordingStatusBar.command = 'snap.recordToggle';
    context.subscriptions.push(recordingStatusBar);

    const updateRecordingStatus = () => {
        const recording = isRecording(workspaceRoot);
        if (recording) {
            recordingStatusBar.text = '$(circle-filled) Snap: Recording';
            recordingStatusBar.color = new vscode.ThemeColor('statusBarItem.errorForeground');
            recordingStatusBar.backgroundColor = new vscode.ThemeColor('statusBarItem.errorBackground');
            recordingStatusBar.tooltip = 'Click to stop recording';
        } else {
            recordingStatusBar.text = '$(circle-outline) Snap: Not Recording';
            recordingStatusBar.color = undefined;
            recordingStatusBar.backgroundColor = undefined;
            recordingStatusBar.tooltip = 'Click to start recording';
        }
        recordingStatusBar.show();
    };
    updateRecordingStatus();

    // Refresh recording status periodically
    const recordingTimer = setInterval(updateRecordingStatus, 5000);
    context.subscriptions.push({ dispose: () => clearInterval(recordingTimer) });

    // Load snapshot trees for decoration provider
    const refreshDecorationData = async () => {
        const { getSnapshots } = await import('./snapCli');
        const snapshots = await getSnapshots(workspaceRoot);
        decorationProvider.updateSnapshotTrees(snapshots);
        decorationProvider.refresh();
    };
    refreshDecorationData();

    const treeView = vscode.window.createTreeView('snapTimeline', {
        treeDataProvider: snapProvider,
    });
    context.subscriptions.push(treeView);
    context.subscriptions.push(vscode.window.registerFileDecorationProvider(decorationProvider));
    vscode.window.registerTreeDataProvider('snapChanges', changesProvider);
    vscode.window.registerTreeDataProvider('snapRecordingTimeline', recordingProvider);
    vscode.window.registerTreeDataProvider('snapWatchedFiles', watchProvider);

    const searchViewProvider = new SearchViewProvider(context.extensionUri, {
        onSearch: async (query, opts) => {
            searchViewProvider.setLoading(true);

            const { getSnapshots } = await import('./snapCli');
            const allSnapshots = await getSnapshots(workspaceRoot);
            const snapMetaMap = new Map<number, { message: string; description: string; time: string; fileCount: number; pinned: boolean }>();
            for (const s of allSnapshots) {
                snapMetaMap.set(s.id, {
                    message: s.message,
                    description: s.description,
                    time: new Date(s.timestamp).toLocaleString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }),
                    fileCount: s.fileCount,
                    pinned: s.pinned,
                });
            }

            if (opts.mode === 'checkpoint') {
                const matchStr = opts.caseSensitive
                    ? (haystack: string) => haystack.includes(query)
                    : (haystack: string) => haystack.toLowerCase().includes(query.toLowerCase());
                const matched = allSnapshots.filter(s =>
                    matchStr(s.message) ||
                    matchStr(s.description) ||
                    s.id.toString() === query
                ).map(s => ({
                    id: s.id,
                    message: s.message,
                    description: s.description,
                    time: snapMetaMap.get(s.id)!.time,
                    fileCount: s.fileCount,
                    pinned: s.pinned,
                    autoSave: s.autoSave,
                    files: s.files,
                    tree: s.tree,
                    fileStatuses: computeFileStatuses(workspaceRoot, s.tree),
                    isLatest: s.id === allSnapshots[allSnapshots.length - 1]?.id,
                }));
                searchViewProvider.setResults({ type: 'checkpoint', query, results: matched });
            } else if (opts.mode === 'file') {
                const results = await searchFiles(workspaceRoot, query, { caseSensitive: opts.caseSensitive });
                const enriched = results.map(r => ({
                    ...r,
                    checkpoint_meta: snapMetaMap.get(r.snapshot_id) || null,
                }));
                searchViewProvider.setResults({ type: 'file', query, results: enriched });
            } else {
                const results = await grepContent(workspaceRoot, query, {
                    caseSensitive: opts.caseSensitive,
                    regex: opts.regex,
                });
                const enriched = results.map(r => ({
                    ...r,
                    checkpoint_meta: snapMetaMap.get(r.snapshot_id) || null,
                }));
                searchViewProvider.setResults({ type: 'content', query, results: enriched });
            }
        },
        onOpenFile: async (filePath, snapshotId, line?) => {
            const currentFilePath = path.join(workspaceRoot, filePath);
            const fs = await import('fs');

            if (line && fs.existsSync(currentFilePath)) {
                const uri = vscode.Uri.file(currentFilePath);
                const doc = await vscode.workspace.openTextDocument(uri);
                const editor = await vscode.window.showTextDocument(doc, { preview: true });
                const lineIdx = Math.max(0, line - 1);
                const range = new vscode.Range(lineIdx, 0, lineIdx, 0);
                editor.selection = new vscode.Selection(range.start, range.start);
                editor.revealRange(range, vscode.TextEditorRevealType.InCenter);
            } else {
                const result = await execSnap(workspaceRoot, ['show', snapshotId.toString(), filePath]);
                if (!result.success) {
                    vscode.window.showErrorMessage(`Failed: ${result.error}`);
                    return;
                }
                let content = result.output;
                const headerEnd = content.indexOf('\n\n');
                if (headerEnd !== -1) { content = content.substring(headerEnd + 2); }
                const uri = vscode.Uri.parse(`snap://search/${snapshotId}/${filePath}?ts=${Date.now()}`);
                contentProvider.setContent(uri.toString(), content);
                const doc = await vscode.workspace.openTextDocument(uri);
                const editor = await vscode.window.showTextDocument(doc, { preview: true });
                await vscode.commands.executeCommand('workbench.action.files.setActiveEditorReadonlyInSession');
                if (line) {
                    const lineIdx = Math.max(0, line - 1);
                    const range = new vscode.Range(lineIdx, 0, lineIdx, 0);
                    editor.selection = new vscode.Selection(range.start, range.start);
                    editor.revealRange(range, vscode.TextEditorRevealType.InCenter);
                }
            }
        },
        onDiffFile: async (filePath, snapshotId) => {
            const result = await execSnap(workspaceRoot, ['show', snapshotId.toString(), filePath]);
            if (!result.success) {
                vscode.window.showErrorMessage(`Failed: ${result.error}`);
                return;
            }
            let snapshotContent = result.output;
            const headerEnd = snapshotContent.indexOf('\n\n');
            if (headerEnd !== -1) { snapshotContent = snapshotContent.substring(headerEnd + 2); }
            const snapshotUri = vscode.Uri.parse(`snap://search-diff/${snapshotId}/${filePath}?ts=${Date.now()}`);
            contentProvider.setContent(snapshotUri.toString(), snapshotContent);
            const currentUri = vscode.Uri.file(path.join(workspaceRoot, filePath));
            await vscode.commands.executeCommand('vscode.diff', snapshotUri, currentUri,
                `#${snapshotId} ↔ Current: ${path.basename(filePath)}`, { renderSideBySide: true });
        },
        onOpenCheckpoint: async (snapshotId) => {
            const result = await execSnap(workspaceRoot, ['show', snapshotId.toString()]);
            if (result.success) {
                const outputChannel = vscode.window.createOutputChannel(`Snap #${snapshotId}`);
                outputChannel.append(result.output);
                outputChannel.show();
            }
        },
        onRestoreFile: async (filePath, snapshotId) => {
            const confirm = await vscode.window.showWarningMessage(
                `Restore ${path.basename(filePath)} from checkpoint #${snapshotId}?`,
                { modal: true },
                'Restore'
            );
            if (confirm !== 'Restore') { return; }
            const result = await execSnap(workspaceRoot, ['restore-file', snapshotId.toString(), filePath]);
            if (result.success) {
                vscode.window.showInformationMessage(`Restored ${path.basename(filePath)} from #${snapshotId}`);
                snapProvider.refresh();
                changesProvider.refresh();
            } else {
                vscode.window.showErrorMessage(`Restore failed: ${result.error}`);
            }
        },
    });
    context.subscriptions.push(
        vscode.window.registerWebviewViewProvider(SearchViewProvider.viewType, searchViewProvider)
    );

    treeView.onDidChangeVisibility(e => {
        if (e.visible) {
            snapProvider.refresh();
            changesProvider.refresh();
            recordingProvider.refresh();
            watchProvider.refresh();
            refreshDecorationData();
            updateBadge();
            updateRecordingStatus();
        }
    });

    const updateBadge = async () => {
        const { getStatus, getSnapshots } = await import('./snapCli');
        const changes = await getStatus(workspaceRoot);
        if (changes.length > 0) {
            const snapshots = await getSnapshots(workspaceRoot);
            const lastFull = [...snapshots].reverse().find(s => s.fileCount > 5) || snapshots[snapshots.length - 1];
            const label = lastFull ? `#${lastFull.id} "${lastFull.message}"` : 'last snapshot';
            treeView.badge = { value: changes.length, tooltip: `${changes.length} files changed since ${label}` };
        } else {
            treeView.badge = undefined;
        }
    };
    updateBadge();

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
                updateBadge();
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
                refreshDecorationData();
                updateBadge();
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

        vscode.commands.registerCommand('snap.compareFile', async (item: FileItem) => {
            const { getSnapshots } = await import('./snapCli');
            const allSnapshots = await getSnapshots(workspaceRoot);

            const candidates = allSnapshots.filter(
                s => s.id !== item.snapshotId && s.files.includes(item.filePath)
            );

            if (candidates.length === 0) {
                vscode.window.showInformationMessage('No other checkpoints contain this file.');
                return;
            }

            const picked = await vscode.window.showQuickPick(
                candidates.map(s => ({
                    label: `#${s.id} — ${s.message}`,
                    description: `${new Date(s.timestamp).toLocaleString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })} • ${s.fileCount} files`,
                    id: s.id,
                })),
                { placeHolder: `Compare ${path.basename(item.filePath)} — select another checkpoint` }
            );

            if (!picked) { return; }

            const [resultA, resultB] = await Promise.all([
                execSnap(workspaceRoot, ['show', item.snapshotId.toString(), item.filePath]),
                execSnap(workspaceRoot, ['show', picked.id.toString(), item.filePath]),
            ]);

            if (!resultA.success || !resultB.success) {
                vscode.window.showErrorMessage('Failed to fetch file content from checkpoints.');
                return;
            }

            const parseContent = (raw: string) => {
                const headerEnd = raw.indexOf('\n\n');
                return headerEnd !== -1 ? raw.substring(headerEnd + 2) : raw;
            };

            const contentA = parseContent(resultA.output);
            const contentB = parseContent(resultB.output);

            const uriA = vscode.Uri.parse(`snap://compare/${item.snapshotId}/${item.filePath}?ts=${Date.now()}`);
            const uriB = vscode.Uri.parse(`snap://compare/${picked.id}/${item.filePath}?ts=${Date.now()}`);

            contentProvider.setContent(uriA.toString(), contentA);
            contentProvider.setContent(uriB.toString(), contentB);

            await vscode.commands.executeCommand(
                'vscode.diff',
                uriA,
                uriB,
                `#${item.snapshotId} ↔ #${picked.id}: ${path.basename(item.filePath)}`,
                { renderSideBySide: true }
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
                refreshDecorationData();
                updateBadge();
            } else {
                vscode.window.showErrorMessage(`Restore failed: ${result.error}`);
            }
        }),

        vscode.commands.registerCommand('snap.saveFile', async (_uri?: vscode.Uri, uris?: vscode.Uri[]) => {
            let filePaths: string[] = [];

            if (uris && uris.length > 0) {
                filePaths = uris.map(u => path.relative(workspaceRoot, u.fsPath));
            } else if (_uri) {
                filePaths = [path.relative(workspaceRoot, _uri.fsPath)];
            } else {
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
                updateBadge();
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
                refreshDecorationData();
                updateBadge();
            } else {
                vscode.window.showErrorMessage(`Delete failed: ${result.error}`);
            }
        }),

        vscode.commands.registerCommand('snap.pin', async (item?: SnapshotItem) => {
            if (!item) { return; }
            const result = await execSnap(workspaceRoot, ['pin', item.snapshotId.toString()]);
            if (result.success) {
                vscode.window.showInformationMessage(`Pinned #${item.snapshotId}`);
                snapProvider.refresh();
                refreshDecorationData();
            } else {
                vscode.window.showErrorMessage(`Pin failed: ${result.error}`);
            }
        }),

        vscode.commands.registerCommand('snap.unpin', async (item?: SnapshotItem) => {
            if (!item) { return; }
            const result = await execSnap(workspaceRoot, ['unpin', item.snapshotId.toString()]);
            if (result.success) {
                vscode.window.showInformationMessage(`Unpinned #${item.snapshotId}`);
                snapProvider.refresh();
                refreshDecorationData();
            } else {
                vscode.window.showErrorMessage(`Unpin failed: ${result.error}`);
            }
        }),

        vscode.commands.registerCommand('snap.export', async (item?: SnapshotItem) => {
            if (!item) { return; }

            const password = await vscode.window.showInputBox({
                prompt: 'Password (optional — leave empty for no encryption)',
                placeHolder: 'Press Enter to skip',
                password: true,
            });

            if (password === undefined) { return; }

            const saveUri = await vscode.window.showSaveDialog({
                defaultUri: vscode.Uri.file(path.join(workspaceRoot, `checkpoint-${item.snapshotId}.snap`)),
                filters: { 'Snap Files': ['snap'] },
            });

            if (!saveUri) { return; }

            const args = ['export', item.snapshotId.toString(), '-o', saveUri.fsPath];
            if (password) {
                args.push('-p', password);
            }

            const result = await execSnap(workspaceRoot, args);
            if (result.success) {
                const msg = password ? 'Exported (encrypted)' : 'Exported';
                vscode.window.showInformationMessage(`${msg}: ${path.basename(saveUri.fsPath)}`);
            } else {
                vscode.window.showErrorMessage(`Export failed: ${result.error}`);
            }
        }),

        vscode.commands.registerCommand('snap.import', async () => {
            const fileUris = await vscode.window.showOpenDialog({
                canSelectMany: false,
                filters: { 'Snap Files': ['snap'] },
                openLabel: 'Import',
            });

            if (!fileUris || fileUris.length === 0) { return; }

            const filePath = fileUris[0].fsPath;

            const password = await vscode.window.showInputBox({
                prompt: 'Password (leave empty if not encrypted)',
                placeHolder: 'Press Enter if no password',
                password: true,
            });

            if (password === undefined) { return; }

            const args = ['import', filePath];
            if (password) {
                args.push('-p', password);
            }

            const result = await execSnap(workspaceRoot, args);
            if (result.success) {
                vscode.window.showInformationMessage('Imported checkpoint successfully!');
                snapProvider.refresh();
                refreshDecorationData();
                updateBadge();
            } else {
                vscode.window.showErrorMessage(`Import failed: ${result.error}`);
            }
        }),

        // ── Recording commands ──

        vscode.commands.registerCommand('snap.recordToggle', async () => {
            const recording = isRecording(workspaceRoot);
            if (recording) {
                const result = await execSnap(workspaceRoot, ['record', 'stop']);
                if (result.success) {
                    vscode.window.showInformationMessage('Recording stopped');
                } else {
                    vscode.window.showErrorMessage(`Stop failed: ${result.error}`);
                }
            } else {
                const result = await execSnap(workspaceRoot, ['record', 'start']);
                if (result.success) {
                    vscode.window.showInformationMessage('Recording started — all file changes are being tracked');
                } else {
                    vscode.window.showErrorMessage(`Start failed: ${result.error}`);
                }
            }
            updateRecordingStatus();
            recordingProvider.refresh();
        }),

        vscode.commands.registerCommand('snap.recordStart', async () => {
            const result = await execSnap(workspaceRoot, ['record', 'start']);
            if (result.success) {
                vscode.window.showInformationMessage('Recording started');
            } else {
                vscode.window.showErrorMessage(`Start failed: ${result.error}`);
            }
            updateRecordingStatus();
            recordingProvider.refresh();
        }),

        vscode.commands.registerCommand('snap.recordStop', async () => {
            const result = await execSnap(workspaceRoot, ['record', 'stop']);
            if (result.success) {
                vscode.window.showInformationMessage('Recording stopped');
            } else {
                vscode.window.showErrorMessage(`Stop failed: ${result.error}`);
            }
            updateRecordingStatus();
            recordingProvider.refresh();
        }),

        // ── Rewind ──

        vscode.commands.registerCommand('snap.rewind', async () => {
            const options = [
                { label: '1 minute ago', value: '1 minute ago' },
                { label: '5 minutes ago', value: '5 minutes ago' },
                { label: '15 minutes ago', value: '15 minutes ago' },
                { label: '30 minutes ago', value: '30 minutes ago' },
                { label: '1 hour ago', value: '1 hour ago' },
                { label: 'Custom time...', value: '__custom__' },
            ];

            const picked = await vscode.window.showQuickPick(options, {
                placeHolder: 'Rewind to...',
            });

            if (!picked) { return; }

            let timeStr = picked.value;
            if (timeStr === '__custom__') {
                const custom = await vscode.window.showInputBox({
                    prompt: 'Enter time (e.g., "14:30", "2:47 PM", "10 minutes ago")',
                    placeHolder: '14:30',
                });
                if (!custom) { return; }
                timeStr = custom;
            }

            const confirm = await vscode.window.showWarningMessage(
                `Rewind to "${timeStr}"? Current state will be auto-saved first.`,
                'Rewind',
                'Cancel'
            );

            if (confirm !== 'Rewind') { return; }

            const result = await execSnap(workspaceRoot, ['rewind', timeStr]);
            if (result.success) {
                vscode.window.showInformationMessage(`Rewound to ${timeStr}`);
                snapProvider.refresh();
                changesProvider.refresh();
                refreshDecorationData();
                updateBadge();
            } else {
                vscode.window.showErrorMessage(`Rewind failed: ${result.error}`);
            }
        }),

        // ── Timeline change actions ──

        vscode.commands.registerCommand('snap.diffTimelineChange', async (item: TimelineChangeItem) => {
            const fs = await import('fs');
            const change = item.change;

            if (!change.newHash) {
                vscode.window.showInformationMessage('No content available for this change.');
                return;
            }

            // Read the object content via snap CLI by finding a snapshot that has this hash
            // Simpler: read the object directly from the store
            const objectPath = path.join(workspaceRoot, '.snap', 'objects', change.newHash.substring(0, 2), change.newHash.substring(2));

            if (!fs.existsSync(objectPath)) {
                vscode.window.showErrorMessage('Object not found in store.');
                return;
            }

            const zlib = await import('zlib');
            const compressed = fs.readFileSync(objectPath);
            const content = zlib.inflateSync(compressed).toString();

            const time = new Date(change.timestamp);
            const timeStr = time.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });

            const snapshotUri = vscode.Uri.parse(`snap://timeline/${timeStr}/${change.path}?ts=${Date.now()}`);
            contentProvider.setContent(snapshotUri.toString(), content);

            const currentFilePath = path.join(workspaceRoot, change.path);
            if (fs.existsSync(currentFilePath)) {
                const currentUri = vscode.Uri.file(currentFilePath);
                await vscode.commands.executeCommand(
                    'vscode.diff',
                    snapshotUri,
                    currentUri,
                    `${timeStr} ↔ Current: ${path.basename(change.path)}`,
                    { renderSideBySide: true }
                );
            } else {
                const doc = await vscode.workspace.openTextDocument(snapshotUri);
                await vscode.window.showTextDocument(doc, { preview: true });
                await vscode.commands.executeCommand('workbench.action.files.setActiveEditorReadonlyInSession');
            }
        }),

        vscode.commands.registerCommand('snap.showTimelineFile', async (item: TimelineChangeItem) => {
            const fs = await import('fs');
            const zlib = await import('zlib');
            const change = item.change;

            if (!change.newHash) {
                vscode.window.showInformationMessage('No content available (file was deleted).');
                return;
            }

            const objectPath = path.join(workspaceRoot, '.snap', 'objects', change.newHash.substring(0, 2), change.newHash.substring(2));
            if (!fs.existsSync(objectPath)) {
                vscode.window.showErrorMessage('Object not found in store.');
                return;
            }

            const compressed = fs.readFileSync(objectPath);
            const content = zlib.inflateSync(compressed).toString();

            const time = new Date(change.timestamp);
            const timeStr = time.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });

            const uri = vscode.Uri.parse(`snap://timeline-show/${timeStr}/${change.path}?ts=${Date.now()}`);
            contentProvider.setContent(uri.toString(), content);

            const doc = await vscode.workspace.openTextDocument(uri);
            await vscode.window.showTextDocument(doc, { preview: true });
            await vscode.commands.executeCommand('workbench.action.files.setActiveEditorReadonlyInSession');
        }),

        vscode.commands.registerCommand('snap.diffTimelinePrevious', async (item: TimelineChangeItem) => {
            const fs = await import('fs');
            const zlib = await import('zlib');
            const change = item.change;

            const readObject = (hash: string): string | null => {
                const objPath = path.join(workspaceRoot, '.snap', 'objects', hash.substring(0, 2), hash.substring(2));
                if (!fs.existsSync(objPath)) { return null; }
                const compressed = fs.readFileSync(objPath);
                return zlib.inflateSync(compressed).toString();
            };

            const time = new Date(change.timestamp);
            const timeStr = time.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });

            if (change.oldHash && change.newHash) {
                const oldContent = readObject(change.oldHash);
                const newContent = readObject(change.newHash);

                if (oldContent === null || newContent === null) {
                    vscode.window.showErrorMessage('Could not read object from store.');
                    return;
                }

                const uriOld = vscode.Uri.parse(`snap://timeline-old/${timeStr}/${change.path}?ts=${Date.now()}`);
                const uriNew = vscode.Uri.parse(`snap://timeline-new/${timeStr}/${change.path}?ts=${Date.now()}`);
                contentProvider.setContent(uriOld.toString(), oldContent);
                contentProvider.setContent(uriNew.toString(), newContent);

                await vscode.commands.executeCommand(
                    'vscode.diff',
                    uriOld,
                    uriNew,
                    `Before ↔ After (${timeStr}): ${path.basename(change.path)}`,
                    { renderSideBySide: true }
                );
            } else if (change.newHash) {
                const content = readObject(change.newHash);
                if (content === null) {
                    vscode.window.showErrorMessage('Could not read object from store.');
                    return;
                }
                const uri = vscode.Uri.parse(`snap://timeline-created/${timeStr}/${change.path}?ts=${Date.now()}`);
                contentProvider.setContent(uri.toString(), content);
                const doc = await vscode.workspace.openTextDocument(uri);
                await vscode.window.showTextDocument(doc, { preview: true });
                await vscode.commands.executeCommand('workbench.action.files.setActiveEditorReadonlyInSession');
            } else {
                vscode.window.showInformationMessage('No previous version available.');
            }
        }),

        vscode.commands.registerCommand('snap.restoreTimelineFile', async (item: TimelineChangeItem) => {
            const fs = await import('fs');
            const zlib = await import('zlib');
            const change = item.change;

            if (!change.newHash) {
                vscode.window.showInformationMessage('Cannot restore a deleted file from timeline.');
                return;
            }

            const confirm = await vscode.window.showWarningMessage(
                `Restore ${change.path} to its state at this point in the timeline?`,
                'Restore',
                'Cancel'
            );

            if (confirm !== 'Restore') { return; }

            const objectPath = path.join(workspaceRoot, '.snap', 'objects', change.newHash.substring(0, 2), change.newHash.substring(2));
            if (!fs.existsSync(objectPath)) {
                vscode.window.showErrorMessage('Object not found in store.');
                return;
            }

            const compressed = fs.readFileSync(objectPath);
            const content = zlib.inflateSync(compressed);

            const fullPath = path.join(workspaceRoot, change.path);
            const dir = path.dirname(fullPath);
            fs.mkdirSync(dir, { recursive: true });
            fs.writeFileSync(fullPath, content);

            vscode.window.showInformationMessage(`Restored ${change.path}`);
            changesProvider.refresh();
            decorationProvider.refresh();
            updateBadge();
        }),

        // ── Watch commands ──

        vscode.commands.registerCommand('snap.watchFile', async (_uri?: vscode.Uri) => {
            let filePath: string | undefined;

            if (_uri) {
                filePath = path.relative(workspaceRoot, _uri.fsPath);
            } else {
                const editor = vscode.window.activeTextEditor;
                if (editor) {
                    filePath = path.relative(workspaceRoot, editor.document.uri.fsPath);
                }
            }

            if (!filePath) {
                filePath = await vscode.window.showInputBox({
                    prompt: 'File path to watch',
                    placeHolder: 'e.g., config/config.go',
                });
            }

            if (!filePath) { return; }

            const result = await execSnap(workspaceRoot, ['watch', filePath]);
            if (result.success) {
                vscode.window.showInformationMessage(`Watching: ${filePath}`);
                watchProvider.refresh();
            } else {
                vscode.window.showErrorMessage(`Watch failed: ${result.error}`);
            }
        }),

        vscode.commands.registerCommand('snap.unwatchFile', async (item?: any) => {
            let filePath: string | undefined;

            if (item && item.filePath) {
                filePath = item.filePath;
            } else {
                const { getWatchlist } = await import('./snapCli');
                const files = await getWatchlist(workspaceRoot);
                if (files.length === 0) {
                    vscode.window.showInformationMessage('No files being watched.');
                    return;
                }

                const picked = await vscode.window.showQuickPick(
                    files.map(f => ({ label: f })),
                    { placeHolder: 'Select file to unwatch' }
                );
                if (!picked) { return; }
                filePath = picked.label;
            }

            if (!filePath) { return; }

            const result = await execSnap(workspaceRoot, ['watch', 'rm', filePath]);
            if (result.success) {
                vscode.window.showInformationMessage(`Unwatched: ${filePath}`);
                watchProvider.refresh();
            } else {
                vscode.window.showErrorMessage(`Unwatch failed: ${result.error}`);
            }
        }),

        // ── Clean ──

        vscode.commands.registerCommand('snap.clean', async () => {
            const dryRun = await execSnap(workspaceRoot, ['clean', '--dry-run']);
            if (!dryRun.success) {
                vscode.window.showErrorMessage(`Clean analysis failed: ${dryRun.error}`);
                return;
            }

            const lines = dryRun.output.split('\n');
            const removeLine = lines.find(l => l.includes('Safe to remove'));
            const spaceLine = lines.find(l => l.includes('Space to free'));

            const removeCount = removeLine?.match(/(\d+)/)?.[1] || '0';
            const space = spaceLine?.match(/:\s+(.+)/)?.[1]?.trim() || '0 B';

            if (removeCount === '0') {
                vscode.window.showInformationMessage('Already clean — nothing to remove.');
                return;
            }

            const action = await vscode.window.showInformationMessage(
                `Snap Clean: ${removeCount} snapshots safe to remove. Free ${space}.`,
                'Clean Now',
                'Show Details',
                'Cancel'
            );

            if (action === 'Show Details') {
                const outputChannel = vscode.window.createOutputChannel('Snap Clean');
                outputChannel.append(dryRun.output);
                outputChannel.show();
                return;
            }

            if (action !== 'Clean Now') { return; }

            const result = await execSnap(workspaceRoot, ['clean', '--auto']);
            if (result.success) {
                vscode.window.showInformationMessage(`Cleaned: freed ${space}`);
                snapProvider.refresh();
                refreshDecorationData();
                updateBadge();
            } else {
                vscode.window.showErrorMessage(`Clean failed: ${result.error}`);
            }
        }),

        // ── Clear Timeline ──

        vscode.commands.registerCommand('snap.clearTimeline', async () => {
            const confirm = await vscode.window.showWarningMessage(
                'Clear all recording timeline data? This cannot be undone.',
                'Clear',
                'Cancel'
            );

            if (confirm !== 'Clear') { return; }

            const success = await clearTimeline(workspaceRoot);
            if (success) {
                vscode.window.showInformationMessage('Recording timeline cleared.');
                recordingProvider.refresh();
            } else {
                vscode.window.showErrorMessage('Failed to clear timeline.');
            }
        }),

        // ── Save Timeline Entry as Checkpoint ──

        vscode.commands.registerCommand('snap.saveTimelineAsCheckpoint', async (item: TimelineChangeItem) => {
            const change = item.change;
            if (!change.newHash) {
                vscode.window.showInformationMessage('Cannot save a delete entry as checkpoint.');
                return;
            }

            const time = new Date(change.timestamp);
            const timeStr = time.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });

            const message = await vscode.window.showInputBox({
                prompt: 'Checkpoint message',
                value: `${change.path} at ${timeStr}`,
            });

            if (message === undefined) { return; }

            const result = await execSnap(workspaceRoot, ['save-file', change.path, '-m', message || `timeline: ${change.path} at ${timeStr}`]);
            if (result.success) {
                vscode.window.showInformationMessage(`Saved as checkpoint: ${change.path}`);
                snapProvider.refresh();
                updateBadge();
            } else {
                vscode.window.showErrorMessage(`Save failed: ${result.error}`);
            }
        }),

        // ── Refresh ──

        vscode.commands.registerCommand('snap.refresh', () => {
            snapProvider.refresh();
            changesProvider.refresh();
            recordingProvider.refresh();
            watchProvider.refresh();
            refreshDecorationData();
            updateBadge();
            updateRecordingStatus();
        }),

        vscode.commands.registerCommand('snap.refreshTimeline', () => {
            recordingProvider.refresh();
        }),

        vscode.commands.registerCommand('snap.refreshWatch', () => {
            watchProvider.refresh();
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
            decorationProvider.refresh();
            updateBadge();
            if (isRecording(workspaceRoot)) {
                recordingProvider.refresh();
            }
        }, 2000);
    };

    watcher.onDidChange(debouncedRefresh);
    watcher.onDidCreate(debouncedRefresh);
    watcher.onDidDelete(debouncedRefresh);
    context.subscriptions.push(watcher);
}

export function deactivate() {}

import * as vscode from 'vscode';
import * as path from 'path';
import { getTimelineChanges, TimelineChange } from './snapCli';

export class TimelineChangeItem extends vscode.TreeItem {
    constructor(
        public readonly change: TimelineChange,
        public readonly workspaceRoot: string,
    ) {
        const fileName = path.basename(change.path);
        super(fileName, vscode.TreeItemCollapsibleState.None);

        const time = new Date(change.timestamp);
        const timeStr = time.toLocaleTimeString('en-US', {
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit',
            hour12: false,
        });

        this.description = `${timeStr}  ${change.action}`;

        const agentTag = change.isAgent ? ' [agent]' : '';
        this.tooltip = `${change.path}\n${timeStr} — ${change.action}${agentTag}\nClick to diff with current`;

        switch (change.action) {
            case 'create':
                this.iconPath = new vscode.ThemeIcon('diff-added', new vscode.ThemeColor('gitDecoration.addedResourceForeground'));
                break;
            case 'modify':
                this.iconPath = new vscode.ThemeIcon('diff-modified', new vscode.ThemeColor('gitDecoration.modifiedResourceForeground'));
                break;
            case 'delete':
                this.iconPath = new vscode.ThemeIcon('diff-removed', new vscode.ThemeColor('gitDecoration.deletedResourceForeground'));
                break;
        }

        if (change.isAgent) {
            this.description += '  [agent]';
        }

        this.contextValue = 'timelineChange';

        if (change.action !== 'delete' && change.newHash) {
            this.command = {
                command: 'snap.diffTimelineChange',
                title: 'Diff with Current',
                arguments: [this],
            };
        }
    }
}

export class TimelineGapItem extends vscode.TreeItem {
    constructor(gapMinutes: number) {
        const label = gapMinutes >= 60
            ? `── ${Math.floor(gapMinutes / 60)}h ${gapMinutes % 60}m gap ──`
            : `── ${gapMinutes}m gap ──`;
        super(label, vscode.TreeItemCollapsibleState.None);
        this.iconPath = new vscode.ThemeIcon('ellipsis');
        this.contextValue = 'timelineGap';
    }
}

type TimelineNode = TimelineChangeItem | TimelineGapItem;

export class RecordingTimelineProvider implements vscode.TreeDataProvider<TimelineNode> {
    private _onDidChangeTreeData = new vscode.EventEmitter<TimelineNode | undefined | null>();
    readonly onDidChangeTreeData = this._onDidChangeTreeData.event;

    constructor(private workspaceRoot: string) {}

    refresh(): void {
        this._onDidChangeTreeData.fire(undefined);
    }

    getTreeItem(element: TimelineNode): vscode.TreeItem {
        return element;
    }

    async getChildren(): Promise<TimelineNode[]> {
        const changes = await getTimelineChanges(this.workspaceRoot);
        if (changes.length === 0) {
            return [];
        }

        const nodes: TimelineNode[] = [];
        const recent = changes.slice(-50);

        for (let i = 0; i < recent.length; i++) {
            if (i > 0) {
                const prev = new Date(recent[i - 1].timestamp).getTime();
                const curr = new Date(recent[i].timestamp).getTime();
                const gapMs = curr - prev;
                const gapMin = Math.floor(gapMs / 60000);
                if (gapMin >= 5) {
                    nodes.push(new TimelineGapItem(gapMin));
                }
            }
            nodes.push(new TimelineChangeItem(recent[i], this.workspaceRoot));
        }

        return nodes.reverse();
    }
}

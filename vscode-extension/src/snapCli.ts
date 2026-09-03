import { execFile } from 'child_process';

export interface SnapResult {
    success: boolean;
    output: string;
    error: string;
}

export function execSnap(workspaceRoot: string, args: string[]): Promise<SnapResult> {
    return new Promise((resolve) => {
        execFile('snap', args, { cwd: workspaceRoot }, (error, stdout, stderr) => {
            if (error) {
                resolve({
                    success: false,
                    output: stdout,
                    error: stderr || error.message,
                });
            } else {
                resolve({
                    success: true,
                    output: stdout,
                    error: '',
                });
            }
        });
    });
}

export interface SnapshotInfo {
    id: number;
    message: string;
    description: string;
    timestamp: string;
    fileCount: number;
    autoSave: boolean;
    pinned: boolean;
    files: string[];
    tree: Record<string, string>;
}

export async function getSnapshots(workspaceRoot: string): Promise<SnapshotInfo[]> {
    const path = await import('path');
    const fs = await import('fs');
    const snapshotsDir = path.join(workspaceRoot, '.snap', 'snapshots');

    if (!fs.existsSync(snapshotsDir)) {
        return [];
    }

    const entries = fs.readdirSync(snapshotsDir)
        .filter((f: string) => f.endsWith('.json'))
        .sort();

    const snapshots: SnapshotInfo[] = [];

    for (const file of entries) {
        const content = fs.readFileSync(path.join(snapshotsDir, file), 'utf-8');
        const data = JSON.parse(content);
        const files = data.tree ? Object.keys(data.tree).sort() : [];
        snapshots.push({
            id: data.id,
            message: data.message,
            description: data.description || '',
            timestamp: data.timestamp,
            fileCount: data.file_count,
            autoSave: data.auto_save || false,
            pinned: data.pinned || false,
            files,
            tree: data.tree || {},
        });
    }

    return snapshots;
}

export interface TimelineChange {
    timestamp: string;
    path: string;
    action: string;
    oldHash: string;
    newHash: string;
    fullSize: number;
    isAgent: boolean;
}

export async function getTimelineChanges(workspaceRoot: string): Promise<TimelineChange[]> {
    const pathMod = await import('path');
    const fs = await import('fs');
    const zlib = await import('zlib');

    const timelineDir = pathMod.join(workspaceRoot, '.snap', 'timeline');
    if (!fs.existsSync(timelineDir)) {
        return [];
    }

    const segments = fs.readdirSync(timelineDir)
        .filter((f: string) => f.endsWith('.seg'))
        .sort();

    const allChanges: TimelineChange[] = [];

    for (const seg of segments) {
        try {
            const compressed = fs.readFileSync(pathMod.join(timelineDir, seg));
            const data = zlib.inflateSync(compressed);
            const changes: any[] = JSON.parse(data.toString());
            for (const c of changes) {
                allChanges.push({
                    timestamp: c.ts,
                    path: c.path,
                    action: c.action,
                    oldHash: c.old_hash || '',
                    newHash: c.new_hash || '',
                    fullSize: c.full_size || 0,
                    isAgent: c.is_agent || false,
                });
            }
        } catch {
            continue;
        }
    }

    allChanges.sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime());

    // Detect agent bursts (5+ changes in 3 seconds)
    for (let i = 0; i < allChanges.length; i++) {
        let count = 1;
        const startTime = new Date(allChanges[i].timestamp).getTime();
        for (let j = i + 1; j < allChanges.length; j++) {
            if (new Date(allChanges[j].timestamp).getTime() - startTime <= 3000) {
                count++;
            } else {
                break;
            }
        }
        if (count >= 5) {
            for (let j = i; j < i + count && j < allChanges.length; j++) {
                allChanges[j].isAgent = true;
            }
        }
    }

    return allChanges;
}

export function isRecording(workspaceRoot: string): boolean {
    const pathMod = require('path');
    const fs = require('fs');
    const pidPath = pathMod.join(workspaceRoot, '.snap', 'recorder.pid');
    return fs.existsSync(pidPath);
}

export async function getWatchlist(workspaceRoot: string): Promise<string[]> {
    const pathMod = await import('path');
    const fs = await import('fs');
    const watchFile = pathMod.join(workspaceRoot, '.snap', 'watchlist.json');
    if (!fs.existsSync(watchFile)) {
        return [];
    }
    try {
        const data = fs.readFileSync(watchFile, 'utf-8');
        return JSON.parse(data);
    } catch {
        return [];
    }
}

export async function clearTimeline(workspaceRoot: string): Promise<boolean> {
    const pathMod = await import('path');
    const fs = await import('fs');
    const timelineDir = pathMod.join(workspaceRoot, '.snap', 'timeline');
    if (!fs.existsSync(timelineDir)) {
        return true;
    }
    try {
        const entries = fs.readdirSync(timelineDir);
        for (const entry of entries) {
            if (entry.endsWith('.seg')) {
                fs.unlinkSync(pathMod.join(timelineDir, entry));
            }
        }
        return true;
    } catch {
        return false;
    }
}

export async function getStatus(workspaceRoot: string): Promise<string[]> {
    const result = await execSnap(workspaceRoot, ['status']);
    if (!result.success) {
        return [];
    }

    const lines = result.output.split('\n');
    const changes: string[] = [];

    for (const line of lines) {
        const trimmed = line.trim();
        if (trimmed.startsWith('+') || trimmed.startsWith('~') || trimmed.startsWith('-')) {
            changes.push(trimmed);
        }
    }

    return changes;
}

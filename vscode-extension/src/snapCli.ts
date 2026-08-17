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
            files,
            tree: data.tree || {},
        });
    }

    return snapshots;
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

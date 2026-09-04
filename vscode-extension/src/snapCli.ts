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

// ── Search: fuzzy file name ──

export interface FileSearchResult {
    snapshot_id: number;
    path: string;
    score: number;
    hash: string;
}

export async function searchFiles(workspaceRoot: string, query: string, opts?: { caseSensitive?: boolean }): Promise<FileSearchResult[]> {
    const snapshots = await getSnapshots(workspaceRoot);
    if (!snapshots.length) { return []; }

    const cs = opts?.caseSensitive || false;
    const q = cs ? query : query.toLowerCase();
    const results: FileSearchResult[] = [];

    for (const s of snapshots) {
        for (const [p, h] of Object.entries(s.tree)) {
            const target = cs ? p : p.toLowerCase();
            const score = substringScore(q, target);
            if (score > -100) {
                results.push({ snapshot_id: s.id, path: p, score, hash: h });
            }
        }
    }

    results.sort((a, b) => b.score - a.score);
    return results;
}

function substringScore(queryLower: string, targetLower: string): number {
    const idx = targetLower.indexOf(queryLower);
    if (idx === -1) { return -100; }

    let score = 100;
    const lastSlash = targetLower.lastIndexOf('/');
    const filename = lastSlash >= 0 ? targetLower.substring(lastSlash + 1) : targetLower;

    if (filename === queryLower) { score += 50; }
    else if (filename.startsWith(queryLower)) { score += 30; }
    else if (idx > lastSlash) { score += 20; }

    if (idx === 0 || '/\\._- '.includes(targetLower[idx - 1])) { score += 10; }

    score -= idx;

    return score;
}

// ── Search: content grep ──

export interface ContentSearchResult {
    snapshot_id: number;
    path: string;
    line: number;
    content: string;
}

export async function grepContent(workspaceRoot: string, pattern: string, opts: { caseSensitive: boolean; regex: boolean }): Promise<ContentSearchResult[]> {
    const pathMod = await import('path');
    const fs = await import('fs');
    const zlib = await import('zlib');

    const snapshots = await getSnapshots(workspaceRoot);
    if (!snapshots.length) { return []; }

    // Dedup by (path, hash) — each unique file version appears once with its latest checkpoint
    const versionMap = new Map<string, { snapId: number; path: string; hash: string }>();
    for (const s of snapshots) {
        for (const [p, h] of Object.entries(s.tree)) {
            const key = `${p}\0${h}`;
            versionMap.set(key, { snapId: s.id, path: p, hash: h });
        }
    }

    const hashToFiles = new Map<string, { snapId: number; path: string }[]>();
    for (const [, entry] of versionMap) {
        const list = hashToFiles.get(entry.hash) || [];
        list.push({ snapId: entry.snapId, path: entry.path });
        hashToFiles.set(entry.hash, list);
    }

    let matcher: (line: string) => boolean;
    if (opts.regex) {
        try {
            const re = new RegExp(pattern, opts.caseSensitive ? '' : 'i');
            matcher = (line) => re.test(line);
        } catch {
            return [];
        }
    } else if (opts.caseSensitive) {
        matcher = (line) => line.includes(pattern);
    } else {
        const patLower = pattern.toLowerCase();
        matcher = (line) => line.toLowerCase().includes(patLower);
    }

    const objectsDir = pathMod.join(workspaceRoot, '.snap', 'objects');
    const results: ContentSearchResult[] = [];

    const binaryExts = new Set([
        '.png', '.jpg', '.jpeg', '.gif', '.bmp', '.ico', '.webp', '.svg',
        '.mp3', '.mp4', '.wav', '.ogg', '.webm', '.avi', '.mov', '.flac',
        '.zip', '.gz', '.tar', '.bz2', '.7z', '.rar', '.xz',
        '.pdf', '.doc', '.docx', '.xls', '.xlsx', '.ppt', '.pptx',
        '.exe', '.dll', '.so', '.dylib', '.bin', '.dat',
        '.woff', '.woff2', '.ttf', '.otf', '.eot',
        '.pyc', '.pyo', '.class', '.o', '.obj', '.a', '.lib',
        '.vsix', '.snap', '.db', '.sqlite', '.sqlite3',
    ]);

    for (const [hash, files] of hashToFiles) {
        const ext = files[0].path.substring(files[0].path.lastIndexOf('.')).toLowerCase();
        if (binaryExts.has(ext)) { continue; }

        const objPath = pathMod.join(objectsDir, hash.substring(0, 2), hash.substring(2));
        if (!fs.existsSync(objPath)) { continue; }

        let content: string;
        try {
            const compressed = fs.readFileSync(objPath);
            content = zlib.inflateSync(compressed).toString('utf-8');
        } catch {
            continue;
        }

        const first512 = content.substring(0, 512);
        let nullCount = 0;
        for (let i = 0; i < first512.length; i++) {
            if (first512.charCodeAt(i) === 0) { nullCount++; }
        }
        if (nullCount > first512.length / 8) { continue; }

        const lines = content.split('\n');
        const matchingLines: { num: number; text: string }[] = [];

        for (let i = 0; i < lines.length; i++) {
            if (matcher(lines[i])) {
                let trimmed = lines[i];
                if (trimmed.length > 200) { trimmed = trimmed.substring(0, 200) + '...'; }
                matchingLines.push({ num: i + 1, text: trimmed });
            }
        }

        if (matchingLines.length > 0) {
            for (const fe of files) {
                for (const ml of matchingLines) {
                    results.push({
                        snapshot_id: fe.snapId,
                        path: fe.path,
                        line: ml.num,
                        content: ml.text,
                    });
                }
            }
        }
    }

    results.sort((a, b) => a.path < b.path ? -1 : a.path > b.path ? 1 : a.line - b.line);
    return results;
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

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { AnimatePresence } from 'framer-motion';
import Layout from './components/Layout';
import DashboardOverview from './components/DashboardOverview';
import AccountList from './components/AccountList';
import WindowExplorer from './components/WindowExplorer';
import SettingsPanel from './components/SettingsPanel';
import { api } from './api';
import type { DirectoryInfo, HealthResponse, MetaResponse, ScanHistoryEntry, ScanSnapshot, ThemeType } from './types';
import './index.css';

type ViewKey = 'dashboard' | 'accounts' | 'windows' | 'settings';

interface AppSettings {
  theme: ThemeType;
  directory: string;
  fullValueUSD: number;
  autoConcurrency: boolean;
  manualConcurrency: number;
  autoRefreshEnabled: boolean;
  autoRefreshMinutes: number;
  pageSize: number;
}

interface ScanProgressState {
  jobId: string;
  done: number;
  total: number;
  percent: number;
  message: string;
}

interface PendingFolderImport {
  id: string;
  folderName: string;
  entries: Array<{ file: File; relativePath: string }>;
  fileCount: number;
}

interface PendingFolderImportSummary {
	id: string;
	folderName: string;
	fileCount: number;
}

const SETTINGS_KEY = 'codex-overview-ui-settings';
const HISTORY_KEY = 'codex-overview-scan-history';
const AUTOSCAN_PAUSED_KEY = 'codex-overview-autoscan-paused';
const LAST_SNAPSHOT_KEY = 'codex-overview-last-snapshot';

function defaultSettings(): AppSettings {
  return {
    theme: 'ocean',
    directory: '',
    fullValueUSD: 7.5,
    autoConcurrency: true,
    manualConcurrency: 0,
    autoRefreshEnabled: false,
    autoRefreshMinutes: 30,
    pageSize: 20,
  };
}

function loadSettings(): AppSettings {
  try {
    const raw = localStorage.getItem(SETTINGS_KEY);
    if (!raw) return defaultSettings();
    return { ...defaultSettings(), ...(JSON.parse(raw) as Partial<AppSettings>), autoRefreshEnabled: false };
  } catch {
    return defaultSettings();
  }
}

function loadHistory(): ScanHistoryEntry[] {
  try {
    const raw = localStorage.getItem(HISTORY_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as ScanHistoryEntry[];
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function loadAutoScanPaused(): boolean {
  try {
    return localStorage.getItem(AUTOSCAN_PAUSED_KEY) === '1';
  } catch {
    return false;
  }
}

function loadLastSnapshot(): ScanSnapshot | null {
  try {
    const raw = localStorage.getItem(LAST_SNAPSHOT_KEY);
    if (!raw) return null;
    return JSON.parse(raw) as ScanSnapshot;
  } catch {
    return null;
  }
}

function isAbsoluteDirectory(value: string): boolean {
	const trimmed = value.trim();
	return /^[a-zA-Z]:[\\/]/.test(trimmed) || trimmed.startsWith('\\\\') || trimmed.startsWith('/');
}

function resolveDirectoryCandidate(meta: MetaResponse | null, preferred: string): string {
	const trimmed = preferred.trim();
	if (!meta) return trimmed;
	if (!trimmed) return meta.defaultDirectory || '';
	if (isAbsoluteDirectory(trimmed)) return trimmed;
	if (meta.directories.some((item) => item.name === trimmed)) return trimmed;
	return meta.defaultDirectory || '';
}

function buildCombinedImportName(folderCount: number): string {
	const now = new Date();
	const stamp = [
		now.getFullYear(),
		String(now.getMonth() + 1).padStart(2, '0'),
		String(now.getDate()).padStart(2, '0'),
		String(now.getHours()).padStart(2, '0'),
		String(now.getMinutes()).padStart(2, '0'),
		String(now.getSeconds()).padStart(2, '0'),
	].join('');
	return `批量导入_${folderCount}个文件夹_${stamp}`;
}

function App() {
  const [activeView, setActiveView] = useState<ViewKey>('dashboard');
  const [settings, setSettings] = useState<AppSettings>(loadSettings);
  const [meta, setMeta] = useState<MetaResponse | null>(null);
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [snapshot, setSnapshot] = useState<ScanSnapshot | null>(loadLastSnapshot);
  const [history, setHistory] = useState<ScanHistoryEntry[]>(loadHistory);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState(loadAutoScanPaused() ? '已暂停自动扫描，等待你手动点击“立即扫描”。' : '');
  const [progress, setProgress] = useState<ScanProgressState | null>(null);
  const [pendingImports, setPendingImports] = useState<PendingFolderImport[]>([]);
  const [autoScanPaused, setAutoScanPaused] = useState(loadAutoScanPaused);
  const folderInputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    document.body.setAttribute('data-theme', settings.theme);
  }, [settings.theme]);

  useEffect(() => {
    localStorage.setItem(SETTINGS_KEY, JSON.stringify(settings));
  }, [settings]);

  useEffect(() => {
    localStorage.setItem(HISTORY_KEY, JSON.stringify(history.slice(0, 12)));
  }, [history]);

  useEffect(() => {
    if (!snapshot) {
      localStorage.removeItem(LAST_SNAPSHOT_KEY);
      return;
    }
    localStorage.setItem(LAST_SNAPSHOT_KEY, JSON.stringify(snapshot));
  }, [snapshot]);

  useEffect(() => {
    if (autoScanPaused) {
      localStorage.setItem(AUTOSCAN_PAUSED_KEY, '1');
      return;
    }
    localStorage.removeItem(AUTOSCAN_PAUSED_KEY);
  }, [autoScanPaused]);

  const loadMeta = useCallback(async () => {
    const [metaResponse, healthResponse] = await Promise.all([api.getMeta(), api.getHealth()]);
    setMeta(metaResponse);
    setHealth(healthResponse);
	setHistory((prev) => prev.filter((entry) => metaResponse.directories.some((item) => item.name === entry.directory)));
    setSettings((prev) => {
	  const resolvedDirectory = resolveDirectoryCandidate(metaResponse, prev.directory);
	  return {
        ...prev,
        directory: resolvedDirectory,
        fullValueUSD: prev.fullValueUSD > 0 ? prev.fullValueUSD : metaResponse.defaultPrice,
        manualConcurrency:
          prev.manualConcurrency > 0 ? prev.manualConcurrency : metaResponse.system.recommendedConcurrency,
	  };
    });
    return metaResponse;
  }, []);

  const pushHistory = useCallback((result: ScanSnapshot) => {
    const entry: ScanHistoryEntry = {
      id: `${result.directory}-${result.scannedAt}`,
      directory: result.directory,
      scannedAt: result.scannedAt,
      durationMs: result.durationMs,
      totalAccounts: result.summary.totalAccounts,
      successCount: result.summary.successCount,
      failedCount: result.summary.failedCount,
      totalValueUSD: result.summary.totalValueUSD,
      concurrencyUsed: result.concurrencyUsed,
    };
    setHistory((prev) => [entry, ...prev.filter((item) => item.id !== entry.id)].slice(0, 12));
  }, []);

  const importPendingFolders = useCallback(async () => {
    if (pendingImports.length === 0) {
      return [] as string[];
    }
	  const expectedFileCount = pendingImports.reduce((sum, item) => sum + item.fileCount, 0);
	  const importedNames: string[] = [];
	  if (pendingImports.length === 1) {
		  const [batch] = pendingImports;
		  try {
			  const result = await api.importFolderEntries(batch.entries, batch.folderName);
			  if (result.imported.jsonCount !== expectedFileCount) {
				  throw new Error(`导入数量不完整：预计 ${expectedFileCount} 个，实际导入 ${result.imported.jsonCount} 个`);
			  }
			  importedNames.push(result.imported.name);
			  setNotice(`已导入文件夹“${result.imported.name}”，共 ${result.imported.jsonCount} 个 JSON 文件。`);
		  } catch (error) {
			  const message = error instanceof Error ? error.message : '未知错误';
			  throw new Error(`导入文件夹“${batch.folderName}”失败：${message}`);
		  }
	  } else {
		  const mergedEntries = pendingImports.flatMap((item) => item.entries);
		  const batchName = buildCombinedImportName(pendingImports.length);
		  try {
			  const result = await api.importFolderEntries(mergedEntries, batchName);
			  if (result.imported.jsonCount !== expectedFileCount) {
				  throw new Error(`导入数量不完整：预计 ${expectedFileCount} 个，实际导入 ${result.imported.jsonCount} 个`);
			  }
			  importedNames.push(result.imported.name);
			  setNotice(`已合并导入 ${pendingImports.length} 个文件夹，共 ${result.imported.jsonCount} 个 JSON 文件。`);
		  } catch (error) {
			  const message = error instanceof Error ? error.message : '未知错误';
			  throw new Error(`批量导入 ${pendingImports.length} 个文件夹失败：${message}`);
		  }
	  }
    const refreshedMeta = await loadMeta();
    setPendingImports([]);
    return importedNames.length > 0 ? importedNames : (refreshedMeta.defaultDirectory ? [refreshedMeta.defaultDirectory] : []);
  }, [loadMeta, pendingImports]);

  const runJob = useCallback(async (refresh: boolean, accountIds: string[] = [], source: 'manual' | 'auto' = 'manual') => {
    if (source === 'auto' && autoScanPaused) {
      return;
    }
    if (source === 'manual' && autoScanPaused) {
      setAutoScanPaused(false);
      setNotice('已恢复手动扫描，正在重新统计最新数据。');
    }

    const currentMeta = meta ?? await loadMeta();
    let directory = resolveDirectoryCandidate(currentMeta, settings.directory);
    setLoading(true);
    setError('');
    if (source === 'manual') {
      setNotice('');
    }
    setProgress(null);
    try {
      if (pendingImports.length > 0 && accountIds.length === 0) {
        const importedNames = await importPendingFolders();
        if (importedNames.length > 0) {
          directory = importedNames[0];
          setSettings((prev) => ({ ...prev, directory }));
        }
      }
      if (!directory) {
        setError('请先选择文件夹，或确认当前目录下存在可扫描的认证文件夹。');
        return;
      }
      if (directory !== settings.directory) {
        setSettings((prev) => (prev.directory === directory ? prev : { ...prev, directory }));
      }
      const payload = {
        directory,
        resultId: snapshot?.resultId,
        fullValueUSD: settings.fullValueUSD,
        autoConcurrency: settings.autoConcurrency,
        concurrency: settings.autoConcurrency ? 0 : settings.manualConcurrency,
        force: true,
        accountIds,
      };
      const starter = refresh || accountIds.length > 0 ? api.startRefreshJob : api.startScanJob;
      const { jobId } = await starter(payload);
      setProgress({ jobId, done: 0, total: 0, percent: 0, message: '任务已启动' });

      let completed = false;
      while (!completed) {
        await new Promise((resolve) => window.setTimeout(resolve, 450));
        const job = await api.getJob(jobId);
        setProgress({ jobId, done: job.done, total: job.total, percent: job.percent, message: job.message || '' });
        if (job.status === 'failed') {
          throw new Error(job.message || '扫描任务失败');
        }
        if (job.status === 'completed' && job.snapshot) {
          setSnapshot(job.snapshot);
          pushHistory(job.snapshot);
          setNotice(`扫描完成：${job.snapshot.summary.totalAccounts} 个账户，成功 ${job.snapshot.summary.successCount} 个，失败 ${job.snapshot.summary.failedCount} 个。`);
          completed = true;
        }
      }
    } catch (scanError) {
      setNotice('');
      setError(scanError instanceof Error ? scanError.message : '扫描失败');
    } finally {
      setLoading(false);
      setProgress(null);
    }
  }, [autoScanPaused, importPendingFolders, loadMeta, meta, pendingImports.length, pushHistory, settings.autoConcurrency, settings.directory, settings.fullValueUSD, settings.manualConcurrency, snapshot?.resultId]);

  useEffect(() => {
    void loadMeta();
  }, [loadMeta]);

  useEffect(() => {
    if (!meta || !snapshot) return;
    const exists = meta.directories.some((item) => item.name === snapshot.directory);
    if (!exists) {
      setSnapshot(null);
    }
  }, [meta, snapshot]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      api.getHealth().then(setHealth).catch(() => undefined);
    }, 15000);
    return () => window.clearInterval(timer);
  }, []);

  const handleSettingsChange = useCallback((partial: Partial<AppSettings>) => {
    setSettings((prev) => ({ ...prev, ...partial, autoRefreshEnabled: false }));
  }, []);

  const handleExport = useCallback(() => {
	const directory = resolveDirectoryCandidate(meta, settings.directory);
    if (!directory) return;
    const url = api.exportCsvUrl({
      directory,
      fullValueUSD: settings.fullValueUSD,
      autoConcurrency: settings.autoConcurrency,
      concurrency: settings.autoConcurrency ? 0 : settings.manualConcurrency,
      force: false,
    });
    window.open(url, '_blank');
  }, [meta?.defaultDirectory, settings.autoConcurrency, settings.directory, settings.fullValueUSD, settings.manualConcurrency]);

  const selectedDirectoryInfo = useMemo<DirectoryInfo | null>(() => {
    return meta?.directories.find((item) => item.name === settings.directory) ?? null;
  }, [meta?.directories, settings.directory]);

  const importedDirectoryCount = useMemo(() => {
    return meta?.directories.filter((item) => item.imported).length ?? 0;
  }, [meta?.directories]);

  const handleImportFolder = useCallback(async (files: FileList | null) => {
    if (!files || files.length === 0) return;
    const first = files[0] as File & { webkitRelativePath?: string };
    const folderName = first?.webkitRelativePath?.split('/')[0] || `导入目录_${Date.now()}`;
    const entries = Array.from(files).map((file) => ({
      file,
      relativePath: (file as File & { webkitRelativePath?: string }).webkitRelativePath || file.name,
    }));
    setPendingImports((prev) => {
      const next = prev.filter((item) => item.folderName !== folderName);
      next.push({
        id: `${folderName}-${Date.now()}`,
        folderName,
        entries,
        fileCount: entries.length,
      });
      return next;
    });
    setError('');
    setNotice(`已暂存文件夹“${folderName}”，共 ${entries.length} 个 JSON 文件，点击“立即扫描”后会统一导入。`);
    if (folderInputRef.current) folderInputRef.current.value = '';
  }, []);

	const triggerFolderPicker = useCallback(() => {
		folderInputRef.current?.click();
	}, []);

  const clearPendingImports = useCallback(() => {
    setPendingImports([]);
    setError('');
    setNotice('已清空待导入文件夹队列。');
    if (folderInputRef.current) {
      folderInputRef.current.value = '';
    }
  }, []);

  const removePendingImport = useCallback((id: string) => {
    setPendingImports((prev) => prev.filter((item) => item.id !== id));
  }, []);

  const handleDeleteDirectory = useCallback(async () => {
    if (!selectedDirectoryInfo?.imported) return;
    setLoading(true);
    setError('');
    setNotice('');
    try {
      await api.deleteDirectory(selectedDirectoryInfo.name);
      const refreshedMeta = await loadMeta();
      const nextDirectory = refreshedMeta.defaultDirectory;
      setSettings((prev) => ({ ...prev, directory: nextDirectory }));
      setSnapshot(null);
      setNotice(`已删除导入目录“${selectedDirectoryInfo.name}”。`);
    } catch (deleteError) {
      setNotice('');
      setError(deleteError instanceof Error ? deleteError.message : '删除目录失败');
    } finally {
      setLoading(false);
    }
  }, [loadMeta, selectedDirectoryInfo?.imported, selectedDirectoryInfo?.name]);

  const handleClearImportedFiles = useCallback(async () => {
    setLoading(true);
    setError('');
    setNotice('');
    try {
      const response = await api.clearImportedFiles();
      const refreshedMeta = await loadMeta();
      setPendingImports([]);
      if (folderInputRef.current) {
        folderInputRef.current.value = '';
      }
      const nextDirectory = resolveDirectoryCandidate(refreshedMeta, settings.directory);
      setSettings((prev) => ({ ...prev, directory: nextDirectory }));
      if (!snapshot || !refreshedMeta.directories.some((item) => item.name === snapshot.directory)) {
        setSnapshot(null);
      }
      setNotice(response.removedCount > 0 ? `已清空 ${response.removedCount} 个已导入目录。` : '当前没有可清空的已导入目录。');
    } catch (clearError) {
      setNotice('');
      setError(clearError instanceof Error ? clearError.message : '清空账号文件失败');
    } finally {
      setLoading(false);
    }
  }, [loadMeta, settings.directory, snapshot]);

  const handleClearAllStats = useCallback(async () => {
    setLoading(true);
    setError('');
    setNotice('');
    try {
      await api.clearStats();
      setSnapshot(null);
      setProgress(null);
      setHistory([]);
      localStorage.removeItem(HISTORY_KEY);
      localStorage.removeItem(LAST_SNAPSHOT_KEY);
      setAutoScanPaused(true);
      const healthResponse = await api.getHealth();
      setHealth(healthResponse);
      setNotice('已清空所有统计数据，并暂停自动扫描。刷新页面后仍会保持清空状态，直到你手动点击“立即扫描”。');
    } catch (clearError) {
      setNotice('');
      setError(clearError instanceof Error ? clearError.message : '清空统计数据失败');
    } finally {
      setLoading(false);
    }
  }, []);

  const content = useMemo(() => {
    switch (activeView) {
      case 'accounts':
        return (
          <AccountList
            resultId={snapshot?.resultId}
            loading={loading}
            autoRefreshEnabled={settings.autoRefreshEnabled}
            autoRefreshMinutes={settings.autoRefreshMinutes}
            pageSize={settings.pageSize}
            onPageSizeChange={(pageSize) => handleSettingsChange({ pageSize })}
            onAutoRefreshChange={(enabled) => handleSettingsChange({ autoRefreshEnabled: enabled })}
            onAutoRefreshMinutesChange={(minutes) => handleSettingsChange({ autoRefreshMinutes: minutes })}
            onRefreshAll={() => runJob(false)}
            onRefreshSelected={(ids) => runJob(true, ids)}
          />
        );
      case 'windows':
        return (
          <WindowExplorer
            resultId={snapshot?.resultId}
            loading={loading}
            pageSize={settings.pageSize}
            onPageSizeChange={(pageSize) => handleSettingsChange({ pageSize })}
          />
        );
      case 'settings':
        return (
          <SettingsPanel
            meta={meta}
            health={health}
            settings={settings}
            loading={loading}
            onChange={handleSettingsChange}
            onApply={() => runJob(false)}
            onPickFolder={triggerFolderPicker}
            onDeleteDirectory={handleDeleteDirectory}
            canDeleteDirectory={Boolean(selectedDirectoryInfo?.imported)}
            selectedDirectoryPath={selectedDirectoryInfo?.path ?? ''}
          />
        );
      case 'dashboard':
      default:
        return (
          <DashboardOverview
            meta={meta}
            health={health}
            snapshot={snapshot}
            history={history}
            loading={loading}
            error={error}
            notice={notice}
            progress={progress}
            pendingImportCount={pendingImports.length}
            pendingImportNames={pendingImports.map((item) => item.folderName)}
            pendingImports={pendingImports.map<PendingFolderImportSummary>((item) => ({
              id: item.id,
              folderName: item.folderName,
              fileCount: item.fileCount,
            }))}
            selectedDirectory={settings.directory}
            selectedDirectoryPath={selectedDirectoryInfo?.path ?? ''}
            fullValueUSD={settings.fullValueUSD}
            onFullValueUSDChange={(value) => handleSettingsChange({ fullValueUSD: value })}
            onRefresh={() => runJob(false)}
            onSelectHistory={(entry) => handleSettingsChange({ directory: entry.directory })}
            onPickFolder={triggerFolderPicker}
            onClearPendingImports={clearPendingImports}
            onClearImportedFiles={handleClearImportedFiles}
            onClearAllStats={handleClearAllStats}
            onRemovePendingImport={removePendingImport}
            onDeleteDirectory={handleDeleteDirectory}
            canDeleteDirectory={Boolean(selectedDirectoryInfo?.imported)}
            canClearImportedFiles={importedDirectoryCount > 0}
          />
        );
    }
  }, [activeView, clearPendingImports, error, handleClearAllStats, handleClearImportedFiles, handleDeleteDirectory, handleSettingsChange, health, history, importedDirectoryCount, loading, meta, notice, pendingImports, progress, removePendingImport, runJob, selectedDirectoryInfo?.imported, selectedDirectoryInfo?.path, settings, snapshot, triggerFolderPicker]);

  return (
    <>
      <input
        ref={(node) => {
          folderInputRef.current = node;
          if (node) {
            node.setAttribute('webkitdirectory', 'true');
            node.setAttribute('directory', 'true');
          }
        }}
        id="folder-picker"
        name="folderPicker"
        type="file"
        multiple
        style={{ display: 'none' }}
        onChange={(event) => void handleImportFolder(event.target.files)}
      />
      <Layout
        activeView={activeView}
        onChangeView={setActiveView}
        currentTheme={settings.theme}
        onThemeChange={(theme) => handleSettingsChange({ theme })}
        onExport={handleExport}
        onRefreshAll={() => runJob(false)}
        isRefreshing={loading}
        health={health}
        subtitle={progress ? `${progress.message || '扫描中'} · ${progress.percent.toFixed(1)}%` : error ? `操作失败：${error}` : notice ? notice : snapshot ? `最近扫描：${new Date(snapshot.scannedAt).toLocaleString('zh-CN')}` : '等待首次扫描'}
      >
        <AnimatePresence mode="wait">
          <div className="flex flex-col gap-8">{content}</div>
        </AnimatePresence>
      </Layout>
    </>
  );
}

export default App;

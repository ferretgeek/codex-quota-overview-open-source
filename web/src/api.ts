import type {
  AccountsPageResponse,
  ClearImportedFilesResponse,
  ClearStatsResponse,
  DeleteDirectoryResponse,
  HealthResponse,
  ImportFolderResponse,
  MetaResponse,
  ScanJob,
  ScanRequest,
  ScanSnapshot,
} from './types';

const API_BASE = import.meta.env.VITE_API_BASE?.trim() || '';
const IMPORT_CHUNK_SIZE = 200;
const RETRYABLE_FETCH_PATTERN = /Failed to fetch|NetworkError|Load failed|fetch failed/i;

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

function isRetryableFetchError(error: unknown): boolean {
  if (error instanceof TypeError) return true;
  const message = error instanceof Error ? error.message : String(error ?? '');
  return RETRYABLE_FETCH_PATTERN.test(message);
}

async function fetchWithRetry(input: RequestInfo | URL, init?: RequestInit, attempts = 3): Promise<Response> {
  let lastError: unknown;
  for (let attempt = 1; attempt <= attempts; attempt += 1) {
    try {
      return await fetch(input, init);
    } catch (error) {
      lastError = error;
      if (!isRetryableFetchError(error) || attempt === attempts) {
        throw error;
      }
      await sleep(250 * attempt);
    }
  }
  throw lastError instanceof Error ? lastError : new Error('Fetch failed');
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetchWithRetry(`${API_BASE}${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
  });

  if (!response.ok) {
    let message = `HTTP ${response.status}`;
    try {
      const payload = (await response.json()) as { error?: string };
      if (payload?.error) {
        message = payload.error;
      }
    } catch {
      const text = await response.text();
      if (text.trim()) message = `${message} ${text.trim()}`;
    }
    throw new Error(message);
  }

  return (await response.json()) as T;
}

async function postImportFolderChunk(
  entries: Array<{ file: File; relativePath: string }>,
  folderName?: string,
  append = false,
): Promise<ImportFolderResponse> {
  const formData = new FormData();
  formData.append('folderName', folderName || `import_${Date.now()}`);
  formData.append('append', append ? 'true' : 'false');

  entries.forEach((entry) => {
    formData.append('files', entry.file, entry.file.name);
    formData.append('paths', entry.relativePath);
  });

  const response = await fetchWithRetry(`${API_BASE}/api/import-folder`, {
    method: 'POST',
    body: formData,
  });

  if (!response.ok) {
    let message = `HTTP ${response.status}`;
    try {
      const payload = (await response.json()) as { error?: string };
      if (payload?.error) message = payload.error;
    } catch {
      const text = await response.text();
      if (text.trim()) message = text.trim();
    }
    throw new Error(message);
  }

  return (await response.json()) as ImportFolderResponse;
}

async function importFolderEntries(entries: Array<{ file: File; relativePath: string }>, folderName?: string): Promise<ImportFolderResponse> {
  if (entries.length === 0) {
    throw new Error('No files selected for import.');
  }

  if (entries.length <= IMPORT_CHUNK_SIZE) {
    return postImportFolderChunk(entries, folderName, false);
  }

  let lastResponse: ImportFolderResponse | null = null;
  for (let index = 0; index < entries.length; index += IMPORT_CHUNK_SIZE) {
    const chunk = entries.slice(index, index + IMPORT_CHUNK_SIZE);
    lastResponse = await postImportFolderChunk(chunk, folderName, index > 0);
  }

  if (!lastResponse) {
    throw new Error('Import finished without a server response.');
  }

  return lastResponse;
}

export const api = {
  getHealth(): Promise<HealthResponse> {
    return request<HealthResponse>('/api/health');
  },
  getMeta(): Promise<MetaResponse> {
    return request<MetaResponse>('/api/meta');
  },
  scan(payload: ScanRequest): Promise<ScanSnapshot> {
    return request<ScanSnapshot>('/api/scan', {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  },
  refresh(payload: ScanRequest): Promise<ScanSnapshot> {
    return request<ScanSnapshot>('/api/refresh', {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  },
  startScanJob(payload: ScanRequest): Promise<{ jobId: string }> {
    return request<{ jobId: string }>('/api/scan-job', {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  },
  startRefreshJob(payload: ScanRequest): Promise<{ jobId: string }> {
    return request<{ jobId: string }>('/api/refresh-job', {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  },
  getJob(jobId: string): Promise<ScanJob> {
    return request<ScanJob>(`/api/job?id=${encodeURIComponent(jobId)}`);
  },
  getAccountsPage(params: {
    resultId: string;
    page: number;
    pageSize: number;
    search?: string;
    status?: string;
    sort?: string;
    onlyFailure?: boolean;
  }): Promise<AccountsPageResponse> {
    const query = new URLSearchParams({
      resultId: params.resultId,
      page: String(params.page),
      pageSize: String(params.pageSize),
    });
    if (params.search?.trim()) query.set('search', params.search.trim());
    if (params.status?.trim() && params.status !== 'all') query.set('status', params.status.trim());
    if (params.sort?.trim()) query.set('sort', params.sort.trim());
    if (params.onlyFailure) query.set('onlyFailure', 'true');
    return request<AccountsPageResponse>(`/api/accounts?${query.toString()}`);
  },
  async importFolder(files: FileList): Promise<ImportFolderResponse> {
    const first = files[0] as File & { webkitRelativePath?: string };
    const rootName = first?.webkitRelativePath?.split('/')[0] || `import_${Date.now()}`;
    const entries = Array.from(files).map((file) => ({
      file,
      relativePath: (file as File & { webkitRelativePath?: string }).webkitRelativePath || file.name,
    }));
    return importFolderEntries(entries, rootName);
  },
  importFolderEntries,
  deleteDirectory(directory: string): Promise<DeleteDirectoryResponse> {
    return request<DeleteDirectoryResponse>('/api/delete-directory', {
      method: 'POST',
      body: JSON.stringify({ directory }),
    });
  },
  clearImportedFiles(): Promise<ClearImportedFilesResponse> {
    return request<ClearImportedFilesResponse>('/api/clear-imported-files', {
      method: 'POST',
      body: JSON.stringify({}),
    });
  },
  clearStats(): Promise<ClearStatsResponse> {
    return request<ClearStatsResponse>('/api/clear-stats', {
      method: 'POST',
      body: JSON.stringify({}),
    });
  },
  exportCsvUrl(payload: ScanRequest): string {
    const params = new URLSearchParams({
      directory: payload.directory,
      fullValueUSD: String(payload.fullValueUSD),
      autoConcurrency: String(payload.autoConcurrency),
      concurrency: String(payload.concurrency || 0),
      force: String(Boolean(payload.force)),
    });
    return `${API_BASE}/api/export.csv?${params.toString()}`;
  },
};

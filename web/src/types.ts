export type ThemeType = 'ocean' | 'dark' | 'monochrome' | 'rose' | 'lavender' | 'matcha';
export type AccountStatus = 'normal' | 'depleted' | 'expired' | 'disabled';

export interface DirectoryInfo {
  name: string;
  path: string;
  jsonCount: number;
  imported: boolean;
}

export interface ImportFolderResponse {
  imported: DirectoryInfo;
}

export interface DeleteDirectoryResponse {
  deleted: string;
}

export interface ClearImportedFilesResponse {
  removed: string[];
  removedCount: number;
}

export interface ClearStatsResponse {
  cleared: boolean;
  clearedCache: number;
  clearedJobs: number;
  remainingCache: number;
  remainingRunning: number;
}

export interface SystemInfo {
  logicalCPU: number;
  recommendedConcurrency: number;
  detectedMaxConcurrency: number;
}

export interface HealthResponse {
  ok: boolean;
  appName: string;
  time: string;
  uptimeSeconds: number;
  cacheEntries: number;
  directoryCount: number;
  staticReady: boolean;
  logicalCPU: number;
}

export interface MetaResponse {
  appName: string;
  workspaceRoot: string;
  directories: DirectoryInfo[];
  defaultDirectory: string;
  system: SystemInfo;
  defaultPrice: number;
  cacheTTLSeconds: number;
}

export interface CodexUsageWindow {
  label: string;
  usedPercent?: number;
  remainingPercent?: number;
  limitWindowSeconds?: number;
  resetAfterSeconds?: number;
  resetAt?: number;
  resetAtISO?: string;
}

export interface AccountRecord {
  id: string;
  file: string;
  email: string;
  plan: string;
  quotaPercent: number;
  usdValue: number;
  resetDate?: string;
  status: AccountStatus;
  disabled: boolean;
  lastRefresh?: string;
  expiredAt?: string;
  statusCode?: number;
  note?: string;
  windows?: CodexUsageWindow[];
}

export interface QuotaDistribution {
  healthy: number;
  medium: number;
  low: number;
  depleted: number;
}

export interface Summary {
  totalAccounts: number;
  successCount: number;
  failedCount: number;
  monthlyGrowthPercent: number;
  totalValueUSD: number;
  averageQuotaPercent: number;
  minQuotaPercent: number;
  maxQuotaPercent: number;
  quotaDistribution: QuotaDistribution;
}

export interface ScanSnapshot {
  resultId?: string;
  directory: string;
  directoryPath: string;
  scannedAt: string;
  durationMs: number;
  fullValueUSD: number;
  autoConcurrency: boolean;
  concurrencyUsed: number;
  recommendedConcurrency: number;
  logicalCPU: number;
  previewAccounts?: AccountRecord[];
  storedAccountCount?: number;
  accountsPartial?: boolean;
  summary: Summary;
  accounts: AccountRecord[];
}

export interface AccountsPageResponse {
  resultId: string;
  page: number;
  pageSize: number;
  total: number;
  totalPages: number;
  search?: string;
  status?: string;
  sort?: string;
  onlyFailure?: boolean;
  items: AccountRecord[];
}

export interface ScanRequest {
  directory: string;
  resultId?: string;
  fullValueUSD: number;
  autoConcurrency: boolean;
  concurrency: number;
  force?: boolean;
  accountIds?: string[];
}

export interface ScanHistoryEntry {
  id: string;
  directory: string;
  scannedAt: string;
  durationMs: number;
  totalAccounts: number;
  successCount: number;
  failedCount: number;
  totalValueUSD: number;
  concurrencyUsed: number;
}

export interface ScanJob {
  id: string;
  status: 'running' | 'completed' | 'failed';
  directory: string;
  done: number;
  total: number;
  percent: number;
  message?: string;
  startedAt: string;
  finishedAt?: string;
  snapshot?: ScanSnapshot;
}

export type AccountRow = AccountRecord;

export interface ScanMeta {
  directory: string;
  directoryPath: string;
  scannedAt: string;
  elapsedMs: number;
  fileCount: number;
  successCount: number;
  failureCount: number;
  concurrencyMode: 'auto' | 'manual';
  recommendedConcurrency: number;
  concurrencyUsed: number;
  logicalCpu: number;
  queriesPerSecond: number;
  fullValueUsd: number;
  partial: boolean;
  selectedCount?: number;
}

export interface SummaryMetrics {
  totalAccounts: number;
  totalValueUsd: number;
  averagePercent: number;
  averageValueUsd: number;
  minPercent: number;
  maxPercent: number;
  healthy: number;
  medium: number;
  low: number;
  depleted: number;
}

import type { AccountRow, SummaryMetrics } from './types';

export const EXCHANGE_RATE_AS_OF = '2026-03-06';

export const EXCHANGE_RATES = {
  CNY: 6.9047,
  JPY: 157.9189,
  EUR: 0.865,
  GBP: 0.7499,
  RUB: 78.19,
};

export function computeSummary(accounts: AccountRow[]): SummaryMetrics {
  const percents = accounts
    .map((item) => item.quotaPercent)
    .filter((value): value is number => typeof value === 'number');
  const values = accounts
    .map((item) => item.usdValue)
    .filter((value): value is number => typeof value === 'number');

  const metrics: SummaryMetrics = {
    totalAccounts: accounts.length,
    totalValueUsd: round2(values.reduce((sum, value) => sum + value, 0)),
    averagePercent: percents.length ? round2(percents.reduce((sum, value) => sum + value, 0) / percents.length) : 0,
    averageValueUsd: values.length ? round2(values.reduce((sum, value) => sum + value, 0) / values.length) : 0,
    minPercent: percents.length ? Math.min(...percents) : 0,
    maxPercent: percents.length ? Math.max(...percents) : 0,
    healthy: 0,
    medium: 0,
    low: 0,
    depleted: 0,
  };

  accounts.forEach((account) => {
    const percent = account.quotaPercent ?? 0;
    if (account.status === 'depleted' || percent <= 0) {
      metrics.depleted += 1;
      return;
    }
    if (percent >= 80) {
      metrics.healthy += 1;
      return;
    }
    if (percent >= 50) {
      metrics.medium += 1;
      return;
    }
    metrics.low += 1;
  });

  return metrics;
}

export function formatRelativeReset(resetDate?: string): string {
  if (!resetDate) {
    return '暂无';
  }
  const target = new Date(resetDate).getTime();
  if (Number.isNaN(target)) {
    return resetDate;
  }
  const diffMs = target - Date.now();
  const hours = Math.floor(diffMs / 3600000);
  if (hours > 24) {
    return `剩余 ${Math.floor(hours / 24)} 天`;
  }
  if (hours > 0) {
    return `剩余 ${hours} 小时`;
  }
  if (diffMs > 0) {
    const minutes = Math.max(1, Math.floor(diffMs / 60000));
    return `剩余 ${minutes} 分钟`;
  }
  return '已到期';
}

export function formatDateTime(value?: string): string {
  if (!value) {
    return '暂无';
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

export function downloadCSV(filename: string, rows: AccountRow[]): void {
  const header = ['文件', '邮箱', '套餐', '额度百分比', '美元价值', '重置时间', '状态', 'HTTP状态', '备注'];
  const lines = rows.map((row) => [
    row.file,
    row.email,
    row.plan,
    row.quotaPercent ?? '',
    row.usdValue ?? '',
    row.resetDate ?? '',
    row.status,
    row.statusCode ?? '',
    sanitizeCSV(row.note ?? ''),
  ]);
  const csv = [header, ...lines]
    .map((line) => line.map((cell) => `"${String(cell ?? '').replaceAll('"', '""')}"`).join(','))
    .join('\n');
  const blob = new Blob(['\uFEFF' + csv], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  document.body.removeChild(anchor);
  URL.revokeObjectURL(url);
}

function sanitizeCSV(value: string): string {
  return value.replace(/\r?\n/g, ' ');
}

function round2(value: number): number {
  return Math.round(value * 100) / 100;
}

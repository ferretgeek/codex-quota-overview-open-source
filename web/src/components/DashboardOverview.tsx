import type { ReactNode } from 'react';
import { Activity, Cpu, DollarSign, FolderOpen, RefreshCw, Trash2, Upload, Users } from 'lucide-react';
import { motion, type Variants } from 'framer-motion';
import type { HealthResponse, MetaResponse, ScanHistoryEntry, ScanJob, ScanSnapshot } from '../types';
import { EXCHANGE_RATE_AS_OF, EXCHANGE_RATES, formatDateTime } from '../utils';
import './Dashboard.css';

const containerVariants: Variants = {
  hidden: { opacity: 0 },
  show: { opacity: 1, transition: { staggerChildren: 0.08 } },
};

const itemVariants: Variants = {
  hidden: { opacity: 0, y: 20 },
  show: { opacity: 1, y: 0, transition: { type: 'spring', stiffness: 300, damping: 24 } },
};

interface DashboardOverviewProps {
  meta: MetaResponse | null;
  health: HealthResponse | null;
  snapshot: ScanSnapshot | null;
  history: ScanHistoryEntry[];
  loading: boolean;
  error: string;
  notice: string;
  progress: ScanJob | { done: number; total: number; percent: number; message: string } | null;
  pendingImportCount: number;
  pendingImportNames: string[];
  pendingImports: Array<{ id: string; folderName: string; fileCount: number }>;
  selectedDirectory: string;
  selectedDirectoryPath: string;
  fullValueUSD: number;
  onFullValueUSDChange: (value: number) => void;
  onRefresh: () => void;
  onSelectHistory: (entry: ScanHistoryEntry) => void;
  onPickFolder: () => void;
  onClearPendingImports: () => void;
  onClearImportedFiles: () => void;
  onClearAllStats: () => void;
  onRemovePendingImport: (id: string) => void;
  onDeleteDirectory: () => void;
  canDeleteDirectory: boolean;
  canClearImportedFiles: boolean;
}

export default function DashboardOverview({
  meta,
  health,
  snapshot,
  history,
  loading,
  error,
  notice,
  progress,
  pendingImportCount,
  pendingImportNames,
  pendingImports,
  selectedDirectory,
  selectedDirectoryPath,
  fullValueUSD,
  onFullValueUSDChange,
  onRefresh,
  onSelectHistory,
  onPickFolder,
  onClearPendingImports,
  onClearImportedFiles,
  onClearAllStats,
  onRemovePendingImport,
  onDeleteDirectory,
  canDeleteDirectory,
  canClearImportedFiles,
}: DashboardOverviewProps) {
  const summary = snapshot?.summary;
  const currentValueUSD = summary?.totalValueUSD ?? 0;
  const totalQuotaUSD = (summary?.totalAccounts ?? 0) * fullValueUSD;
  const lostValueUSD = Math.max(totalQuotaUSD - currentValueUSD, 0);
  const detectedConcurrency = snapshot?.recommendedConcurrency ?? meta?.system.recommendedConcurrency ?? 0;
  const actualConcurrency = snapshot?.concurrencyUsed ?? 0;
  const currentTaskLimit = snapshot?.summary.totalAccounts ?? meta?.directories.find((item) => item.name === selectedDirectory)?.jsonCount ?? 0;
  const metrics = [
    { title: '总账户数', value: String(summary?.totalAccounts ?? 0), sub: `${summary?.successCount ?? 0} 个可用`, icon: <Users size={22} /> },
    {
      title: '智能并发',
      value: `${detectedConcurrency} 线程`,
      sub: actualConcurrency > 0
        ? `本次实际开启 ${actualConcurrency} 个线程扫描${actualConcurrency < detectedConcurrency && currentTaskLimit > 0 ? `（当前仅 ${currentTaskLimit} 个文件）` : ''}`
	        : '自动规则：每检测到 1 个 CPU 线程配置 20 个扫描线程，检测失败默认 20 线程',
      icon: <Cpu size={22} />,
    },
    { title: '扫描速度', value: `${queriesPerSecond(snapshot)}/s`, sub: `耗时 ${((snapshot?.durationMs ?? 0) / 1000).toFixed(2)}s`, icon: <Activity size={22} /> },
  ];

  const currencies = [
    { label: '当前剩余额度', prefix: '$', value: currentValueUSD, hint: '美元 USD', valueColor: 'var(--text-primary)' },
    { label: '总额度', prefix: '$', value: totalQuotaUSD, hint: `${fullValueUSD.toFixed(2)} × ${(summary?.totalAccounts ?? 0).toLocaleString('zh-CN')}`, valueColor: 'var(--text-primary)' },
    { label: '奥特曼已亏损', prefix: '$', value: lostValueUSD, hint: '总额度 - 当前额度', valueColor: 'var(--status-error)' },
    { label: '人民币 CNY', prefix: '¥', value: currentValueUSD * EXCHANGE_RATES.CNY, hint: `当前剩余额度 · 汇率 ${EXCHANGE_RATES.CNY.toFixed(4)} · ${EXCHANGE_RATE_AS_OF}`, valueColor: 'var(--text-primary)' },
    { label: '日元 JPY', prefix: '¥', value: currentValueUSD * EXCHANGE_RATES.JPY, hint: `当前剩余额度 · 汇率 ${EXCHANGE_RATES.JPY.toFixed(4)} · ${EXCHANGE_RATE_AS_OF}`, valueColor: 'var(--text-primary)' },
    { label: '卢布 RUB', prefix: '₽', value: currentValueUSD * EXCHANGE_RATES.RUB, hint: `当前剩余额度 · 汇率 ${EXCHANGE_RATES.RUB.toFixed(4)} · ${EXCHANGE_RATE_AS_OF}`, valueColor: 'var(--text-primary)' },
  ];

  const distribution = [
    { label: '额度充足', count: summary?.quotaDistribution.healthy ?? 0, color: 'var(--status-success)' },
    { label: '额度适中', count: summary?.quotaDistribution.medium ?? 0, color: 'var(--status-warning)' },
    { label: '额度偏低', count: summary?.quotaDistribution.low ?? 0, color: 'var(--status-error)' },
    { label: '严重耗尽', count: summary?.quotaDistribution.depleted ?? 0, color: 'var(--status-disabled)' },
  ];

  const topLowAccounts = [...(snapshot?.previewAccounts ?? [])].sort((a, b) => a.quotaPercent - b.quotaPercent).slice(0, 6);
  const pendingImportFileCount = pendingImports.reduce((sum, item) => sum + item.fileCount, 0);

  return (
    <motion.div className="dashboard-container" variants={containerVariants} initial="hidden" animate="show">
      <motion.div variants={itemVariants} className="card" style={{ padding: '24px 32px', display: 'flex', flexDirection: 'column', gap: '24px', background: 'var(--bg-card)' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
          <div className="icon-wrapper bg-gradient" style={{ width: '48px', height: '48px' }}><DollarSign size={24} /></div>
          <span style={{ fontSize: '20px', fontWeight: 600, color: 'var(--text-secondary)' }}>资产总值看板（实时计算）</span>
        </div>
        <div className="currency-grid">
          {currencies.map((item) => {
            const valueText = `${item.prefix}${Math.round(item.value).toLocaleString('zh-CN')}`;
            const compactClass = valueText.length >= 9 ? ' currency-value--compact' : '';
            return (
              <div key={item.label} className="currency-card">
                <div className={`currency-value${compactClass}`} style={{ color: item.valueColor }}>{valueText}</div>
                <div className="currency-label" style={{ color: item.valueColor === 'var(--status-error)' ? 'var(--status-error)' : 'var(--text-secondary)' }}>{item.label}</div>
                <div className="currency-hint">{item.hint}</div>
              </div>
            );
          })}
        </div>
      </motion.div>

      <motion.div variants={itemVariants} className="card" style={{ padding: '20px 24px', display: 'grid', gap: '16px' }}>
        <div style={{ display: 'grid', gridTemplateColumns: '220px minmax(420px, 1.8fr) 160px 160px auto auto auto', gap: '12px', alignItems: 'end' }}>
          <div className="card" style={{ padding: '18px 20px', background: 'var(--bg-hover)' }}>
            <div className="text-secondary" style={{ fontWeight: 600 }}>总账户数</div>
            <div style={{ marginTop: '8px', fontSize: '30px', fontWeight: 800 }}>{summary?.totalAccounts ?? 0}</div>
          </div>
          <div style={{ display: 'grid', gap: '6px' }}>
            <span className="text-secondary" style={{ fontWeight: 600 }}>扫描目录</span>
            <button className="btn btn-secondary" style={{ minHeight: '56px', justifyContent: 'flex-start', paddingInline: '14px', width: '100%' }} onClick={onPickFolder}>
              <FolderOpen size={16} />
              <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', display: 'inline-block', maxWidth: '100%' }}>
                {selectedDirectoryPath ? selectedDirectory : '点击选择文件夹（支持多选一次性导入）'}
              </span>
            </button>
          </div>
          <label style={{ display: 'grid', gap: '6px' }}>
            <span className="text-secondary" style={{ fontWeight: 600 }}>100% 单价($)</span>
            <input id="dashboard-full-value" name="dashboardFullValue" className="settings-input" type="number" min="0.01" step="0.01" value={fullValueUSD} onChange={(event) => onFullValueUSDChange(Number(event.target.value || 0))} style={{ minHeight: '56px' }} />
          </label>
          <div style={{ display: 'grid', gap: '6px' }}>
            <span className="text-secondary" style={{ fontWeight: 600 }}>推荐并发</span>
            <div className="card" style={{ padding: '16px 14px', minHeight: '56px', display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'var(--bg-hover)' }}>{detectedConcurrency} 线程</div>
          </div>
          <button className="btn btn-secondary" style={{ minHeight: '56px' }} onClick={onPickFolder}><Upload size={16} /><span>手动选择文件夹</span></button>
          {canDeleteDirectory ? <button className="btn btn-secondary" style={{ minHeight: '56px' }} onClick={onDeleteDirectory}><Trash2 size={16} /><span>删除导入目录</span></button> : <div />}
          <button className="btn btn-primary" style={{ minHeight: '56px' }} onClick={onRefresh} disabled={loading || (!selectedDirectory && pendingImportCount === 0)}><RefreshCw size={16} className={loading ? 'spin' : ''} /><span>{loading ? '扫描中' : '立即扫描'}</span></button>
        </div>
        <div className="text-subtle" title={selectedDirectoryPath} style={{ maxWidth: '100%', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', fontSize: '14px' }}>
          当前路径：{shortenPath(selectedDirectoryPath) || '未选择'}
        </div>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '12px', flexWrap: 'wrap' }}>
          <div className="text-subtle">
            {pendingImportCount > 0
              ? `已暂存 ${pendingImportCount} 个文件夹，共 ${pendingImportFileCount} 个 JSON 文件：${pendingImportNames.slice(0, 3).join('、')}${pendingImportCount > 3 ? ' ...' : ''}`
              : '当前未暂存待导入文件夹'}
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', flexWrap: 'wrap' }}>
            <button className="btn btn-secondary" onClick={onClearPendingImports} disabled={pendingImportCount === 0 || loading}>清空输入文件夹</button>
            <button className="btn btn-secondary" onClick={onClearImportedFiles} disabled={!canClearImportedFiles || loading}>清空账号文件</button>
            <button className="btn btn-secondary" onClick={onClearAllStats} disabled={loading || (!snapshot && history.length === 0 && !error)}>清空所有统计数据</button>
          </div>
        </div>
        {pendingImports.length > 0 ? (
          <div style={{ display: 'grid', gap: '10px' }}>
            {pendingImports.map((item) => (
              <div key={item.id} className="card" style={{ padding: '12px 14px', display: 'grid', gridTemplateColumns: '1fr auto auto', gap: '12px', alignItems: 'center', background: 'var(--bg-hover)' }}>
                <div style={{ minWidth: 0 }}>
                  <div className="text-body" style={{ fontWeight: 600, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{item.folderName}</div>
                  <div className="text-subtle">{item.fileCount} 个 JSON 文件待导入</div>
                </div>
                <div className="text-micro text-subtle">待导入</div>
                <button className="btn btn-secondary" onClick={() => onRemovePendingImport(item.id)}>移除</button>
              </div>
            ))}
          </div>
        ) : null}
        {progress ? (
          <div style={{ display: 'grid', gap: '8px' }}>
            <div className="text-subtle">{progress.message || '扫描进行中'}（{progress.done} / {progress.total}）</div>
            <div className="progress-bar" style={{ height: '10px' }}>
              <motion.div className="progress-fill bg-success-fill" initial={{ width: 0 }} animate={{ width: `${progress.percent}%` }} transition={{ duration: 0.3 }} />
            </div>
          </div>
        ) : null}
        {meta && meta.directories.length === 0 ? <div style={{ color: 'var(--status-warning)', fontWeight: 600 }}>当前工作区下还没有可扫描的认证目录，或者你也可以点击“手动选择文件夹”直接导入。</div> : null}
        {health ? <div className="text-subtle" style={{ display: 'flex', gap: '18px', flexWrap: 'wrap' }}><span>服务：{health.ok ? '正常' : '异常'}</span><span>缓存：{health.cacheEntries}</span><span>可用目录：{health.directoryCount}</span><span>静态页：{health.staticReady ? '已就绪' : '未构建'}</span><span>运行：{health.uptimeSeconds}s</span></div> : null}
        {notice ? <div style={{ color: 'var(--status-success)', fontWeight: 600 }}>{notice}</div> : null}
        {error ? <div style={{ color: 'var(--status-error)' }}>{error}</div> : null}
      </motion.div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', gap: '24px' }}>
        {metrics.map((item) => <MetricCard key={item.title} icon={item.icon} title={item.title} value={item.value} sub={item.sub} />)}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1.35fr 1fr', gap: '24px' }}>
        <motion.div variants={itemVariants} className="card" style={{ padding: '24px 28px', display: 'grid', gap: '18px', background: 'var(--bg-card)' }}>
          <h3 className="text-h3">扫描摘要</h3>
          <SummaryRow label="工作区" value={meta?.workspaceRoot ?? '未知'} />
          <SummaryRow label="最近扫描" value={snapshot ? new Date(snapshot.scannedAt).toLocaleString('zh-CN') : '暂无'} />
          <SummaryRow label="成功 / 失败" value={`${summary?.successCount ?? 0} / ${summary?.failedCount ?? 0}`} />
          <SummaryRow label="平均剩余额度" value={`${summary?.averageQuotaPercent ?? 0}%`} />
          <SummaryRow label="近 30 日变动" value={`${summary?.monthlyGrowthPercent ?? 0}%`} />
          <SummaryRow label="额度区间" value={`${summary?.minQuotaPercent ?? 0}% ~ ${summary?.maxQuotaPercent ?? 0}%`} />
        </motion.div>

        <motion.div variants={itemVariants} className="card" style={{ padding: '24px 28px', display: 'grid', gap: '16px', background: 'var(--bg-card)' }}>
          <h3 className="text-h3">额度分布</h3>
          {distribution.map((item) => {
            const percent = summary?.totalAccounts ? Math.round((item.count / summary.totalAccounts) * 1000) / 10 : 0;
            return (
              <div key={item.label} style={{ display: 'grid', gap: '10px' }}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                    <span style={{ width: '12px', height: '12px', borderRadius: '999px', background: item.color, display: 'inline-block' }} />
                    <span className="text-secondary">{item.label}</span>
                  </div>
                  <strong>{item.count} / {percent}%</strong>
                </div>
                <div className="progress-bar">
                  <motion.div className="progress-fill" initial={{ width: 0 }} animate={{ width: `${percent}%`, backgroundColor: item.color }} transition={{ duration: 0.8 }} />
                </div>
              </div>
            );
          })}
        </motion.div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1.2fr 1fr', gap: '24px' }}>
        <motion.div variants={itemVariants} className="card" style={{ padding: '24px 28px', display: 'grid', gap: '14px' }}>
          <h3 className="text-h3">低额度预警</h3>
          {topLowAccounts.length === 0 ? <div className="text-subtle">暂无数据</div> : topLowAccounts.map((account) => (
            <div key={account.id} style={{ display: 'grid', gridTemplateColumns: '1.5fr auto auto', gap: '12px', paddingBottom: '12px', borderBottom: '1px solid var(--border-subtle)' }}>
              <div><div className="text-body" style={{ fontWeight: 600 }}>{account.email}</div><div className="text-micro text-subtle">{account.file}</div></div>
              <div className="text-body" style={{ fontWeight: 700 }}>{account.quotaPercent.toFixed(2)}%</div>
              <div className="text-subtle">${account.usdValue.toFixed(2)}</div>
            </div>
          ))}
        </motion.div>

        <motion.div variants={itemVariants} className="card" style={{ padding: '24px 28px', display: 'grid', gap: '14px' }}>
          <div className="flex items-center justify-between">
            <h3 className="text-h3">扫描历史</h3>
            <div className="text-micro text-subtle">最近 {history.length} 次</div>
          </div>
          {history.length === 0 ? <div className="text-subtle">暂无历史</div> : history.map((entry) => (
            <button key={entry.id} className="btn btn-secondary" style={{ justifyContent: 'space-between', textAlign: 'left' }} onClick={() => onSelectHistory(entry)}>
              <span>{entry.directory} · {entry.totalAccounts} 个</span>
              <span>{formatDateTime(entry.scannedAt)}</span>
            </button>
          ))}
        </motion.div>
      </div>
    </motion.div>
  );
}

function MetricCard({ icon, title, value, sub }: { icon: ReactNode; title: string; value: string; sub: string }) {
  return (
    <motion.div variants={itemVariants} className="card" style={{ padding: '24px 32px', display: 'flex', flexDirection: 'column', gap: '16px', justifyContent: 'center', background: 'var(--bg-card)' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
        <div className="icon-wrapper bg-gradient" style={{ width: '48px', height: '48px' }}>{icon}</div>
        <span style={{ fontSize: '20px', fontWeight: 600, color: 'var(--text-secondary)' }}>{title}</span>
      </div>
      <div style={{ fontSize: '30px', fontWeight: 800, lineHeight: 1, letterSpacing: '-0.03em', color: 'var(--text-primary)' }}>{value}</div>
      <div className="text-success" style={{ fontSize: '15px', fontWeight: 500 }}>{sub}</div>
    </motion.div>
  );
}

function SummaryRow({ label, value }: { label: string; value: string }) {
  return (
    <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', paddingBottom: '12px', borderBottom: '1px solid var(--border-subtle)' }}>
      <span className="text-secondary">{label}</span>
      <strong style={{ textAlign: 'right' }}>{value}</strong>
    </div>
  );
}

function queriesPerSecond(snapshot: ScanSnapshot | null): number {
  if (!snapshot || snapshot.durationMs <= 0 || snapshot.summary.totalAccounts <= 0) {
    return 0;
  }
  return Math.round((snapshot.summary.totalAccounts / (snapshot.durationMs / 1000)) * 100) / 100;
}

function shortenPath(path: string): string {
  if (!path) return '';
  if (path.length <= 56) return path;
  return `${path.slice(0, 24)} ... ${path.slice(-24)}`;
}

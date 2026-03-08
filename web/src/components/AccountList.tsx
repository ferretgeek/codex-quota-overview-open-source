import { useEffect, useMemo, useState } from 'react';
import { CheckSquare, Filter, MoreVertical, RefreshCw, Search, Square as SquareIcon } from 'lucide-react';
import { AnimatePresence, motion, type Variants } from 'framer-motion';
import './AccountList.css';
import type { AccountStatus } from '../types';
import { useAccountsPage } from '../hooks/useAccountsPage';

const listVariants: Variants = {
  hidden: { opacity: 0 },
  show: { opacity: 1, transition: { staggerChildren: 0.04 } },
};

const rowVariants: Variants = {
  hidden: { opacity: 0, x: -10 },
  show: { opacity: 1, x: 0, transition: { type: 'spring', stiffness: 300, damping: 24 } },
};

interface AccountListProps {
  resultId?: string;
  loading: boolean;
  autoRefreshEnabled: boolean;
  autoRefreshMinutes: number;
  pageSize: number;
  onPageSizeChange: (pageSize: number) => void;
  onAutoRefreshChange: (enabled: boolean) => void;
  onAutoRefreshMinutesChange: (minutes: number) => void;
  onRefreshAll: () => Promise<void> | void;
  onRefreshSelected: (ids: string[]) => Promise<void> | void;
}

const STATUS_OPTIONS: Array<{ value: 'all' | AccountStatus; label: string }> = [
  { value: 'all', label: '全部状态' },
  { value: 'normal', label: '正常运行' },
  { value: 'depleted', label: '额度已尽' },
  { value: 'expired', label: '自动失效' },
  { value: 'disabled', label: '已被封禁' },
];

const SORT_OPTIONS = [
  { value: 'quotaAsc', label: '额度从低到高' },
  { value: 'quotaDesc', label: '额度从高到低' },
  { value: 'valueDesc', label: '价值从高到低' },
  { value: 'emailAsc', label: '邮箱 A-Z' },
  { value: 'statusDesc', label: '失败优先' },
] as const;

type SortKey = (typeof SORT_OPTIONS)[number]['value'];

export default function AccountList({
  resultId,
  loading,
  autoRefreshEnabled: _autoRefreshEnabled,
  autoRefreshMinutes: _autoRefreshMinutes,
  pageSize,
  onPageSizeChange,
  onAutoRefreshChange: _onAutoRefreshChange,
  onAutoRefreshMinutesChange: _onAutoRefreshMinutesChange,
  onRefreshAll,
  onRefreshSelected,
}: AccountListProps) {
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [search, setSearch] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState<'all' | AccountStatus>('all');
  const [sortKey, setSortKey] = useState<SortKey>('quotaAsc');
  const [page, setPage] = useState(1);

  useEffect(() => {
    const timer = window.setTimeout(() => setDebouncedSearch(search.trim()), 250);
    return () => window.clearTimeout(timer);
  }, [search]);

  useEffect(() => {
    setPage(1);
  }, [debouncedSearch, pageSize, resultId, sortKey, statusFilter]);

  useEffect(() => {
    setSelectedIds(new Set());
  }, [page, debouncedSearch, pageSize, resultId, sortKey, statusFilter]);

  const { data, loading: pageLoading, error } = useAccountsPage({
    resultId,
    page,
    pageSize,
    search: debouncedSearch,
    status: statusFilter,
    sort: sortKey,
  });

  const items = data?.items ?? [];
  const total = data?.total ?? 0;
  const totalPages = Math.max(1, data?.totalPages ?? 1);
  const currentPage = data?.page ?? page;
  const visibleRangeStart = total === 0 ? 0 : (currentPage - 1) * (data?.pageSize ?? pageSize) + 1;
  const visibleRangeEnd = total === 0 ? 0 : Math.min(currentPage * (data?.pageSize ?? pageSize), total);
  const allVisibleSelected = items.length > 0 && items.every((item) => selectedIds.has(item.id));

  const toolbarNote = useMemo(() => {
    if (!resultId) return '请先在首页手动点击“立即扫描”，再查看账户列表。';
    if (loading) return '正在执行扫描任务，列表会在任务完成后刷新。';
    if (pageLoading) return '正在按页加载账户数据...';
    return `当前展示第 ${currentPage} / ${totalPages} 页，共 ${total.toLocaleString('zh-CN')} 个账户。`;
  }, [currentPage, loading, pageLoading, resultId, total, totalPages]);

  const toggleSelectAll = () => {
    if (allVisibleSelected) {
      setSelectedIds(new Set());
      return;
    }
    setSelectedIds(new Set(items.map((account) => account.id)));
  };

  const toggleSelect = (id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const handleRefreshSelected = async () => {
    if (selectedIds.size === 0 || loading) return;
    await onRefreshSelected([...selectedIds]);
    setSelectedIds(new Set());
  };

  return (
    <motion.div className="card account-list-container" initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.3 }}>
      <div className="list-toolbar" style={{ flexWrap: 'wrap', gap: '12px' }}>
        <div className="toolbar-left" style={{ flexWrap: 'wrap' }}>
          <div className="search-box" style={{ minWidth: '320px', flex: '1 1 320px' }}>
            <Search size={16} />
            <input id="account-search" name="accountSearch" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="搜索邮箱、文件名、ID 或备注" />
          </div>
          <div className="refresh-controls" style={{ flexWrap: 'wrap' }}>
            <Filter size={16} />
            <select id="account-status-filter" name="accountStatusFilter" className="btn btn-secondary" value={statusFilter} onChange={(event) => setStatusFilter(event.target.value as 'all' | AccountStatus)}>
              {STATUS_OPTIONS.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}
            </select>
            <select id="account-sort-key" name="accountSortKey" className="btn btn-secondary" value={sortKey} onChange={(event) => setSortKey(event.target.value as SortKey)}>
              {SORT_OPTIONS.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}
            </select>
          </div>
        </div>

        <div className="toolbar-right" style={{ flexWrap: 'wrap' }}>
          <div className="text-micro text-subtle">仅支持手动刷新 · 大结果由服务端分页加载</div>
          <button className="btn btn-secondary" disabled={!resultId || loading} onClick={() => void onRefreshAll()}>
            <RefreshCw size={16} className={loading ? 'animate-spin' : ''} />
            <span>刷新全部</span>
          </button>
          <button className="btn btn-primary" disabled={selectedIds.size === 0 || loading} onClick={() => void handleRefreshSelected()}>
            <RefreshCw size={16} className={loading ? 'animate-spin' : ''} />
            <span>刷新选中（{selectedIds.size}）</span>
          </button>
        </div>
      </div>

      <div className="text-subtle" style={{ padding: '0 24px 16px' }}>{error || toolbarNote}</div>

      <div className="table-wrapper">
        <table className="account-table">
          <thead>
            <tr>
              <th style={{ width: '56px', paddingLeft: '24px' }}>
                <div className="checkbox-wrapper" onClick={toggleSelectAll}>
                  {allVisibleSelected ? <CheckSquare size={18} className="icon-active" /> : <SquareIcon size={18} />}
                </div>
              </th>
              <th>账户</th>
              <th>套餐</th>
              <th>额度</th>
              <th>价值</th>
              <th>重置时间</th>
              <th>状态</th>
              <th>备注</th>
              <th style={{ width: '64px', paddingRight: '24px' }}>操作</th>
            </tr>
          </thead>
          <motion.tbody variants={listVariants} initial="hidden" animate="show">
            <AnimatePresence mode="popLayout">
              {items.length === 0 ? (
                <motion.tr key="empty" variants={rowVariants} className="table-row">
                  <td colSpan={9} style={{ padding: '28px 24px', textAlign: 'center', color: 'var(--text-secondary)' }}>
                    {!resultId ? '暂无扫描结果。' : pageLoading ? '正在加载这一页的数据...' : error ? '账户分页加载失败。' : '这一页没有匹配结果。'}
                  </td>
                </motion.tr>
              ) : items.map((account) => (
                <motion.tr key={account.id} variants={rowVariants} layout className={`table-row ${selectedIds.has(account.id) ? 'selected' : ''}`}>
                  <td style={{ paddingLeft: '24px' }}>
                    <div className="checkbox-wrapper" onClick={() => toggleSelect(account.id)}>
                      {selectedIds.has(account.id) ? <CheckSquare size={18} className="icon-active" /> : <SquareIcon size={18} />}
                    </div>
                  </td>
                  <td>
                    <div className="account-email text-body">{account.email || account.file}</div>
                    <div className="text-micro text-subtle" style={{ marginTop: '4px' }}>{account.file}</div>
                  </td>
                  <td><span className="plan-tag">{formatPlan(account.plan)}</span></td>
                  <td>
                    <div className="quota-wrapper">
                      <div className="quota-text">
                        <span className="text-body" style={{ fontWeight: 600 }}>{account.quotaPercent.toFixed(2)}%</span>
                      </div>
                      <div className="progress-bar">
                        <motion.div
                          className={`progress-fill ${account.quotaPercent < 20 ? 'bg-error-fill' : account.quotaPercent < 60 ? 'bg-warning-fill' : 'bg-success-fill'}`}
                          initial={{ width: 0 }}
                          animate={{ width: `${Math.max(0, Math.min(100, account.quotaPercent))}%` }}
                          transition={{ duration: 0.6 }}
                        />
                      </div>
                    </div>
                  </td>
                  <td><div className="text-body" style={{ fontWeight: 600 }}>${account.usdValue.toFixed(2)}</div></td>
                  <td><div className="text-body text-secondary">{formatResetTime(account.resetDate)}</div></td>
                  <td>{getStatusBadge(account.status)}</td>
                  <td>
                    <div className="text-subtle" style={{ maxWidth: '220px', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }} title={account.note ?? ''}>
                      {account.note || (account.statusCode ? `HTTP ${account.statusCode}` : '—')}
                    </div>
                  </td>
                  <td style={{ paddingRight: '24px' }}>
                    <button className="btn btn-ghost btn-icon-only" disabled>
                      <MoreVertical size={16} />
                    </button>
                  </td>
                </motion.tr>
              ))}
            </AnimatePresence>
          </motion.tbody>
        </table>
      </div>

      <div className="list-footer" style={{ gap: '12px', flexWrap: 'wrap' }}>
        <div className="text-subtle">
          当前显示 <b style={{ color: 'var(--text-primary)' }}>{visibleRangeStart}-{visibleRangeEnd}</b> 行，共计 {total.toLocaleString('zh-CN')} 个配额实体
        </div>
        <div className="pagination" style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
          <select id="account-page-size" name="accountPageSize" className="btn btn-secondary" value={pageSize} onChange={(event) => onPageSizeChange(Number(event.target.value) || 20)}>
            {[20, 50, 100, 200].map((size) => <option key={size} value={size}>{size}/页</option>)}
          </select>
          <button className="btn btn-secondary" style={{ padding: '6px 12px' }} disabled={currentPage <= 1 || pageLoading} onClick={() => setPage((prev) => Math.max(1, prev - 1))}>上一页</button>
          <div className="text-micro text-subtle">第 {currentPage} / {totalPages} 页</div>
          <button className="btn btn-secondary" style={{ padding: '6px 12px' }} disabled={currentPage >= totalPages || pageLoading} onClick={() => setPage((prev) => Math.min(totalPages, prev + 1))}>下一页</button>
        </div>
      </div>
    </motion.div>
  );
}

function formatPlan(plan: string): string {
  if (!plan) return 'UNKNOWN';
  return plan.toUpperCase();
}

function formatResetTime(value?: string): string {
  if (!value) return '暂无';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  });
}

function getStatusBadge(status: AccountStatus) {
  switch (status) {
    case 'normal':
      return <span className="badge badge-success">正常运行</span>;
    case 'depleted':
      return <span className="badge badge-warning">额度已尽</span>;
    case 'expired':
      return <span className="badge badge-error">自动失效</span>;
    case 'disabled':
      return <span className="badge badge-neutral">已被封禁</span>;
    default:
      return <span className="badge badge-neutral">未知</span>;
  }
}

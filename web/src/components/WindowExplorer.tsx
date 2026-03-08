import { useEffect, useState } from 'react';
import type { AccountRecord } from '../types';
import { useAccountsPage } from '../hooks/useAccountsPage';

interface WindowExplorerProps {
  resultId?: string;
  loading: boolean;
  pageSize: number;
  onPageSizeChange: (pageSize: number) => void;
}

export default function WindowExplorer({ resultId, loading, pageSize, onPageSizeChange }: WindowExplorerProps) {
  const [showOnlyFailures, setShowOnlyFailures] = useState(false);
  const [page, setPage] = useState(1);

  useEffect(() => {
    setPage(1);
  }, [pageSize, resultId, showOnlyFailures]);

  const { data, loading: pageLoading, error } = useAccountsPage({
    resultId,
    page,
    pageSize,
    sort: showOnlyFailures ? 'statusDesc' : 'fileAsc',
    onlyFailure: showOnlyFailures,
  });

  const items = data?.items ?? [];
  const total = data?.total ?? 0;
  const totalPages = Math.max(1, data?.totalPages ?? 1);
  const currentPage = data?.page ?? page;
  const visibleRangeStart = total === 0 ? 0 : (currentPage - 1) * (data?.pageSize ?? pageSize) + 1;
  const visibleRangeEnd = total === 0 ? 0 : Math.min(currentPage * (data?.pageSize ?? pageSize), total);

  return (
    <div className="card" style={{ padding: '24px 28px', display: 'grid', gap: '20px' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', gap: '24px', flexWrap: 'wrap' }}>
        <div>
          <h3 className="text-h3">扫描详情</h3>
          <div className="text-subtle" style={{ marginTop: '6px' }}>
            {error || (!resultId ? '请先在首页手动点击“立即扫描”，再查看窗口详情。' : pageLoading ? '正在按页加载窗口详情...' : '查看每个账号的窗口信息、HTTP 状态和备注。')}
          </div>
        </div>
        <div style={{ display: 'flex', gap: '12px', alignItems: 'center', flexWrap: 'wrap' }}>
          <button className={`btn ${showOnlyFailures ? 'btn-primary' : 'btn-secondary'}`} onClick={() => setShowOnlyFailures((prev) => !prev)}>
            {showOnlyFailures ? '显示全部' : '仅看异常'}
          </button>
          <select id="window-page-size" name="windowPageSize" className="btn btn-secondary" value={pageSize} onChange={(event) => onPageSizeChange(Number(event.target.value) || 20)}>
            {[20, 50, 100, 200].map((size) => <option key={size} value={size}>{size}/页</option>)}
          </select>
          <div className="text-micro text-subtle">{loading ? '扫描中...' : `共 ${total.toLocaleString('zh-CN')} 个账号`}</div>
        </div>
      </div>

      <div style={{ overflow: 'auto' }}>
        <table className="account-table">
          <thead>
            <tr>
              <th style={{ paddingLeft: '24px' }}>文件</th>
              <th>状态</th>
              <th>HTTP</th>
              <th>窗口数</th>
              <th>最早重置</th>
              <th style={{ paddingRight: '24px' }}>备注</th>
            </tr>
          </thead>
          <tbody>
            {items.length === 0 ? (
              <tr className="table-row">
                <td colSpan={6} style={{ padding: '28px 24px', textAlign: 'center', color: 'var(--text-secondary)' }}>
                  {!resultId ? '暂无扫描结果。' : pageLoading ? '正在加载这一页的数据...' : error ? '窗口详情加载失败。' : '当前没有匹配结果。'}
                </td>
              </tr>
            ) : items.map((account) => (
              <tr key={account.id} className="table-row">
                <td style={{ paddingLeft: '24px' }}>
                  <div className="text-body" style={{ fontWeight: 600 }}>{account.file}</div>
                  <div className="text-micro text-subtle">{account.email}</div>
                </td>
                <td>{renderStatus(account)}</td>
                <td>{account.statusCode ?? '-'}</td>
                <td>{account.windows?.length ?? 0}</td>
                <td>{formatDateTime(account.resetDate)}</td>
                <td style={{ paddingRight: '24px', maxWidth: '420px' }}>
                  <span className="text-subtle">{account.note ?? '—'}</span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="list-footer" style={{ padding: 0, gap: '12px', flexWrap: 'wrap' }}>
        <div className="text-subtle">
          当前显示 <b style={{ color: 'var(--text-primary)' }}>{visibleRangeStart}-{visibleRangeEnd}</b> 行，共计 {total.toLocaleString('zh-CN')} 个详情实体
        </div>
        <div className="pagination" style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
          <button className="btn btn-secondary" style={{ padding: '6px 12px' }} disabled={currentPage <= 1 || pageLoading} onClick={() => setPage((prev) => Math.max(1, prev - 1))}>上一页</button>
          <div className="text-micro text-subtle">第 {currentPage} / {totalPages} 页</div>
          <button className="btn btn-secondary" style={{ padding: '6px 12px' }} disabled={currentPage >= totalPages || pageLoading} onClick={() => setPage((prev) => Math.min(totalPages, prev + 1))}>下一页</button>
        </div>
      </div>
    </div>
  );
}

function renderStatus(account: AccountRecord) {
  if (account.note || (account.statusCode ?? 200) >= 400 || (account.windows?.length ?? 0) === 0) {
    return <span className="badge badge-warning">异常</span>;
  }
  return <span className="badge badge-success">正常</span>;
}

function formatDateTime(value?: string): string {
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

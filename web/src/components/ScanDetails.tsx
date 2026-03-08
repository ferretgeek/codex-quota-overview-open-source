import { motion } from 'framer-motion';
import type { AccountRow, ScanMeta } from '../types';
import { formatDateTime } from '../utils';

interface ScanDetailsProps {
  accounts: AccountRow[];
  scanMeta?: ScanMeta | null;
}

export default function ScanDetails({ accounts, scanMeta }: ScanDetailsProps) {
  return (
    <motion.div className="card" initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }} style={{ padding: '24px 28px', display: 'grid', gap: '20px' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', gap: '24px', flexWrap: 'wrap' }}>
        <div>
          <h3 className="text-h3">扫描详情</h3>
          <div className="text-subtle" style={{ marginTop: '6px' }}>展示每个账号的原始窗口、HTTP 状态和错误说明。</div>
        </div>
        <div className="text-micro text-subtle" style={{ textAlign: 'right' }}>
          <div>目录：{scanMeta?.directory ?? '未选择'}</div>
          <div>扫描时间：{formatDateTime(scanMeta?.scannedAt)}</div>
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
            {accounts.map((account) => (
              <tr key={account.id} className="table-row">
                <td style={{ paddingLeft: '24px' }}>
                  <div className="text-body" style={{ fontWeight: 600 }}>{account.file}</div>
                  <div className="text-micro text-subtle">{account.email}</div>
                </td>
                <td>{account.status}</td>
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
    </motion.div>
  );
}


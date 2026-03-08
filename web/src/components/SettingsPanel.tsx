import type { ReactNode } from 'react';
import { FolderOpen } from 'lucide-react';
import type { HealthResponse, MetaResponse, ThemeType } from '../types';
import { motion } from 'framer-motion';

interface SettingsShape {
  theme: ThemeType;
  directory: string;
  fullValueUSD: number;
  autoConcurrency: boolean;
  manualConcurrency: number;
  autoRefreshEnabled: boolean;
  autoRefreshMinutes: number;
  pageSize: number;
}

interface SettingsPanelProps {
  meta: MetaResponse | null;
  health: HealthResponse | null;
  settings: SettingsShape;
  loading: boolean;
  onChange: (partial: Partial<SettingsShape>) => void;
  onApply: () => void;
  onPickFolder: () => void;
  onDeleteDirectory: () => void;
  canDeleteDirectory: boolean;
  selectedDirectoryPath: string;
}

export default function SettingsPanel({ meta, health, settings, loading, onChange, onApply, onPickFolder, onDeleteDirectory, canDeleteDirectory, selectedDirectoryPath }: SettingsPanelProps) {
  return (
    <motion.div className="card" initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }} style={{ padding: '24px 28px', display: 'grid', gap: '20px' }}>
      <div>
        <h3 className="text-h3">系统设置</h3>
	        <div className="text-subtle" style={{ marginTop: '6px' }}>自动模式按本机检测到的 CPU 线程数量计算并发，规则为每检测到 1 个 CPU 线程配置 20 个扫描线程；如果未检测到，则默认 20 线程，同时仍受目录文件数限制。</div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1.4fr auto auto', gap: '12px', alignItems: 'end' }}>
        <div style={{ display: 'grid', gap: '8px' }}>
          <span className="text-secondary" style={{ fontWeight: 600 }}>扫描目录</span>
          <button className="btn btn-secondary" style={{ minHeight: '52px', justifyContent: 'flex-start', paddingInline: '14px', width: '100%' }} onClick={onPickFolder}>
            <FolderOpen size={16} />
            <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', display: 'inline-block', maxWidth: '100%' }}>
              {selectedDirectoryPath ? settings.directory : '点击选择文件夹'}
            </span>
          </button>
        </div>
        <button className="btn btn-secondary" style={{ minHeight: '52px' }} onClick={onPickFolder}>手动选择文件夹</button>
        {canDeleteDirectory ? <button className="btn btn-secondary" style={{ minHeight: '52px' }} onClick={onDeleteDirectory}>删除导入目录</button> : <div />}
      </div>

      <div className="text-subtle" title={selectedDirectoryPath} style={{ maxWidth: '100%', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
        当前路径：{shortenPath(selectedDirectoryPath) || '未选择'}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, minmax(0, 1fr))', gap: '18px' }}>
        <Field label="100% 单价（美元）">
          <input id="settings-full-value" name="settingsFullValue" className="settings-input" type="number" min="0.01" step="0.01" value={settings.fullValueUSD} onChange={(event) => onChange({ fullValueUSD: Number(event.target.value || 0) })} />
        </Field>
        <Field label="并发模式">
          <select id="settings-concurrency-mode" name="settingsConcurrencyMode" className="settings-input" value={settings.autoConcurrency ? 'auto' : 'manual'} onChange={(event) => onChange({ autoConcurrency: event.target.value === 'auto' })}>
            <option value="auto">自动（推荐）</option>
            <option value="manual">手动</option>
          </select>
        </Field>
        <Field label="手动并发">
          <input id="settings-manual-concurrency" name="settingsManualConcurrency" className="settings-input" type="number" min="1" step="1" disabled={settings.autoConcurrency} value={settings.manualConcurrency} onChange={(event) => onChange({ manualConcurrency: Number(event.target.value || 1) })} />
        </Field>
        <Field label="账户列表分页大小">
          <select id="settings-page-size" name="settingsPageSize" className="settings-input" value={settings.pageSize} onChange={(event) => onChange({ pageSize: Number(event.target.value) || 20 })}>
            <option value="20">20/页</option>
            <option value="50">50/页</option>
            <option value="100">100/页</option>
            <option value="200">200/页</option>
          </select>
        </Field>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, minmax(0, 1fr))', gap: '12px' }}>
        <InfoBox label="逻辑线程" value={`${meta?.system.logicalCPU ?? 0} 线程`} />
        <InfoBox label="推荐并发" value={`${meta?.system.recommendedConcurrency ?? 0} 线程`} />
        <InfoBox label="检测上限" value={`${meta?.system.detectedMaxConcurrency ?? 0} 线程`} />
        <InfoBox label="工作区" value={meta?.workspaceRoot ?? '未知'} />
        <InfoBox label="服务状态" value={health?.ok ? '正常' : '异常'} />
        <InfoBox label="缓存项" value={String(health?.cacheEntries ?? 0)} />
        <InfoBox label="目录数量" value={String(health?.directoryCount ?? 0)} />
        <InfoBox label="静态页面" value={health?.staticReady ? '已就绪' : '未就绪'} />
      </div>

      <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '12px' }}>
        <button className="btn btn-primary" onClick={onApply} disabled={loading}>{loading ? '扫描中...' : '应用设置并扫描'}</button>
      </div>
    </motion.div>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label style={{ display: 'grid', gap: '8px' }}>
      <span className="text-secondary" style={{ fontWeight: 600 }}>{label}</span>
      {children}
    </label>
  );
}

function InfoBox({ label, value }: { label: string; value: string }) {
  return (
    <div className="card" style={{ padding: '16px 18px', background: 'var(--bg-hover)', border: '1px solid var(--border-subtle)' }}>
      <div className="text-micro text-subtle">{label}</div>
      <div style={{ marginTop: '8px', fontSize: '18px', fontWeight: 700, overflowWrap: 'anywhere' }}>{value}</div>
    </div>
  );
}

function shortenPath(path: string): string {
  if (!path) return '';
  if (path.length <= 56) return path;
  return `${path.slice(0, 24)} ... ${path.slice(-24)}`;
}

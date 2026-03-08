import { Download, Palette, RefreshCw, ShieldCheck } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { AnimatePresence, motion } from 'framer-motion';
import type { HealthResponse, ThemeType } from '../types';

interface TopbarProps {
  onThemeChange: (theme: ThemeType) => void;
  currentTheme: ThemeType;
  onExport: () => void;
  onRefreshAll: () => void;
  isRefreshing: boolean;
  subtitle: string;
  health: HealthResponse | null;
}

const THEMES: { id: ThemeType; name: string; color: string }[] = [
  { id: 'ocean', name: '蔚蓝之境', color: '#E8F0F8' },
  { id: 'dark', name: '深邃暗渊', color: '#0B0F19' },
  { id: 'monochrome', name: '极致黑白', color: '#ECECEC' },
  { id: 'rose', name: '玫瑰冰茶', color: '#FFF1F2' },
  { id: 'lavender', name: '微醺浅紫', color: '#F5F3FF' },
  { id: 'matcha', name: '初春抹茶', color: '#ECFDF5' },
];

export default function Topbar({
  onThemeChange,
  currentTheme,
  onExport,
  onRefreshAll,
  isRefreshing,
  subtitle,
  health,
}: TopbarProps) {
  const [showThemeMenu, setShowThemeMenu] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);
  const today = new Date().toLocaleDateString('zh-CN', {
    weekday: 'long',
    month: 'long',
    day: 'numeric',
  });

  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        setShowThemeMenu(false);
      }
    }
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  const healthLabel = health?.ok ? '服务正常' : '服务异常';
  const healthColor = health?.ok ? 'var(--status-success)' : 'var(--status-error)';

  return (
    <header className="topbar">
      <div>
        <h2 className="text-h2">控制面板</h2>
        <div className="text-subtle" style={{ marginTop: '2px', display: 'flex', alignItems: 'center', gap: '10px', flexWrap: 'wrap' }}>
          <span>{today} · {subtitle}</span>
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: '6px', color: healthColor, fontWeight: 600 }}>
            <ShieldCheck size={14} />
            {healthLabel}
          </span>
        </div>
      </div>

      <div className="flex items-center gap-4">
        <button className="btn btn-secondary" onClick={onRefreshAll} disabled={isRefreshing}>
          <RefreshCw size={16} className={isRefreshing ? 'animate-spin' : ''} />
          <span>{isRefreshing ? '扫描中...' : '刷新全部'}</span>
        </button>

        <button className="btn btn-secondary" onClick={onExport}>
          <Download size={16} />
          <span>导出报表</span>
        </button>

        <div className="relative" ref={menuRef}>
          <button
            className="btn btn-ghost"
            onClick={() => setShowThemeMenu((prev) => !prev)}
            style={{
              padding: '10px',
              borderRadius: 'var(--radius-full)',
              backgroundColor: showThemeMenu ? 'var(--bg-hover)' : 'transparent',
              color: showThemeMenu ? 'var(--text-primary)' : 'var(--text-secondary)',
            }}
            aria-label="选择主题"
          >
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <Palette size={20} />
              <span style={{ fontWeight: 600, fontSize: '14px' }}>切换主题</span>
            </div>
          </button>

          <AnimatePresence>
            {showThemeMenu ? (
              <motion.div
                className="absolute card"
                initial={{ opacity: 0, y: 10, scale: 0.95 }}
                animate={{ opacity: 1, y: 0, scale: 1 }}
                exit={{ opacity: 0, y: 10, scale: 0.95 }}
                transition={{ duration: 0.2 }}
                style={{ top: '100%', right: 0, marginTop: '8px', padding: '8px', zIndex: 100, display: 'flex', gap: '4px', flexDirection: 'column', minWidth: '180px', boxShadow: 'var(--shadow-lg)' }}
              >
                <div className="text-micro" style={{ padding: '4px 8px', marginBottom: '8px', opacity: 0.7 }}>选择主题</div>
                {THEMES.map((theme) => (
                  <div
                    key={theme.id}
                    className={`theme-menu-item ${currentTheme === theme.id ? 'active' : ''}`}
                    onClick={() => {
                      onThemeChange(theme.id);
                      setShowThemeMenu(false);
                    }}
                    style={{ display: 'flex', alignItems: 'center', gap: '12px', padding: '10px 14px', fontSize: '13px', cursor: 'pointer', borderRadius: 'var(--radius-sm)', color: currentTheme === theme.id ? 'var(--accent-base)' : 'var(--text-primary)', backgroundColor: currentTheme === theme.id ? 'var(--bg-hover)' : 'transparent', fontWeight: currentTheme === theme.id ? 600 : 500, transition: 'all 0.2s' }}
                  >
                    <div style={{ width: '16px', height: '16px', borderRadius: '50%', backgroundColor: theme.color, border: '1px solid var(--border-strong)', boxShadow: 'inset 0 1px 2px rgba(0,0,0,0.1)' }} />
                    <span>{theme.name}</span>
                  </div>
                ))}
              </motion.div>
            ) : null}
          </AnimatePresence>
        </div>
      </div>
    </header>
  );
}

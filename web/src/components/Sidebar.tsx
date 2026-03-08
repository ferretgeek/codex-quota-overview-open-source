import { Activity, Gauge, LayoutDashboard, Settings, Users } from 'lucide-react';
import { motion } from 'framer-motion';
import type { ViewKey } from './Layout';

const NAV_ITEMS: { id: ViewKey; label: string; icon: typeof LayoutDashboard }[] = [
  { id: 'dashboard', label: '控制台概览', icon: LayoutDashboard },
  { id: 'accounts', label: '账户管理', icon: Users },
  { id: 'windows', label: '窗口详情', icon: Activity },
  { id: 'settings', label: '系统设置', icon: Settings },
];

interface SidebarProps {
  activeView: ViewKey;
  onChangeView: (view: ViewKey) => void;
}

export default function Sidebar({ activeView, onChangeView }: SidebarProps) {
  return (
    <motion.aside
      className="sidebar"
      initial={{ x: -20, opacity: 0 }}
      animate={{ x: 0, opacity: 1 }}
      transition={{ duration: 0.6, ease: [0.16, 1, 0.3, 1] }}
    >
      <div className="sidebar-header">
        <div className="sidebar-logo">
          <Gauge size={16} />
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', marginLeft: '4px' }}>
          <span style={{ fontWeight: 700, fontSize: '18px', letterSpacing: '-0.02em', lineHeight: '1.2' }}>Codex普号</span>
          <span style={{ fontWeight: 600, fontSize: '13px', letterSpacing: '0.05em', color: 'var(--text-secondary)' }}>额度概览</span>
        </div>
      </div>

      <nav className="sidebar-nav">
        {NAV_ITEMS.map((item) => {
          const isActive = activeView === item.id;
          const Icon = item.icon;
          return (
            <div key={item.id} className={`nav-item ${isActive ? 'active' : ''}`} onClick={() => onChangeView(item.id)}>
              {isActive ? (
                <motion.div
                  layoutId="active-nav"
                  className="nav-active-bg"
                  transition={{ type: 'spring', stiffness: 300, damping: 30 }}
                />
              ) : null}
              <Icon size={18} />
              <span>{item.label}</span>
            </div>
          );
        })}
      </nav>
    </motion.aside>
  );
}

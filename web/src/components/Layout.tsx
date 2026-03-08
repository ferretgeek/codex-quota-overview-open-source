import type { PropsWithChildren } from 'react';
import Sidebar from './Sidebar';
import Topbar from './Topbar';
import type { HealthResponse, ThemeType } from '../types';
import './Layout.css';
import { motion } from 'framer-motion';

export type ViewKey = 'dashboard' | 'accounts' | 'windows' | 'settings';

interface LayoutProps {
  onThemeChange: (theme: ThemeType) => void;
  currentTheme: ThemeType;
  activeView: ViewKey;
  onChangeView: (view: ViewKey) => void;
  onExport: () => void;
  onRefreshAll: () => void;
  isRefreshing: boolean;
  subtitle: string;
  health: HealthResponse | null;
}

export default function Layout({
  children,
  onThemeChange,
  currentTheme,
  activeView,
  onChangeView,
  onExport,
  onRefreshAll,
  isRefreshing,
  subtitle,
  health,
}: PropsWithChildren<LayoutProps>) {
  return (
    <div className="layout-shell">
      <Sidebar activeView={activeView} onChangeView={onChangeView} />
      <div className="layout-main-wrapper">
        <Topbar
          onThemeChange={onThemeChange}
          currentTheme={currentTheme}
          onExport={onExport}
          onRefreshAll={onRefreshAll}
          isRefreshing={isRefreshing}
          subtitle={subtitle}
          health={health}
        />
        <motion.main
          className="layout-content"
          initial={{ opacity: 0, y: 10 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5, ease: [0.16, 1, 0.3, 1] }}
        >
          {children}
        </motion.main>
      </div>
    </div>
  );
}

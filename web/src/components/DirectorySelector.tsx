import { useEffect, useMemo, useRef, useState } from 'react';
import { ChevronDown, FolderOpen, HardDrive, Import } from 'lucide-react';
import type { DirectoryInfo } from '../types';

interface DirectorySelectorProps {
  directories: DirectoryInfo[];
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  compact?: boolean;
}

export default function DirectorySelector({ directories, value, onChange, placeholder = '选择目录', compact = false }: DirectorySelectorProps) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (rootRef.current && !rootRef.current.contains(event.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  const selected = useMemo(() => directories.find((item) => item.name === value) ?? null, [directories, value]);
  const displayTitle = selected?.name || (directories.length === 0 ? placeholder : value || placeholder);
  const displaySubtitle = selected
    ? `${selected.jsonCount} 个文件${selected.imported ? ' · 已导入' : ''}`
    : directories.length === 0
      ? '暂无目录'
      : '点击展开目录列表';

  return (
    <div ref={rootRef} className="directory-selector-root">
      <button
        type="button"
        className={`directory-selector-trigger ${compact ? 'compact' : ''}`}
        onClick={() => setOpen((prev) => !prev)}
      >
        <span className="directory-selector-leading"><FolderOpen size={16} /></span>
        <span className="directory-selector-content">
          <span className="directory-selector-title">{displayTitle}</span>
          <span className="directory-selector-subtitle">{displaySubtitle}</span>
        </span>
        <ChevronDown size={16} className={`directory-selector-chevron ${open ? 'open' : ''}`} />
      </button>

      {open ? (
        <div className="directory-selector-panel">
          {directories.length === 0 ? (
            <div className="directory-selector-empty">当前没有可选目录</div>
          ) : (
            directories.map((directory) => (
              <button
                key={`${directory.name}-${directory.path}`}
                type="button"
                className={`directory-selector-item ${directory.name === value ? 'active' : ''}`}
                onClick={() => {
                  onChange(directory.name);
                  setOpen(false);
                }}
              >
                <span className="directory-selector-item-icon">
                  {directory.imported ? <Import size={16} /> : <HardDrive size={16} />}
                </span>
                <span className="directory-selector-item-main">
                  <span className="directory-selector-item-title">{directory.name}</span>
                  <span className="directory-selector-item-subtitle">{directory.jsonCount} 个文件 · {shortenPath(directory.path)}</span>
                </span>
              </button>
            ))
          )}
        </div>
      ) : null}
    </div>
  );
}

function shortenPath(path: string): string {
  if (!path) return '未知路径';
  if (path.length <= 52) return path;
  return `${path.slice(0, 20)} ... ${path.slice(-20)}`;
}

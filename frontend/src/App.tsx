import { useState, useEffect, useCallback, useRef } from 'react';
import type { FuncDef } from './types/types';
import { getFunctions, getFnParams, saveFnParams, getConfig } from './api/api';
import FunctionList from './components/FunctionList';
import FunctionPanel from './components/FunctionPanel';
import ConfigPage from './components/ConfigPage';
import RunHistory from './components/RunHistory';
import './App.css';

type Page = 'function' | 'config' | 'history';

export default function App() {
  const [functions, setFunctions] = useState<FuncDef[]>([]);
  const [selected, setSelected] = useState<FuncDef | null>(null);
  const [page, setPage] = useState<Page>('function');
  const [jiraUrl, setJiraUrl] = useState('');
  const ready = useRef(false);

  useEffect(() => {
    Promise.all([
      getFunctions(),
      getFnParams('_tab').catch(() => ({})),
      getConfig().catch(() => ({})),
    ]).then(([fns, tab, cfg]) => {
      setFunctions(fns);
      const savedPage = (tab as Record<string, string>).page as Page | undefined;
      const savedFunc = (tab as Record<string, string>).func;
      if (savedPage) setPage(savedPage);
      const match = savedFunc ? fns.find(f => f.id === savedFunc) : null;
      setSelected(match || fns[0] || null);
      const url = (cfg as Record<string, string>).jira_url || '';
      setJiraUrl(url.replace(/\/+$/, ''));
      ready.current = true;
    });
  }, []);

  const saveTab = useCallback((p: Page, funcId?: string) => {
    if (!ready.current) return;
    const data: Record<string, string> = { page: p };
    if (funcId) data.func = funcId;
    saveFnParams('_tab', data);
  }, []);

  return (
    <div className="app">
      <aside className="sidebar">
        <h1 className="sidebar-title">Jira Tools</h1>
        <nav className="sidebar-nav">
          <div
            className={`nav-item ${page === 'config' ? 'active' : ''}`}
            onClick={() => { setPage('config'); saveTab('config'); }}
          >
            Настройки
          </div>
          <div
            className={`nav-item ${page === 'history' ? 'active' : ''}`}
            onClick={() => { setPage('history'); saveTab('history'); }}
          >
            История
          </div>
        </nav>
        <div className="sidebar-divider" />
        <FunctionList
          functions={functions}
          selected={selected}
          onSelect={(f) => { setSelected(f); setPage('function'); saveTab('function', f.id); }}
          isActive={page === 'function'}
        />
      </aside>
      <main className="main-panel">
        <div style={{ display: page === 'config' ? undefined : 'none' }}>
          <ConfigPage />
        </div>
        <div style={{ display: page === 'history' ? undefined : 'none' }}>
          <RunHistory />
        </div>
        {functions.map(f => (
          <div key={f.id} style={{ display: page === 'function' && selected?.id === f.id ? undefined : 'none' }}>
            <FunctionPanel func={f} jiraUrl={jiraUrl} />
          </div>
        ))}
      </main>
    </div>
  );
}

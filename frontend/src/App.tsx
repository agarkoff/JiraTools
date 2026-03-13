import { useState, useEffect } from 'react';
import type { FuncDef } from './types/types';
import { getFunctions } from './api/api';
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

  useEffect(() => {
    getFunctions().then(fns => {
      setFunctions(fns);
      if (fns.length > 0) setSelected(fns[0]);
    });
  }, []);

  return (
    <div className="app">
      <aside className="sidebar">
        <h1 className="sidebar-title">Jira Tools</h1>
        <nav className="sidebar-nav">
          <div
            className={`nav-item ${page === 'config' ? 'active' : ''}`}
            onClick={() => setPage('config')}
          >
            Настройки
          </div>
          <div
            className={`nav-item ${page === 'history' ? 'active' : ''}`}
            onClick={() => setPage('history')}
          >
            История
          </div>
        </nav>
        <div className="sidebar-divider" />
        <FunctionList
          functions={functions}
          selected={selected}
          onSelect={(f) => { setSelected(f); setPage('function'); }}
          isActive={page === 'function'}
        />
      </aside>
      <main className="main-panel">
        {page === 'config' && <ConfigPage />}
        {page === 'history' && <RunHistory />}
        {page === 'function' && selected && <FunctionPanel func={selected} />}
      </main>
    </div>
  );
}

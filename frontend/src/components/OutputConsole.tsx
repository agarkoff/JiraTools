import { useState, useEffect, useRef, useCallback } from 'react';
import * as XLSX from 'xlsx';
import type { TableData, FileData, GanttData } from '../types/types';
import GanttChart from './GanttChart';

const ISSUE_RE = /([A-Z][A-Z0-9]+-\d+)/g;

interface Props {
  lines: string[];
  tables: TableData[];
  files?: FileData[];
  gantt?: GanttData | null;
  isRunning: boolean;
  progress: { current: number; total: number } | null;
  error: string | null;
  funcName?: string;
  jiraUrl?: string;
}

function downloadFile(file: FileData) {
  const blob = new Blob([file.content], { type: 'application/xml' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = file.filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

function LinkedText({ text, jiraUrl }: { text: string; jiraUrl?: string }) {
  if (!jiraUrl) return <>{text}</>;
  const parts = text.split(ISSUE_RE);
  if (parts.length === 1) return <>{text}</>;
  return (
    <>
      {parts.map((part, i) =>
        ISSUE_RE.test(part) ? (
          <a key={i} href={`${jiraUrl}/browse/${part}`} target="_blank" rel="noopener noreferrer" className="issue-link">{part}</a>
        ) : (
          <span key={i}>{part}</span>
        )
      )}
    </>
  );
}

function downloadExcel(tables: TableData[], fileName: string) {
  const wb = XLSX.utils.book_new();
  const allTables = tables.filter(t => !t.group);
  const groups: Record<string, TableData[]> = {};
  for (const t of tables) {
    if (t.group) {
      if (!groups[t.group]) groups[t.group] = [];
      groups[t.group].push(t);
    }
  }

  if (allTables.length === 1 && Object.keys(groups).length === 0) {
    const t = allTables[0];
    const ws = XLSX.utils.aoa_to_sheet([t.headers, ...t.rows]);
    XLSX.utils.book_append_sheet(wb, ws, 'Результат');
  } else {
    allTables.forEach((t, i) => {
      const ws = XLSX.utils.aoa_to_sheet([t.headers, ...t.rows]);
      const name = t.title || (allTables.length === 1 ? 'Результат' : `Таблица ${i + 1}`);
      XLSX.utils.book_append_sheet(wb, ws, name.slice(0, 31));
    });
    for (const [, groupTables] of Object.entries(groups)) {
      for (const t of groupTables) {
        const ws = XLSX.utils.aoa_to_sheet([t.headers, ...t.rows]);
        const name = (t.title || 'Лист').slice(0, 31);
        XLSX.utils.book_append_sheet(wb, ws, name);
      }
    }
  }

  XLSX.writeFile(wb, `${fileName}.xlsx`);
}

function ResultTable({ table, jiraUrl }: { table: TableData; jiraUrl?: string }) {
  return (
    <div className="result-table-wrapper">
      <table className="result-table">
        <thead>
          <tr>
            {table.headers.map((h, hi) => (
              <th key={hi}>{h}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {table.rows.map((row, ri) => (
            <tr key={ri}>
              {row.map((cell, ci) => (
                <td key={ci}><LinkedText text={cell} jiraUrl={jiraUrl} /></td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export default function OutputConsole({ lines, tables, files, gantt, isRunning, progress, error, funcName, jiraUrl }: Props) {
  const [selectedGroup, setSelectedGroup] = useState<Record<string, string>>({});
  const bottomRef = useRef<HTMLDivElement>(null);

  const plainTables = tables.filter(t => !t.group);
  const groups: Record<string, TableData[]> = {};
  for (const t of tables) {
    if (t.group) {
      if (!groups[t.group]) groups[t.group] = [];
      groups[t.group].push(t);
    }
  }

  // Auto-select first item when group appears
  useEffect(() => {
    for (const group of Object.keys(groups)) {
      if (!selectedGroup[group] && groups[group].length > 0) {
        setSelectedGroup(prev => ({ ...prev, [group]: groups[group][0].title || '' }));
      }
    }
  }, [tables.length]);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  }, [lines.length, tables.length]);

  const handleDownload = useCallback(() => {
    downloadExcel(tables, funcName || 'export');
  }, [tables, funcName]);

  if (lines.length === 0 && tables.length === 0 && (!files || files.length === 0) && !isRunning && !error) return null;

  return (
    <div className="output-area">
      {progress && (
        <div className="progress-bar-container">
          <div className="progress-bar">
            <div
              className="progress-bar-fill"
              style={{ width: `${progress.total > 0 ? (progress.current / progress.total) * 100 : 0}%` }}
            />
          </div>
          <span>{progress.current} / {progress.total}</span>
        </div>
      )}

      {lines.length > 0 && (
        <div className="output-lines">
          {lines.map((line, i) => (
            <div className="output-line-text" key={i}><LinkedText text={line} jiraUrl={jiraUrl} /></div>
          ))}
        </div>
      )}

      {tables.length > 0 && !isRunning && (
        <div className="download-row">
          <button className="btn btn-secondary" onClick={handleDownload}>
            Скачать Excel
          </button>
        </div>
      )}

      {files && files.length > 0 && !isRunning && (
        <div className="download-row">
          {files.map((file, i) => (
            <button key={i} className="btn btn-primary" onClick={() => downloadFile(file)}>
              Скачать {file.filename}
            </button>
          ))}
        </div>
      )}

      {plainTables.map((table, ti) => (
        <ResultTable table={table} key={`plain-${ti}`} jiraUrl={jiraUrl} />
      ))}

      {gantt && !isRunning && <GanttChart data={gantt} jiraUrl={jiraUrl} />}

      {Object.entries(groups).map(([group, groupTables]) => {
        const sel = selectedGroup[group] || groupTables[0]?.title || '';
        const active = groupTables.find(t => t.title === sel);
        return (
          <div key={group} className="grouped-tables">
            <div className="group-selector">
              <select
                value={sel}
                onChange={e => setSelectedGroup(prev => ({ ...prev, [group]: e.target.value }))}
              >
                {groupTables.map(t => (
                  <option key={t.title} value={t.title}>{t.title}</option>
                ))}
              </select>
            </div>
            {active && <ResultTable table={active} jiraUrl={jiraUrl} />}
          </div>
        );
      })}

      {isRunning && <div className="output-lines"><div className="output-line-text" style={{ color: 'var(--primary)' }}>Выполняется...</div></div>}
      {error && <div className="output-lines"><div className="output-line-text" style={{ color: 'var(--danger)' }}>[ОШИБКА] {error}</div></div>}
      <div ref={bottomRef} />
    </div>
  );
}

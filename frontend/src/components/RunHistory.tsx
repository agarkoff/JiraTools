import { useState, useEffect } from 'react';
import type { Run } from '../types/types';
import { getRuns, getRunOutput, deleteRun } from '../api/api';

export default function RunHistory() {
  const [runs, setRuns] = useState<Run[]>([]);
  const [selectedOutput, setSelectedOutput] = useState<{ id: number; lines: string[] } | null>(null);

  const loadRuns = () => {
    getRuns(100).then(setRuns);
  };

  useEffect(loadRuns, []);

  const handleView = async (id: number) => {
    if (selectedOutput?.id === id) {
      setSelectedOutput(null);
      return;
    }
    const output = await getRunOutput(id);
    setSelectedOutput({ id, lines: output.map(l => l.text) });
  };

  const handleDelete = async (id: number) => {
    await deleteRun(id);
    if (selectedOutput?.id === id) setSelectedOutput(null);
    loadRuns();
  };

  const formatDate = (s: string) => {
    const d = new Date(s);
    return d.toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' });
  };

  const duration = (run: Run) => {
    if (!run.finished_at) return '-';
    const ms = new Date(run.finished_at).getTime() - new Date(run.started_at).getTime();
    const sec = Math.round(ms / 1000);
    if (sec < 60) return `${sec}с`;
    return `${Math.floor(sec / 60)}м ${sec % 60}с`;
  };

  return (
    <div>
      <div className="panel-header">
        <h2>История запусков</h2>
      </div>

      {runs.length === 0 ? (
        <p style={{ color: '#64748b' }}>Нет запусков</p>
      ) : (
        <table className="history-table">
          <thead>
            <tr>
              <th>Дата</th>
              <th>Функция</th>
              <th>Статус</th>
              <th>Время</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {runs.map(r => (
              <tr key={r.id}>
                <td>{formatDate(r.started_at)}</td>
                <td>{r.function}</td>
                <td>
                  <span className={`status-badge status-${r.status}`}>
                    {r.status === 'running' ? 'выполняется' : r.status === 'completed' ? 'готово' : 'ошибка'}
                  </span>
                </td>
                <td>{duration(r)}</td>
                <td className="actions">
                  <button className="btn btn-secondary" onClick={() => handleView(r.id)} style={{ padding: '4px 10px', fontSize: 12 }}>
                    {selectedOutput?.id === r.id ? 'Скрыть' : 'Лог'}
                  </button>
                  <button className="btn btn-danger" onClick={() => handleDelete(r.id)} style={{ padding: '4px 10px', fontSize: 12 }}>
                    &times;
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {selectedOutput && (
        <div className="output-console" style={{ marginTop: 16 }}>
          {selectedOutput.lines.map((line, i) => (
            <div className="output-line" key={i}>{line}</div>
          ))}
          {selectedOutput.lines.length === 0 && (
            <div style={{ color: '#64748b' }}>Нет вывода</div>
          )}
        </div>
      )}
    </div>
  );
}

import { useState, useEffect } from 'react';
import { getConfig, updateConfig, testConnection, testGitlab, getUsers, addUser, deleteUser, getVacations, addVacation, deleteVacation } from '../api/api';
import type { Vacation } from '../api/api';

export default function ConfigPage() {
  const [cfg, setCfg] = useState<Record<string, string>>({});
  const [users, setUsers] = useState<string[]>([]);
  const [newUser, setNewUser] = useState('');
  const [testResult, setTestResult] = useState<{ status: string; message: string } | null>(null);
  const [gitlabTestResult, setGitlabTestResult] = useState<{ status: string; message: string } | null>(null);
  const [saving, setSaving] = useState(false);
  const [vacations, setVacations] = useState<Vacation[]>([]);
  const [expandedUser, setExpandedUser] = useState<string | null>(null);
  const [vacForm, setVacForm] = useState<{ from: string; to: string; comment: string }>({ from: '', to: '', comment: '' });

  const loadVacations = () => getVacations().then(setVacations);

  useEffect(() => {
    getConfig().then(setCfg);
    getUsers().then(setUsers);
    loadVacations();
  }, []);

  const handleSave = async () => {
    setSaving(true);
    try {
      await updateConfig(cfg);
      setTestResult({ status: 'ok', message: 'Сохранено' });
    } catch (e) {
      setTestResult({ status: 'error', message: String(e) });
    }
    setSaving(false);
  };

  const handleTest = async () => {
    setTestResult(null);
    try {
      const r = await testConnection();
      setTestResult(r);
    } catch (e) {
      setTestResult({ status: 'error', message: String(e) });
    }
  };

  const handleAddUser = async () => {
    if (!newUser.trim()) return;
    await addUser(newUser.trim());
    setNewUser('');
    setUsers(await getUsers());
  };

  const handleDeleteUser = async (login: string) => {
    await deleteUser(login);
    setUsers(await getUsers());
  };

  const handleAddVacation = async (login: string) => {
    if (!vacForm.from || !vacForm.to) return;
    await addVacation(login, vacForm.from, vacForm.to, vacForm.comment);
    setVacForm({ from: '', to: '', comment: '' });
    await loadVacations();
  };

  const handleDeleteVacation = async (id: number) => {
    await deleteVacation(id);
    await loadVacations();
  };

  const userVacations = (login: string) => vacations.filter(v => v.user_login === login);

  return (
    <div className="config-page">
      <h2>Настройки</h2>

      <div className="config-section">
        <h3>Подключение к Jira</h3>
        <div className="param-form">
          <div className="form-group">
            <label>URL Jira</label>
            <input
              type="text"
              value={cfg.jira_url || ''}
              onChange={e => setCfg({ ...cfg, jira_url: e.target.value })}
              placeholder="https://jira.example.com"
            />
          </div>
          <div className="form-group">
            <label>Логин</label>
            <input
              type="text"
              value={cfg.jira_login || ''}
              onChange={e => setCfg({ ...cfg, jira_login: e.target.value })}
            />
          </div>
          <div className="form-group">
            <label>Пароль</label>
            <input
              type="password"
              value={cfg.jira_password || ''}
              onChange={e => setCfg({ ...cfg, jira_password: e.target.value })}
            />
          </div>
        </div>
        <div className="btn-row">
          <button className="btn btn-primary" onClick={handleSave} disabled={saving}>
            Сохранить
          </button>
          <button className="btn btn-secondary" onClick={handleTest}>
            Проверить подключение
          </button>
        </div>
        {testResult && (
          <p style={{ marginTop: 8, color: testResult.status === 'ok' ? 'var(--success)' : 'var(--danger)', fontSize: 14 }}>
            {testResult.message}
          </p>
        )}
      </div>

      <div className="config-section">
        <h3>GitLab</h3>
        <div className="param-form">
          <div className="form-group">
            <label>URL GitLab</label>
            <input
              type="text"
              value={cfg.gitlab_url || ''}
              onChange={e => setCfg({ ...cfg, gitlab_url: e.target.value })}
              placeholder="https://gitlab.example.com"
            />
          </div>
          <div className="form-group">
            <label>Private Token</label>
            <input
              type="password"
              value={cfg.gitlab_token || ''}
              onChange={e => setCfg({ ...cfg, gitlab_token: e.target.value })}
            />
          </div>
        </div>
        <div className="btn-row">
          <button className="btn btn-secondary" onClick={async () => {
            setGitlabTestResult(null);
            try {
              const r = await testGitlab();
              setGitlabTestResult(r);
            } catch (e) {
              setGitlabTestResult({ status: 'error', message: String(e) });
            }
          }}>
            Проверить подключение
          </button>
        </div>
        {gitlabTestResult && (
          <p style={{ marginTop: 8, color: gitlabTestResult.status === 'ok' ? 'var(--success)' : 'var(--danger)', fontSize: 14 }}>
            {gitlabTestResult.message}
          </p>
        )}
      </div>

      <div className="config-section">
        <h3>Ollama (LLM)</h3>
        <div className="param-form">
          <div className="form-group">
            <label>URL Ollama</label>
            <input
              type="text"
              value={cfg.ollama_url || ''}
              onChange={e => setCfg({ ...cfg, ollama_url: e.target.value })}
              placeholder="http://host.docker.internal:11434"
            />
          </div>
          <div className="form-group">
            <label>Модель</label>
            <input
              type="text"
              value={cfg.ollama_model || ''}
              onChange={e => setCfg({ ...cfg, ollama_model: e.target.value })}
              placeholder="qwen3-coder:30b"
            />
          </div>
        </div>
      </div>

      <div className="config-section">
        <h3>Пользователи</h3>
        <div className="users-list">
          {users.map(u => {
            const uv = userVacations(u);
            const isExpanded = expandedUser === u;
            return (
              <div className="user-block" key={u}>
                <div className="user-item">
                  <span
                    className="user-name-toggle"
                    onClick={() => setExpandedUser(isExpanded ? null : u)}
                    title="Показать/скрыть отпуска"
                  >
                    {u}
                    {uv.length > 0 && <span className="vacation-count">{uv.length}</span>}
                  </span>
                  <button onClick={() => handleDeleteUser(u)} title="Удалить">&times;</button>
                </div>
                {isExpanded && (
                  <div className="vacation-panel">
                    {uv.length > 0 && (
                      <div className="vacation-list">
                        {uv.map(v => (
                          <div className="vacation-item" key={v.id}>
                            <span className="vacation-dates">{v.date_from} &mdash; {v.date_to}</span>
                            {v.comment && <span className="vacation-comment">{v.comment}</span>}
                            <button className="vacation-delete" onClick={() => handleDeleteVacation(v.id)} title="Удалить">&times;</button>
                          </div>
                        ))}
                      </div>
                    )}
                    <div className="vacation-form">
                      <input type="date" value={vacForm.from} onChange={e => setVacForm({ ...vacForm, from: e.target.value })} />
                      <span>&mdash;</span>
                      <input type="date" value={vacForm.to} onChange={e => setVacForm({ ...vacForm, to: e.target.value })} />
                      <input
                        type="text"
                        value={vacForm.comment}
                        onChange={e => setVacForm({ ...vacForm, comment: e.target.value })}
                        placeholder="Комментарий"
                        className="vacation-comment-input"
                      />
                      <button className="btn btn-secondary" onClick={() => handleAddVacation(u)}>+</button>
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
        <div className="add-user-row">
          <input
            type="text"
            value={newUser}
            onChange={e => setNewUser(e.target.value)}
            placeholder="Логин пользователя"
            onKeyDown={e => e.key === 'Enter' && handleAddUser()}
          />
          <button className="btn btn-secondary" onClick={handleAddUser}>Добавить</button>
        </div>
      </div>
    </div>
  );
}

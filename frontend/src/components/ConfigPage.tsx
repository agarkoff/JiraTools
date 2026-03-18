import { useState, useEffect } from 'react';
import { getConfig, updateConfig, testConnection, getUsers, addUser, deleteUser } from '../api/api';

export default function ConfigPage() {
  const [cfg, setCfg] = useState<Record<string, string>>({});
  const [users, setUsers] = useState<string[]>([]);
  const [newUser, setNewUser] = useState('');
  const [testResult, setTestResult] = useState<{ status: string; message: string } | null>(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    getConfig().then(setCfg);
    getUsers().then(setUsers);
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
        <ul className="user-list">
          {users.map(u => (
            <li className="user-item" key={u}>
              <span>{u}</span>
              <button onClick={() => handleDeleteUser(u)} title="Удалить">&times;</button>
            </li>
          ))}
        </ul>
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

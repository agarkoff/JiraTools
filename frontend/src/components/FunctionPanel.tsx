import { useState, useEffect, useRef, useCallback } from 'react';
import type { FuncDef, TableData, FileData, GanttData } from '../types/types';
import { runFunction, getFnParams, saveFnParams } from '../api/api';
import ParamForm from './ParamForm';
import OutputConsole, { downloadExcel } from './OutputConsole';

interface Props {
  func: FuncDef;
  jiraUrl?: string;
}

function buildDefaults(fn: FuncDef): Record<string, string> {
  const defaults: Record<string, string> = {};
  fn.params.forEach(p => {
    if (p.default !== undefined) {
      defaults[p.name] = p.default;
    } else if (p.type === 'multicheck' && p.options) {
      defaults[p.name] = p.options.join(',');
    }
  });
  return defaults;
}

export default function FunctionPanel({ func: fn, jiraUrl }: Props) {
  const [values, setValues] = useState<Record<string, string>>(() => buildDefaults(fn));

  useEffect(() => {
    getFnParams(fn.id).then(saved => {
      if (saved && Object.keys(saved).length > 0) {
        setValues(prev => ({ ...prev, ...saved }));
      }
    }).catch(() => {});
  }, [fn.id]);
  const [lines, setLines] = useState<string[]>([]);
  const [tables, setTables] = useState<TableData[]>([]);
  const [files, setFiles] = useState<FileData[]>([]);
  const [gantt, setGantt] = useState<GanttData | null>(null);
  const [isRunning, setIsRunning] = useState(false);
  const [progress, setProgress] = useState<{ current: number; total: number } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  const saveTimer = useRef<number | undefined>(undefined);
  const handleChange = useCallback((name: string, value: string) => {
    setValues(prev => {
      const next = { ...prev, [name]: value };
      window.clearTimeout(saveTimer.current);
      saveTimer.current = window.setTimeout(() => saveFnParams(fn.id, next), 500);
      return next;
    });
  }, [fn.id]);

  const handleRun = useCallback(async () => {
    for (const p of fn.params) {
      if (p.required && !values[p.name]) {
        setError(`Заполните поле: ${p.label}`);
        return;
      }
    }

    setLines([]);
    setTables([]);
    setFiles([]);
    setGantt(null);
    setError(null);
    setProgress(null);
    setIsRunning(true);

    const controller = new AbortController();
    abortRef.current = controller;

    try {
      await runFunction(fn.id, values, {
        onOutput: (line) => {
          setLines(prev => [...prev, line]);
        },
        onProgress: (current, total) => {
          setProgress({ current, total });
        },
        onTable: (table) => {
          setTables(prev => [...prev, table]);
        },
        onFile: (file) => {
          setFiles(prev => [...prev, file]);
        },
        onGantt: (data) => {
          setGantt(data);
        },
        onError: (msg) => {
          setError(msg);
        },
        onCompleted: () => {
          setProgress(null);
        },
      }, controller.signal);
    } catch (e) {
      if (e instanceof DOMException && e.name === 'AbortError') {
        setError('Отменено');
      } else {
        setError(String(e));
      }
    } finally {
      setIsRunning(false);
      abortRef.current = null;
    }
  }, [fn, values]);

  const handleCancel = useCallback(() => {
    abortRef.current?.abort();
  }, []);

  const handleExcel = useCallback(() => {
    downloadExcel(tables, fn.name || 'export');
  }, [tables, fn.name]);

  const buttons = (
    <>
      <button className="btn btn-primary" onClick={handleRun} disabled={isRunning}>
        {isRunning ? 'Выполняется...' : 'Запустить'}
      </button>
      {isRunning && (
        <button className="btn btn-secondary" onClick={handleCancel}>
          Отмена
        </button>
      )}
      {tables.length > 0 && !isRunning && (
        <button className="btn btn-success" onClick={handleExcel}>
          Скачать Excel
        </button>
      )}
    </>
  );

  return (
    <div>
      <div className="panel-header">
        <h2>{fn.name}</h2>
        <p>{fn.description}</p>
      </div>

      {fn.layout === 'inline' ? (
        <div className="inline-controls">
          <ParamForm
            params={fn.params}
            values={values}
            onChange={handleChange}
            disabled={isRunning}
            inline
          />
          {buttons}
        </div>
      ) : (
        <>
          <ParamForm
            params={fn.params}
            values={values}
            onChange={handleChange}
            disabled={isRunning}
          />
          <div className="btn-row">
            {buttons}
          </div>
        </>
      )}

      <OutputConsole
        lines={lines}
        tables={tables}
        files={files}
        gantt={gantt}
        isRunning={isRunning}
        progress={progress}
        error={error}
        funcName={fn.name}
        jiraUrl={jiraUrl}
      />
    </div>
  );
}

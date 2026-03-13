import { useState, useRef, useCallback } from 'react';
import type { FuncDef } from '../types/types';
import { runFunction } from '../api/api';
import ParamForm from './ParamForm';
import OutputConsole from './OutputConsole';

interface Props {
  func: FuncDef;
}

export default function FunctionPanel({ func: fn }: Props) {
  const [values, setValues] = useState<Record<string, string>>(() => {
    const defaults: Record<string, string> = {};
    fn.params.forEach(p => {
      if (p.default !== undefined) defaults[p.name] = p.default;
    });
    return defaults;
  });
  const [lines, setLines] = useState<string[]>([]);
  const [isRunning, setIsRunning] = useState(false);
  const [progress, setProgress] = useState<{ current: number; total: number } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  // Reset defaults when function changes
  const prevFnId = useRef(fn.id);
  if (prevFnId.current !== fn.id) {
    prevFnId.current = fn.id;
    const defaults: Record<string, string> = {};
    fn.params.forEach(p => {
      if (p.default !== undefined) defaults[p.name] = p.default;
    });
    setValues(defaults);
    setLines([]);
    setError(null);
    setProgress(null);
    setIsRunning(false);
  }

  const handleChange = useCallback((name: string, value: string) => {
    setValues(prev => ({ ...prev, [name]: value }));
  }, []);

  const handleRun = useCallback(async () => {
    // Validate required params
    for (const p of fn.params) {
      if (p.required && !values[p.name]) {
        setError(`Заполните поле: ${p.label}`);
        return;
      }
    }

    setLines([]);
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

  return (
    <div>
      <div className="panel-header">
        <h2>{fn.name}</h2>
        <p>{fn.description}</p>
      </div>

      <ParamForm
        params={fn.params}
        values={values}
        onChange={handleChange}
        disabled={isRunning}
      />

      <div className="btn-row">
        <button className="btn btn-primary" onClick={handleRun} disabled={isRunning}>
          {isRunning ? 'Выполняется...' : 'Запустить'}
        </button>
        {isRunning && (
          <button className="btn btn-secondary" onClick={handleCancel}>
            Отмена
          </button>
        )}
      </div>

      <OutputConsole
        lines={lines}
        isRunning={isRunning}
        progress={progress}
        error={error}
      />
    </div>
  );
}

import type { FuncDef, Run, RunOutputLine, TableData, FileData, GanttData } from '../types/types';

const API = '/api';

async function fetchJson<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...options?.headers,
    },
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `HTTP ${res.status}`);
  }
  if (res.status === 204) return null as T;
  return res.json();
}

// Config
export async function getConfig(): Promise<Record<string, string>> {
  return fetchJson(`${API}/config`);
}

export async function updateConfig(cfg: Record<string, string>): Promise<void> {
  return fetchJson(`${API}/config`, { method: 'PUT', body: JSON.stringify(cfg) });
}

export async function testConnection(): Promise<{ status: string; message: string }> {
  return fetchJson(`${API}/config/test`, { method: 'POST' });
}

// Users
export async function getUsers(): Promise<string[]> {
  return fetchJson(`${API}/users`);
}

export async function addUser(login: string): Promise<void> {
  return fetchJson(`${API}/users`, { method: 'POST', body: JSON.stringify({ login }) });
}

export async function deleteUser(login: string): Promise<void> {
  return fetchJson(`${API}/users/${encodeURIComponent(login)}`, { method: 'DELETE' });
}

// Function params (saved state)
export async function getFnParams(funcId: string): Promise<Record<string, string>> {
  return fetchJson(`${API}/fn-params/${funcId}`);
}

export async function saveFnParams(funcId: string, params: Record<string, string>): Promise<void> {
  return fetchJson(`${API}/fn-params/${funcId}`, { method: 'PUT', body: JSON.stringify(params) });
}

// Functions
export async function getFunctions(): Promise<FuncDef[]> {
  return fetchJson(`${API}/functions`);
}

// Runs
export async function getRuns(limit = 50, offset = 0, fn?: string): Promise<Run[]> {
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
  if (fn) params.set('function', fn);
  return fetchJson(`${API}/runs?${params}`);
}

export async function getRunOutput(id: number): Promise<RunOutputLine[]> {
  return fetchJson(`${API}/runs/${id}/output`);
}

export async function deleteRun(id: number): Promise<void> {
  return fetchJson(`${API}/runs/${id}`, { method: 'DELETE' });
}

// SSE streaming for function execution
export interface SSECallbacks {
  onStarted?: (runId: number) => void;
  onOutput?: (line: string, lineNum: number) => void;
  onProgress?: (current: number, total: number) => void;
  onTable?: (table: TableData) => void;
  onFile?: (file: FileData) => void;
  onGantt?: (data: GanttData) => void;
  onCompleted?: (runId: number) => void;
  onError?: (message: string) => void;
}

export async function runFunction(
  name: string,
  params: Record<string, string>,
  callbacks: SSECallbacks,
  signal?: AbortSignal,
): Promise<void> {
  const res = await fetch(`${API}/functions/${name}/run`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Accept': 'text/event-stream',
    },
    body: JSON.stringify(params),
    signal,
  });

  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `HTTP ${res.status}`);
  }

  const reader = res.body!.getReader();
  const decoder = new TextDecoder();
  let buffer = '';

  const parseEvent = (block: string) => {
    let eventName = '';
    let eventData = '';
    for (const line of block.split('\n')) {
      if (line.startsWith('event:')) {
        eventName = line.slice(6).trim();
      } else if (line.startsWith('data:')) {
        eventData += (eventData ? '\n' : '') + line.slice(5);
      }
    }
    if (!eventName || !eventData) return;

    try {
      const data = JSON.parse(eventData);
      switch (eventName) {
        case 'started':
          callbacks.onStarted?.(data.run_id);
          break;
        case 'output':
          callbacks.onOutput?.(data.line, data.line_num);
          break;
        case 'progress':
          callbacks.onProgress?.(data.current, data.total);
          break;
        case 'completed':
          callbacks.onCompleted?.(data.run_id);
          break;
        case 'table':
          callbacks.onTable?.(data as TableData);
          break;
        case 'file':
          callbacks.onFile?.(data as FileData);
          break;
        case 'gantt':
          callbacks.onGantt?.(data as GanttData);
          break;
        case 'error':
          callbacks.onError?.(data.message);
          break;
      }
    } catch {
      // ignore parse errors
    }
  };

  const processBuffer = () => {
    buffer = buffer.replace(/\r\n/g, '\n').replace(/\r/g, '\n');
    let idx;
    while ((idx = buffer.indexOf('\n\n')) !== -1) {
      const block = buffer.slice(0, idx);
      buffer = buffer.slice(idx + 2);
      parseEvent(block);
    }
  };

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    processBuffer();
  }

  buffer += decoder.decode();
  processBuffer();
  if (buffer.trim()) {
    parseEvent(buffer.replace(/\r\n/g, '\n').replace(/\r/g, '\n'));
  }
}

export interface FuncParam {
  name: string;
  type: 'string' | 'number' | 'boolean' | 'select' | 'textarea';
  label: string;
  required: boolean;
  default?: string;
  options?: string[];
}

export interface FuncDef {
  id: string;
  name: string;
  description: string;
  params: FuncParam[];
}

export interface Run {
  id: number;
  function: string;
  params: Record<string, string>;
  status: 'running' | 'completed' | 'error';
  error: string | null;
  started_at: string;
  finished_at: string | null;
}

export interface RunOutputLine {
  line_num: number;
  text: string;
}

export interface SSEOutputEvent {
  line: string;
  line_num: number;
}

export interface SSEProgressEvent {
  current: number;
  total: number;
}

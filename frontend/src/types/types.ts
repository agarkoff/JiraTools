export interface FuncParam {
  name: string;
  type: 'string' | 'number' | 'boolean' | 'select' | 'textarea' | 'multicheck';
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
  layout?: string;
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

export interface TableData {
  headers: string[];
  rows: string[][];
  title?: string;
  group?: string;
}

export interface FileData {
  filename: string;
  content: string;
}

export interface GanttTask {
  key: string;
  summary: string;
  start: string;
  end: string;
  start_frac: number;
  end_frac: number;
  due_date?: string;
  estimate_hours: number;
  overdue: boolean;
  status: string;
  priority_id: string;
  priority_name: string;
}

export interface GanttVacation {
  date_from: string;
  date_to: string;
  comment: string;
}

export interface GanttUser {
  name: string;
  tasks: GanttTask[];
  vacations?: GanttVacation[];
  overloaded: boolean;
  total_hours: number;
}

export interface GanttData {
  users: GanttUser[];
  date_start: string;
  date_end: string;
  today: string;
  non_working_days?: string[];
}

export interface RunEvent {
  seq: number;
  type: string;
  data: TableData | GanttData | FileData;
}

export interface LatestResult {
  run: Run;
  lines: RunOutputLine[];
  events: RunEvent[];
}

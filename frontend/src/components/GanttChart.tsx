import { useMemo, useState } from 'react';
import type { GanttData, GanttTask } from '../types/types';

interface Props {
  data: GanttData;
  jiraUrl?: string;
}

function parseDate(s: string): Date {
  const [y, m, d] = s.split('-').map(Number);
  return new Date(y, m - 1, d);
}

function fmtDate(d: Date): string {
  return `${d.getDate()}.${String(d.getMonth() + 1).padStart(2, '0')}`;
}

function daysBetween(a: Date, b: Date): number {
  return Math.round((b.getTime() - a.getTime()) / 86400000);
}

function toKey(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

const KEY_COL = 120;


export default function GanttChart({ data, jiraUrl }: Props) {
  const startDate = useMemo(() => parseDate(data.date_start), [data.date_start]);
  const endDate = useMemo(() => parseDate(data.date_end), [data.date_end]);
  const totalDays = daysBetween(startDate, endDate) + 1;

  const nonWorkingSet = useMemo(
    () => new Set(data.non_working_days || []),
    [data.non_working_days],
  );
  const isNonWorking = (d: Date) => nonWorkingSet.has(toKey(d));

  const days = useMemo(() => {
    const result: Date[] = [];
    const d = new Date(startDate);
    for (let i = 0; i < totalDays; i++) {
      result.push(new Date(d));
      d.setDate(d.getDate() + 1);
    }
    return result;
  }, [startDate, totalDays]);

  if (totalDays <= 0 || data.users.length === 0) return null;

  const dayPct = 100 / totalDays;

  const pct = (dateStr: string) => {
    const d = parseDate(dateStr);
    return (daysBetween(startDate, d) / totalDays) * 100;
  };

  const taskLeft = (task: GanttTask) => {
    const dayIdx = daysBetween(startDate, parseDate(task.start));
    return ((dayIdx + (task.start_frac || 0)) / totalDays) * 100;
  };

  const taskWidth = (task: GanttTask) => {
    const startIdx = daysBetween(startDate, parseDate(task.start));
    const endIdx = daysBetween(startDate, parseDate(task.end));
    const left = startIdx + (task.start_frac || 0);
    const right = endIdx + (task.end_frac || 1);
    return Math.max(((right - left) / totalDays) * 100, dayPct * 0.3);
  };

  const todayPct = pct(data.today);
  const minTimelineWidth = Math.max(400, totalDays * 38);

  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});

  const toggle = (name: string) =>
    setCollapsed(prev => ({ ...prev, [name]: !prev[name] }));

  return (
    <div className="gantt">
      {data.users.map(user => (
        <div key={user.name} className="gantt-section">
          <div className="gantt-user-header" onClick={() => toggle(user.name)}>
            <span className={`gantt-toggle ${collapsed[user.name] ? 'collapsed' : ''}`} />
            <span className="gantt-user-label">{user.name}</span>
            <span className="gantt-user-hours">{user.total_hours.toFixed(0)}ч</span>
            {user.tasks.length > 0 && (
              <span className={`gantt-verdict ${user.overloaded ? 'bad' : 'good'}`}>
                {user.overloaded ? 'Не успевает' : 'Успевает'}
              </span>
            )}
          </div>
          {!collapsed[user.name] && (
            <div className="gantt-scroll">
              <div className="gantt-body" style={{ minWidth: minTimelineWidth + KEY_COL }}>
                {/* Date header row — same flex layout as task rows */}
                <div className="gantt-row gantt-dates-row">
                  <div className="gantt-key-col" />
                  <div className="gantt-timeline">
                    {days.map((d, i) => (
                      <div
                        key={i}
                        className={`gantt-date-cell ${isNonWorking(d) ? 'weekend' : ''}`}
                        style={{ left: `${(i / totalDays) * 100}%`, width: `${dayPct}%` }}
                      >
                        {fmtDate(d)}
                      </div>
                    ))}
                  </div>
                </div>

                {/* Task rows */}
                <div className="gantt-rows">
                  {/* Background: weekends + today (once, full height, aligned to timeline) */}
                  <div className="gantt-bg" style={{ left: KEY_COL }}>
                    {days.map((d, di) => isNonWorking(d) ? (
                      <div key={di} className="gantt-weekend"
                        style={{ left: `${(di / totalDays) * 100}%`, width: `${dayPct}%` }} />
                    ) : null)}
                    <div className="gantt-today" style={{ left: `${todayPct + dayPct * 0.5}%` }} />
                  </div>

                  {user.tasks.map((task, ti) => (
                    <div key={ti} className="gantt-row">
                      <div className="gantt-key-col">
                        {jiraUrl ? (
                          <a href={`${jiraUrl}/browse/${task.key}`} target="_blank" rel="noopener noreferrer">
                            {task.key}
                          </a>
                        ) : task.key}
                      </div>
                      <div className="gantt-timeline">
                        <div
                          className={`gantt-bar ${task.overdue ? 'overdue' : ''}`}
                          style={{
                            left: `${taskLeft(task)}%`,
                            width: `${taskWidth(task)}%`,
                          }}
                          title={`${task.key}: ${task.summary}\nПриоритет: ${task.priority_name || '—'}\nОценка: ${task.estimate_hours > 0 ? task.estimate_hours.toFixed(0) + 'ч' : 'нет'}\nСрок: ${task.due_date || 'нет'}\nСтатус: ${task.status}`}
                        />
                        {task.due_date && (
                          <div
                            className={`gantt-due ${task.overdue ? 'overdue' : ''}`}
                            style={{ left: `${pct(task.due_date) + dayPct * 0.5}%` }}
                            title={`Срок: ${task.due_date}`}
                          />
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          )}
        </div>
      ))}
    </div>
  );
}

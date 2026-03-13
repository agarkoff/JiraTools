import { useEffect, useRef } from 'react';

interface Props {
  lines: string[];
  isRunning: boolean;
  progress: { current: number; total: number } | null;
  error: string | null;
}

export default function OutputConsole({ lines, isRunning, progress, error }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [lines.length]);

  if (lines.length === 0 && !isRunning && !error) return null;

  return (
    <div>
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
      <div className="output-console" ref={containerRef}>
        {lines.map((line, i) => (
          <div className="output-line" key={i}>{line}</div>
        ))}
        {isRunning && <div className="output-line" style={{ color: '#89b4fa' }}>...</div>}
        {error && <div className="output-line" style={{ color: '#f38ba8' }}>[ОШИБКА] {error}</div>}
      </div>
    </div>
  );
}

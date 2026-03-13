import type { FuncParam } from '../types/types';

interface Props {
  params: FuncParam[];
  values: Record<string, string>;
  onChange: (name: string, value: string) => void;
  disabled: boolean;
}

export default function ParamForm({ params, values, onChange, disabled }: Props) {
  return (
    <div className="param-form">
      {params.map(p => (
        <div className="form-group" key={p.name}>
          {p.type === 'boolean' ? (
            <div className="checkbox-row">
              <input
                type="checkbox"
                id={p.name}
                checked={values[p.name] === 'true'}
                onChange={e => onChange(p.name, e.target.checked ? 'true' : 'false')}
                disabled={disabled}
              />
              <label htmlFor={p.name}>{p.label}</label>
            </div>
          ) : (
            <>
              <label htmlFor={p.name}>
                {p.label}
                {p.required && <span style={{ color: 'var(--danger)' }}> *</span>}
              </label>
              {p.type === 'select' ? (
                <select
                  id={p.name}
                  value={values[p.name] || ''}
                  onChange={e => onChange(p.name, e.target.value)}
                  disabled={disabled}
                >
                  {p.options?.map(opt => (
                    <option key={opt} value={opt}>{opt}</option>
                  ))}
                </select>
              ) : p.type === 'textarea' ? (
                <textarea
                  id={p.name}
                  value={values[p.name] || ''}
                  onChange={e => onChange(p.name, e.target.value)}
                  disabled={disabled}
                  rows={5}
                />
              ) : (
                <input
                  type={p.type === 'number' ? 'number' : 'text'}
                  id={p.name}
                  value={values[p.name] || ''}
                  onChange={e => onChange(p.name, e.target.value)}
                  disabled={disabled}
                />
              )}
            </>
          )}
        </div>
      ))}
    </div>
  );
}

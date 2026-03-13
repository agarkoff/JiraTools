import type { FuncDef } from '../types/types';

interface Props {
  functions: FuncDef[];
  selected: FuncDef | null;
  onSelect: (f: FuncDef) => void;
  isActive: boolean;
}

export default function FunctionList({ functions, selected, onSelect, isActive }: Props) {
  return (
    <div>
      <div className="func-list-title">Функции</div>
      {functions.map(f => (
        <div
          key={f.id}
          className={`func-item ${isActive && selected?.id === f.id ? 'active' : ''}`}
          onClick={() => onSelect(f)}
        >
          {f.name}
        </div>
      ))}
    </div>
  );
}

import React from "react";

interface InputTerminalProps {
  value: string;
  onChange: (value: string) => void;
}

const InputTerminal = ({ value, onChange }: InputTerminalProps) => {
  return (
    <div className="rounded-lg overflow-hidden shadow-lg border border-gray-200 dark:border-gray-700">
      <div className="bg-gray-100 dark:bg-gray-800 px-4 py-2 border-b border-gray-200 dark:border-gray-700">
        <div className="flex space-x-2">
          <p>Terminal</p>
        </div>
      </div>
      <textarea
        className="w-full bg-white dark:bg-gray-900 text-gray-800 dark:text-gray-100 p-4 font-mono resize-none focus:outline-none"
        style={{height: "300px"}}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder="Ingrese sus comandos aquí..."
        spellCheck="false"
      />
    </div>
  );
};

export default InputTerminal;

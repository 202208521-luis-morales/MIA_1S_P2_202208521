import { useState } from "react";
import { ElementDisplay, type ElementDisplayProps } from "./ElementDisplay";

type FilesPartProps = {
    elems: ElementDisplayProps[]
    selectedValue: string;
    onChange: (value: string) => void;
};

const FilesPart = ({ elems, selectedValue, onChange }: FilesPartProps) => {
    console.log({ elems })
    const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        onChange(e.target.value);
    };

    return <div className="flex flex-wrap space-x-3 space-y-3">
        {elems.map((option) => (
            <div key={option.name}>
                <label className="block mb-2">
                    <input
                        type="radio"
                        name="si_sale"
                        value={option.name}
                        checked={selectedValue === option.name}
                        onChange={handleChange}
                        className="mr-2 hidden"
                    />
                    <ElementDisplay
                        name={option.name}
                        type={option.type}
                        isActive={selectedValue === option.name}
                    ></ElementDisplay>
                </label>
            </div>
        ))}
    </div>
}

export { FilesPart }
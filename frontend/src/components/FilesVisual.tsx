import { useState } from "react";
import { FilesPart } from "./FilesPart";
import { FilesPart2 } from "./FilesPart2";
import type { ElementDisplayProps } from "./ElementDisplay";

const elems: ElementDisplayProps[] = [{
    name: "Disco1.mia",
    type: "disk"
}, {
    name: "Discoasas2.mia",
    type: "folder"
}]

const info: { field: string, value: string | number }[] = [{
    field: "block_start",
    value: 12
},
{
    field: "inode_start",
    value: 13444
}]

export default function FilesVisual() {
    const [selected, setSelected] = useState<string>("");

    function onSeleccionarClicked() {
        console.log("onSeleccionarClickedWorks")
    }

    function onRegresarClicked() {
        console.log("onRegresarClickedWorks")
    }

    return <div className="p-9 rounded-lg shadow-md w-full border border-slate-600">
        <h2 className="text-3xl font-extrabold mb-9">Visualizador de Archivos</h2>
        <div className="grid grid-cols-2">
            <div className="border border-slate-600 p-5">
                <FilesPart
                    elems={elems}
                    onChange={setSelected}
                    selectedValue={selected}
                ></FilesPart>
            </div>
            <div className="border border-slate-600 p-5">
                <FilesPart2
                    elems={info}
                    onRegresarClicked={onRegresarClicked}
                    onSeleccionarClicked={onSeleccionarClicked}
                ></FilesPart2>
            </div>
        </div>
    </div>
}
type FilesPart2Props = {
    elems: {
        field: string
        value: string | number
    }[];
    onRegresarClicked: (value: string) => void
    onSeleccionarClicked: (value: string) => void
};

const FilesPart2: React.FC<FilesPart2Props> = ({ elems, onRegresarClicked, onSeleccionarClicked }) => {
    return <div>
        <div className="flex space-x-3">
            <button className="px-4 py-2 rounded-lg bg-red-500 text-white 
                hover:bg-red-600 transition-colors duration-200 
                shadow-sm flex items-center gap-2" onClick={() => onRegresarClicked}>Regresar</button>
            <button className="px-4 py-2 rounded-lg bg-red-500 text-white 
                hover:bg-red-600 transition-colors duration-200 
                shadow-sm flex items-center gap-2" onClick={() => onSeleccionarClicked}>Seleccionar</button>
        </div>
        <div className="mt-2">
            <h4>Info:</h4>
            {elems.map((elem) => <div key={elem.field}><b>{elem.field}: </b>{elem.value}</div>)}
        </div>
    </div>
}

export { FilesPart2 }
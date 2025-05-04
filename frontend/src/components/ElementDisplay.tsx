import { faFile, faFolderOpen, faHardDrive } from '@fortawesome/free-regular-svg-icons';
import { faInbox } from '@fortawesome/free-solid-svg-icons';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

type TypeElementDisplay = "disk" | "partition" | "folder" | "file";
export type ElementDisplayProps = {
    name: string;
    type: TypeElementDisplay;
};

export type ElementDisplayProps2 = {
    name: string;
    type: TypeElementDisplay;
    isActive: boolean;
};

const ElementDisplay: React.FC<ElementDisplayProps2> = ({ name, type, isActive }) => {
    return <div className={"p-4 cursor-pointer hover:opacity-50 " + (isActive ? "border border-gray-700 rounded" : "")}>
        <div className='text-center'>
            {type == "disk" ? <FontAwesomeIcon icon={faHardDrive} className='text-2xl' /> : 
                ( type == "file" ? <FontAwesomeIcon icon={faFile} className='text-2xl' /> : 
                    type == "folder" ? <FontAwesomeIcon icon={faFolderOpen} className='text-2xl' /> : <FontAwesomeIcon icon={faInbox} className='text-2xl' />
                )
                }
        </div>
        <div className='mt-1'>
            <p>{ name }</p>
        </div>
    </div>
}

export { ElementDisplay };
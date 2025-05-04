package structures

import (
	"encoding/binary"
	"os"
)

// PointerBlock representa un bloque de apuntadores en el sistema de archivos
type PointerBlock struct {
	P_pointers [16]int32 // 16 * 4 = 64 bytes
	// Total: 64 bytes
}

// Serialize escribe el PointerBlock en el disco en la posición especificada
func (pb *PointerBlock) Serialize(path string, position int64) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	// Posicionarse en el lugar correcto del archivo
	_, err = file.Seek(position, 0)
	if err != nil {
		return err
	}

	// Serializar los 16 apuntadores (P_pointers)
	for _, ptr := range pb.P_pointers {
		err = binary.Write(file, binary.LittleEndian, ptr)
		if err != nil {
			return err
		}
	}

	return nil
}

// Deserialize lee el PointerBlock desde el disco en la posición especificada
func (pb *PointerBlock) Deserialize(path string, position int64) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	// Posicionarse en el lugar correcto del archivo
	_, err = file.Seek(position, 0)
	if err != nil {
		return err
	}

	// Leer los 16 apuntadores (P_pointers)
	for i := 0; i < 16; i++ {
		var ptr int32
		err = binary.Read(file, binary.LittleEndian, &ptr)
		if err != nil {
			return err
		}
		pb.P_pointers[i] = ptr
	}

	return nil
}

package structures

import (
	"bytes"           // Paquete para manipulación de buffers
	"encoding/binary" // Paquete para codificación y decodificación de datos binarios
	"fmt"             // Paquete para formateo de E/S
	"os"              // Paquete para funciones del sistema operativo
	"strings"         // Paquete para manipulación de cadenas
)

// EBR representa el Extended Boot Record para particiones lógicas
type EBR struct {
	Part_mount [1]byte  // Indica si la partición está montada ('0' = no, '1' = sí)
	Part_fit   [1]byte  // Tipo de ajuste ('B' = Best, 'F' = First, 'W' = Worst)
	Part_start int32    // Byte de inicio de la partición lógica
	Part_s     int32    // Tamaño total de la partición en bytes
	Part_next  int32    // Byte donde está el próximo EBR (-1 si no hay siguiente)
	Part_name  [16]byte // Nombre de la partición
}

// Serialize escribe la estructura EBR en un archivo binario en la posición especificada
func (ebr *EBR) Serialize(path string, offset int64) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Mover el puntero del archivo a la posición especificada
	_, err = file.Seek(offset, 0)
	if err != nil {
		return fmt.Errorf("error al mover el puntero del archivo: %v", err)
	}

	// Serializar la estructura EBR directamente en el archivo
	err = binary.Write(file, binary.LittleEndian, ebr)
	if err != nil {
		return fmt.Errorf("error serializando el EBR: %v", err)
	}

	return nil
}

// Deserialize lee la estructura EBR desde un archivo binario en la posición especificada
func (ebr *EBR) Deserialize(path string, offset int64) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Mover el puntero del archivo a la posición especificada
	_, err = file.Seek(offset, 0)
	if err != nil {
		return fmt.Errorf("error al mover el puntero del archivo: %v", err)
	}

	// Obtener el tamaño de la estructura EBR
	ebrSize := binary.Size(ebr)
	if ebrSize <= 0 {
		return fmt.Errorf("tamaño inválido del EBR: %d", ebrSize)
	}

	// Leer solo la cantidad de bytes que corresponden al tamaño de la estructura EBR
	buffer := make([]byte, ebrSize)
	_, err = file.Read(buffer)
	if err != nil {
		return fmt.Errorf("error leyendo el EBR: %v", err)
	}

	// Deserializar los bytes leídos en la estructura EBR
	reader := bytes.NewReader(buffer)
	err = binary.Read(reader, binary.LittleEndian, ebr)
	if err != nil {
		return fmt.Errorf("error deserializando el EBR: %v", err)
	}

	return nil
}

// PrintEBR imprime los valores del EBR
func (ebr *EBR) PrintEBR() {
	// Convertir campos a valores legibles
	mountStatus := rune(ebr.Part_mount[0])
	fitType := rune(ebr.Part_fit[0])
	name := strings.Trim(string(ebr.Part_name[:]), "\x00")

	fmt.Printf("EBR:\n")
	fmt.Printf("  Part_mount: %c\n", mountStatus)
	fmt.Printf("  Part_fit: %c\n", fitType)
	fmt.Printf("  Part_start: %d\n", ebr.Part_start)
	fmt.Printf("  Part_s: %d\n", ebr.Part_s)
	fmt.Printf("  Part_next: %d\n", ebr.Part_next)
	fmt.Printf("  Part_name: %s\n", name)
}

package reports

import (
	structures "backend/structures"
	utils "backend/utils"
	"fmt"
	"os"
	"strings"
)

// ReportBMInode genera un reporte del bitmap de inodos y lo guarda en la ruta especificada
// ReportBMInode genera un reporte del bitmap de inodos y lo guarda en la ruta especificada
func ReportBMInode(superblock *structures.SuperBlock, diskPath string, path string) error {
	fmt.Println("DEBUG: Iniciando ReportBMInode - DiskPath:", diskPath, "OutputPath:", path)
	fmt.Println("DEBUG: SuperBlock - S_bm_inode_start:", superblock.S_bm_inode_start)
	fmt.Println("DEBUG: S_inodes_count:", superblock.S_inodes_count, "S_free_inodes_count:", superblock.S_free_inodes_count)

	// Crear las carpetas padre si no existen
	err := utils.CreateParentDirs(path)
	if err != nil {
		return fmt.Errorf("error al crear directorios padre: %v", err)
	}

	// Abrir el archivo de disco
	file, err := os.Open(diskPath)
	if err != nil {
		return fmt.Errorf("error al abrir el archivo de disco: %v", err)
	}
	defer file.Close()

	// Calcular el número total de inodos
	totalInodes := superblock.S_inodes_count + superblock.S_free_inodes_count
	fmt.Println("DEBUG: Total de inodos a leer:", totalInodes)

	// Obtener el contenido del bitmap de inodos
	var bitmapContent strings.Builder
	bitsWritten := int32(0)

	for i := int32(0); i < totalInodes; i++ {
		// Establecer el puntero
		position := int64(superblock.S_bm_inode_start + i)
		_, err := file.Seek(position, 0)
		if err != nil {
			return fmt.Errorf("error al establecer el puntero en posición %d: %v", position, err)
		}

		// Leer un byte (carácter '0' o '1')
		char := make([]byte, 1)
		_, err = file.Read(char)
		if err != nil {
			return fmt.Errorf("error al leer el byte en posición %d: %v", position, err)
		}

		// Debug: Mostrar los primeros 20 bytes y algunos más adelante
		if i < 20 || (i >= totalInodes-20 && i < totalInodes) {
			fmt.Printf("DEBUG: Posición %d (abs: %d) - Byte leído: %c (ASCII: %d)\n", i, position, char[0], char[0])
		}

		// Si el byte es \x00, interpretarlo como '0' (libre)
		if char[0] == 0 {
			bitmapContent.WriteByte('0')
		} else {
			bitmapContent.WriteByte(char[0])
		}
		bitsWritten++

		// Agregar un carácter de nueva línea cada 20 caracteres (20 inodos)
		if (bitsWritten)%20 == 0 {
			bitmapContent.WriteString("\n")
		}
	}

	// Asegurarse de que el contenido termine con un salto de línea
	content := bitmapContent.String()
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	fmt.Println("DEBUG: Contenido del bitmap (primeras 5 líneas):")
	lines := strings.Split(content, "\n")
	for i := 0; i < 5 && i < len(lines); i++ {
		fmt.Println("DEBUG: Línea", i+1, ":", lines[i])
	}
	fmt.Println("DEBUG: Total de líneas generadas:", len(lines))

	// Crear el archivo TXT (usar os.Create para truncar el archivo)
	txtFile, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("error al crear el archivo TXT: %v", err)
	}
	defer txtFile.Close()

	// Escribir el contenido del bitmap en el archivo TXT
	_, err = txtFile.WriteString(content)
	if err != nil {
		return fmt.Errorf("error al escribir en el archivo TXT: %v", err)
	}

	// Asegurarse de que los datos se escriban en el disco
	err = txtFile.Sync()
	if err != nil {
		return fmt.Errorf("error al sincronizar el archivo TXT: %v", err)
	}

	fmt.Println("DEBUG: Archivo del bitmap de inodos generado:", path)
	return nil
}

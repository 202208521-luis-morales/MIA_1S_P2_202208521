package reports

import (
	structures "backend/structures"
	utils "backend/utils"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ReportMBR genera un reporte del MBR y lo guarda en la ruta especificada
func ReportMBR(mbr *structures.MBR, diskPath string) error {
	// Crear las carpetas padre si no existen
	err := utils.CreateParentDirs(diskPath)
	if err != nil {
		return err
	}

	// Obtener el nombre base del archivo sin la extensión
	dotFileName, outputImage := utils.GetFileNames(diskPath)

	// Convertir Mbr_creation_date (float32) a time.Time
	creationDate := time.Unix(int64(mbr.Mbr_creation_date), 0)

	// Definir el contenido DOT con una tabla
	dotContent := fmt.Sprintf(`digraph G {
        node [shape=plaintext]
        tabla [label=<
            <table border="0" cellborder="1" cellspacing="0">
                <tr><td colspan="2"> REPORTE DE MBR </td></tr>
                <tr><td>mbr_tamano</td><td>%d</td></tr>
                <tr><td>mrb_fecha_creacion</td><td>%s</td></tr>
                <tr><td>mbr_disk_signature</td><td>%d</td></tr>
            `, mbr.Mbr_size, creationDate.Format("2006-01-02 15:04"), mbr.Mbr_disk_signature)

	// Agregar las particiones primarias/extendidas a la tabla
	for i, part := range mbr.Mbr_partitions {
		// Convertir Part_name a string y eliminar los caracteres nulos
		partName := strings.TrimRight(string(part.Part_name[:]), "\x00")

		// Convertir Part_status, Part_type y Part_fit a char, manejando el caso de \x00
		partStatus := "-"
		if part.Part_status[0] != 0 {
			partStatus = string(part.Part_status[0])
		}
		partType := "-"
		if part.Part_type[0] != 0 {
			partType = string(part.Part_type[0])
		}
		partFit := "-"
		if part.Part_fit[0] != 0 {
			partFit = string(part.Part_fit[0])
		}

		// Si la partición no está activa (part_size == 0), mostrar "-"
		partStart := "-"
		if part.Part_start != 0 {
			partStart = fmt.Sprintf("%d", part.Part_start)
		}
		partSize := "-"
		if part.Part_size != 0 {
			partSize = fmt.Sprintf("%d", part.Part_size)
		}
		if partName == "" {
			partName = "-"
		}

		// Agregar la partición a la tabla
		dotContent += fmt.Sprintf(`
                <tr><td colspan="2"> PARTICIÓN %d </td></tr>
                <tr><td>part_status</td><td>%s</td></tr>
                <tr><td>part_type</td><td>%s</td></tr>
                <tr><td>part_fit</td><td>%s</td></tr>
                <tr><td>part_start</td><td>%s</td></tr>
                <tr><td>part_size</td><td>%s</td></tr>
                <tr><td>part_name</td><td>%s</td></tr>
            `, i+1, partStatus, partType, partFit, partStart, partSize, partName)
	}

	// Buscar particiones lógicas dentro de la partición extendida
	logicalIndex := 1
	for _, part := range mbr.Mbr_partitions {
		if (part.Part_type[0] == 'e' || part.Part_type[0] == 'E') && part.Part_start != 0 {
			// Abrir el archivo del disco para leer los EBR
			file, err := os.Open(diskPath)
			if err != nil {
				return fmt.Errorf("error al abrir el disco: %v", err)
			}
			defer file.Close()

			// Leer los EBR de la partición extendida
			currentPos := int64(part.Part_start)
			for {
				// Leer el EBR en la posición actual
				ebr := &structures.EBR{}
				err := ebr.Deserialize(diskPath, currentPos)
				if err != nil {
					return fmt.Errorf("error al leer EBR: %v", err)
				}

				// Si el EBR no tiene una partición lógica (Part_s == 0), salir del bucle
				if ebr.Part_s == 0 {
					break
				}

				// Convertir los campos del EBR a string
				ebrStatus := "-"
				if ebr.Part_mount[0] != 0 {
					ebrStatus = string(ebr.Part_mount[0])
				}
				ebrFit := "-"
				if ebr.Part_fit[0] != 0 {
					ebrFit = string(ebr.Part_fit[0])
				}
				ebrName := strings.TrimRight(string(ebr.Part_name[:]), "\x00")
				ebrNext := fmt.Sprintf("%d", ebr.Part_next)
				if ebr.Part_next == -1 {
					ebrNext = "-1"
				}

				// Agregar la partición lógica a la tabla
				dotContent += fmt.Sprintf(`
                        <tr><td colspan="2"> PARTICIÓN LÓGICA %d </td></tr>
                        <tr><td>part_status</td><td>%s</td></tr>
                        <tr><td>part_next</td><td>%s</td></tr>
                        <tr><td>part_fit</td><td>%s</td></tr>
                        <tr><td>part_start</td><td>%d</td></tr>
                        <tr><td>part_size</td><td>%d</td></tr>
                        <tr><td>part_name</td><td>%s</td></tr>
                    `, logicalIndex, ebrStatus, ebrNext, ebrFit, ebr.Part_start, ebr.Part_s, ebrName)

				logicalIndex++

				// Si no hay más particiones lógicas (part_next == -1), salir del bucle
				if ebr.Part_next == -1 {
					break
				}

				// Moverse a la siguiente partición lógica
				currentPos = int64(ebr.Part_next)
			}
		}
	}

	// Cerrar la tabla y el contenido DOT
	dotContent += "</table>>] }"

	// Guardar el contenido DOT en un archivo
	file, err := os.Create(dotFileName)
	if err != nil {
		return fmt.Errorf("error al crear el archivo: %v", err)
	}
	defer file.Close()

	_, err = file.WriteString(dotContent)
	if err != nil {
		return fmt.Errorf("error al escribir en el archivo: %v", err)
	}

	// Ejecutar el comando Graphviz para generar la imagen
	cmd := exec.Command("dot", "-Tpng", dotFileName, "-o", outputImage)
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("error al ejecutar el comando Graphviz: %v", err)
	}

	fmt.Println("Imagen de la tabla generada:", outputImage)
	return nil
}

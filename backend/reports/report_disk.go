package reports

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"backend/structures"
)

func ReportDisk(mbr *structures.MBR, diskPath string, outputPath string) error {
	fmt.Println("DEBUG: Iniciando ReportDisk - DiskPath:", diskPath, "OutputPath:", outputPath)

	// Abrir el disco para leer los EBRs
	file, err := os.OpenFile(diskPath, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("error al abrir disco: %v", err)
	}
	defer file.Close()

	// Usar el MBR proporcionado
	totalDiskSize := mbr.Mbr_size
	if totalDiskSize <= 0 {
		return fmt.Errorf("tamaño del disco inválido: %d", totalDiskSize)
	}
	fmt.Println("DEBUG: MBR - Tamaño del disco:", totalDiskSize)

	// Calcular el tamaño del MBR
	mbrSize := int32(binary.Size(mbr))
	fmt.Println("DEBUG: MBR - Tamaño:", mbrSize, "Porcentaje:", (float64(mbrSize)/float64(totalDiskSize))*100)

	// Lista para almacenar las secciones del reporte (MBR, particiones, espacios libres)
	type Section struct {
		Name        string
		Size        int32
		Percentage  float64
		IsExtended  bool
		SubSections []Section // Para particiones lógicas dentro de una extendida
	}
	var sections []Section

	// Agregar el MBR como primera sección
	mbrPercentage := (float64(mbrSize) / float64(totalDiskSize)) * 100
	sections = append(sections, Section{
		Name:       "MBR",
		Size:       mbrSize,
		Percentage: mbrPercentage,
	})

	// Procesar las particiones primarias y extendidas
	currentPos := mbrSize // Posición actual después del MBR
	for i := 0; i < 4; i++ {
		part := mbr.Mbr_partitions[i]
		if part.Part_status[0] == 'N' || part.Part_size == 0 {
			continue // Partición inactiva o vacía
		}

		fmt.Println("DEBUG: Partición", i, "- Start:", part.Part_start, "Size:", part.Part_size, "Type:", string(part.Part_type[:]), "Name:", strings.Trim(string(part.Part_name[:]), "\x00"))

		// Calcular espacio libre antes de la partición
		if currentPos < part.Part_start {
			freeSize := part.Part_start - currentPos
			freePercentage := (float64(freeSize) / float64(totalDiskSize)) * 100
			sections = append(sections, Section{
				Name:       "Libre",
				Size:       freeSize,
				Percentage: freePercentage,
			})
			fmt.Println("DEBUG: Espacio libre antes de partición", i, "- Tamaño:", freeSize, "Porcentaje:", freePercentage)
		}

		// Procesar la partición
		partSize := part.Part_size
		partPercentage := (float64(partSize) / float64(totalDiskSize)) * 100
		partName := strings.Trim(string(part.Part_name[:]), "\x00")

		if part.Part_type[0] == 'E' {
			// Partición extendida: procesar las particiones lógicas
			var subSections []Section
			currentLogicalPos := part.Part_start
			nextEBRPos := part.Part_start

			for nextEBRPos != -1 {
				// Leer el EBR
				ebr := &structures.EBR{}
				err = ebr.Deserialize(diskPath, int64(nextEBRPos))
				if err != nil {
					return fmt.Errorf("error al leer EBR: %v", err)
				}

				fmt.Println("DEBUG: EBR - Start:", ebr.Part_start, "Size:", ebr.Part_s, "Mount:", string(ebr.Part_mount[:]), "Next:", ebr.Part_next, "Name:", strings.Trim(string(ebr.Part_name[:]), "\x00"))

				// Calcular espacio libre antes del EBR o la partición lógica
				ebrSize := int32(binary.Size(ebr))
				if currentLogicalPos < ebr.Part_start {
					freeSize := ebr.Part_start - currentLogicalPos
					freePercentage := (float64(freeSize) / float64(totalDiskSize)) * 100
					subSections = append(subSections, Section{
						Name:       "Libre",
						Size:       freeSize,
						Percentage: freePercentage,
					})
					fmt.Println("DEBUG: Espacio libre antes de EBR - Tamaño:", freeSize, "Porcentaje:", freePercentage)
				}

				// Agregar el EBR
				ebrPercentage := (float64(ebrSize) / float64(totalDiskSize)) * 100
				subSections = append(subSections, Section{
					Name:       "EBR",
					Size:       ebrSize,
					Percentage: ebrPercentage,
				})
				fmt.Println("DEBUG: EBR - Tamaño:", ebrSize, "Porcentaje:", ebrPercentage)

				// Agregar la partición lógica si existe (solo verificamos Part_s)
				if ebr.Part_s > 0 {
					logicalSize := ebr.Part_s
					logicalPercentage := (float64(logicalSize) / float64(totalDiskSize)) * 100
					logicalName := strings.Trim(string(ebr.Part_name[:]), "\x00")
					subSections = append(subSections, Section{
						Name:       "Lógica (" + logicalName + ")",
						Size:       logicalSize,
						Percentage: logicalPercentage,
					})
					fmt.Println("DEBUG: Partición lógica - Nombre:", logicalName, "Tamaño:", logicalSize, "Porcentaje:", logicalPercentage)
					currentLogicalPos = ebr.Part_start + ebrSize + ebr.Part_s // Corrección aquí
				} else {
					currentLogicalPos = ebr.Part_start + ebrSize
				}

				nextEBRPos = ebr.Part_next
			}

			// Calcular espacio libre al final de la partición extendida
			endOfExtended := part.Part_start + part.Part_size
			if currentLogicalPos < endOfExtended {
				freeSize := endOfExtended - currentLogicalPos
				freePercentage := (float64(freeSize) / float64(totalDiskSize)) * 100
				subSections = append(subSections, Section{
					Name:       "Libre",
					Size:       freeSize,
					Percentage: freePercentage,
				})
				fmt.Println("DEBUG: Espacio libre al final de extendida - Tamaño:", freeSize, "Porcentaje:", freePercentage)
			}

			// Agregar la partición extendida con sus subsecciones
			sections = append(sections, Section{
				Name:        "Extendida",
				Size:        partSize,
				Percentage:  partPercentage,
				IsExtended:  true,
				SubSections: subSections,
			})
		} else {
			// Partición primaria
			sections = append(sections, Section{
				Name:       "Primaria (" + partName + ")",
				Size:       partSize,
				Percentage: partPercentage,
			})
			fmt.Println("DEBUG: Partición primaria - Nombre:", partName, "Tamaño:", partSize, "Porcentaje:", partPercentage)
		}

		currentPos = part.Part_start + part.Part_size
	}

	// Calcular espacio libre al final del disco
	if currentPos < totalDiskSize {
		freeSize := totalDiskSize - currentPos
		freePercentage := (float64(freeSize) / float64(totalDiskSize)) * 100
		sections = append(sections, Section{
			Name:       "Libre",
			Size:       freeSize,
			Percentage: freePercentage,
		})
		fmt.Println("DEBUG: Espacio libre al final del disco - Tamaño:", freeSize, "Porcentaje:", freePercentage)
	}

	// Verificar que la sumatoria de los porcentajes sea 100%
	totalPercentage := 0.0
	for _, section := range sections {
		if section.IsExtended {
			for _, subSection := range section.SubSections {
				totalPercentage += subSection.Percentage
			}
		} else {
			totalPercentage += section.Percentage
		}
	}
	fmt.Println("DEBUG: Sumatoria de porcentajes:", totalPercentage)
	if totalPercentage < 99.9 || totalPercentage > 100.1 {
		return fmt.Errorf("la sumatoria de los porcentajes no es 100%%: %.2f%%", totalPercentage)
	}

	// Ajustar los porcentajes para que sumen exactamente 100%
	if totalPercentage != 100.0 {
		adjustment := (100.0 - totalPercentage) / float64(len(sections))
		for i := range sections {
			if sections[i].IsExtended {
				subAdjustment := adjustment / float64(len(sections[i].SubSections))
				for j := range sections[i].SubSections {
					sections[i].SubSections[j].Percentage += subAdjustment
				}
			} else {
				sections[i].Percentage += adjustment
			}
		}
	}

	// Generar el archivo .dot para Graphviz
	dotFilePath := outputPath + ".dot"
	dotFile, err := os.Create(dotFilePath)
	if err != nil {
		return fmt.Errorf("error al crear archivo .dot: %v", err)
	}
	defer dotFile.Close()

	// Escribir el contenido del archivo .dot
	dotContent := "digraph DiskReport {\n"
	dotContent += "    rankdir=LR;\n" // Orientación horizontal
	dotContent += "    node [shape=plaintext];\n"
	dotContent += "    subgraph cluster_disk {\n"
	dotContent += "        label=\"Reporte de Disco\";\n"
	dotContent += "        disk [label=<<TABLE BORDER=\"0\" CELLBORDER=\"1\" CELLSPACING=\"0\">\n"
	dotContent += "            <TR><TD COLSPAN=\"100\">Disco: " + diskPath + "</TD></TR>\n"
	dotContent += "            <TR>\n"

	// Agregar las secciones a la tabla
	for _, section := range sections {
		if section.IsExtended {
			// Para la partición extendida, mostramos una subtabla
			dotContent += "                <TD><TABLE BORDER=\"0\" CELLBORDER=\"1\" CELLSPACING=\"0\">\n"
			dotContent += "                    <TR><TD COLSPAN=\"" + fmt.Sprintf("%d", len(section.SubSections)) + "\">Extendida (" + fmt.Sprintf("%.2f%%", section.Percentage) + ")</TD></TR>\n"
			dotContent += "                    <TR>\n"
			for _, subSection := range section.SubSections {
				color := "white"
				if strings.HasPrefix(subSection.Name, "EBR") {
					color = "lightgrey"
				} else if strings.HasPrefix(subSection.Name, "Lógica") {
					color = "lightgreen"
				}
				dotContent += "                        <TD BGCOLOR=\"" + color + "\">" + subSection.Name + "<BR/>" + fmt.Sprintf("%.2f%%", subSection.Percentage) + "</TD>\n"
			}
			dotContent += "                    </TR>\n"
			dotContent += "                </TABLE></TD>\n"
		} else {
			// Secciones normales (MBR, primaria, libre)
			color := "white"
			if section.Name == "MBR" {
				color = "orange"
			} else if strings.HasPrefix(section.Name, "Primaria") {
				color = "lightblue"
			}
			dotContent += "                <TD BGCOLOR=\"" + color + "\">" + section.Name + "<BR/>" + fmt.Sprintf("%.2f%%", section.Percentage) + "</TD>\n"
		}
	}

	dotContent += "            </TR>\n"
	dotContent += "        </TABLE>>];\n"
	dotContent += "    }\n"
	dotContent += "}\n"

	// Escribir el contenido en el archivo .dot
	_, err = dotFile.WriteString(dotContent)
	if err != nil {
		return fmt.Errorf("error al escribir archivo .dot: %v", err)
	}

	// Generar el PDF usando Graphviz
	cmd := exec.Command("dot", "-Tpdf", dotFilePath, "-o", outputPath)
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("error al generar PDF con Graphviz: %v (asegúrate de que Graphviz esté instalado)", err)
	}

	// Opcional: Eliminar el archivo .dot después de generar el PDF
	err = os.Remove(dotFilePath)
	if err != nil {
		fmt.Println("Advertencia: no se pudo eliminar el archivo .dot:", err)
	}

	fmt.Println("DEBUG: Reporte DISK generado exitosamente en", outputPath)
	return nil
}

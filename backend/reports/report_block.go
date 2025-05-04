package reports

import (
	"backend/structures"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func ReportBlock(diskPath string, partitionID string, sb *structures.SuperBlock, outputPath string) error {
	fmt.Println("\nDEBUG: Iniciando ReportBlock - DiskPath:", diskPath, "OutputPath:", outputPath)

	// Abrir el disco
	file, err := os.OpenFile(diskPath, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("error al abrir disco: %v", err)
	}
	defer file.Close()

	// Leer el bitmap de bloques
	bmBlock := make([]byte, sb.S_blocks_count)
	_, err = file.Seek(int64(sb.S_bm_block_start), 0)
	if err != nil {
		return fmt.Errorf("error al posicionarse en el bitmap de bloques: %v", err)
	}
	_, err = file.Read(bmBlock)
	if err != nil {
		return fmt.Errorf("error al leer bitmap de bloques: %v", err)
	}

	// Depuración: Imprimir el bitmap de bloques
	fmt.Println("DEBUG: Bitmap de bloques (primeros 10 bytes):", bmBlock[:min(10, len(bmBlock))])

	// Mapa para rastrear qué inodos apuntan a cada bloque
	blockToInodes := make(map[int][]int32) // bloque -> lista de inodos que lo apunten
	for inodeIndex := int32(0); inodeIndex < sb.S_inodes_count; inodeIndex++ {
		inode := &structures.Inode{}
		err = inode.Deserialize(diskPath, int64(sb.S_inode_start+inodeIndex*sb.S_inode_size))
		if err != nil {
			fmt.Println("DEBUG: Error al leer inodo", inodeIndex, ":", err)
			continue
		}
		// Consideramos inodos que tengan un tipo definido ('0' o '1')
		if inode.I_type[0] != '0' && inode.I_type[0] != '1' {
			continue
		}
		for _, blockPos := range inode.I_block {
			if blockPos != -1 {
				// Convertir posición absoluta a índice relativo
				blockIndex := (blockPos - sb.S_block_start) / sb.S_block_size
				blockToInodes[int(blockIndex)] = append(blockToInodes[int(blockIndex)], inodeIndex)
				fmt.Println("DEBUG: Inodo", inodeIndex, "apunta al bloque", blockIndex, "(posición absoluta:", blockPos, ")")
			}
		}
	}

	// Lista para almacenar los bloques ocupados
	type BlockInfo struct {
		Index       int
		Type        string // "carpeta", "archivo", "apuntadores"
		Content     string // Contenido del bloque (para mostrar en el reporte)
		Connections []int  // Índices de bloques a los que apunta
	}
	var blocks []BlockInfo

	// Procesar cada bloque ocupado
	blockSize := int(sb.S_block_size)
	for i := 0; i < int(sb.S_blocks_count); i++ {
		if bmBlock[i] != '1' {
			fmt.Println("DEBUG: Bloque", i, "no está ocupado (valor en bitmap:", string(bmBlock[i]), ")")
			continue // Bloque no utilizado
		}
		fmt.Println("DEBUG: Bloque", i, "está ocupado (valor en bitmap: 1)")

		// Determinar el tipo del bloque
		blockInfo := BlockInfo{Index: i}
		inodes, exists := blockToInodes[i]
		if !exists || len(inodes) == 0 {
			fmt.Println("DEBUG: Bloque", i, "está marcado como ocupado pero no tiene inodos que lo apunten")
			blockInfo.Type = "desconocido"
			blockInfo.Content = "Bloque huérfano"
			blocks = append(blocks, blockInfo)
			continue
		}

		// Usamos el primer inodo que apunta a este bloque para determinar su tipo
		inodeIndex := inodes[0]
		inode := &structures.Inode{}
		err = inode.Deserialize(diskPath, int64(sb.S_inode_start+inodeIndex*sb.S_inode_size))
		if err != nil {
			fmt.Println("DEBUG: Error al leer inodo", inodeIndex, "para el bloque", i, ":", err)
			blockInfo.Type = "desconocido"
			blockInfo.Content = "Error al leer inodo"
			blocks = append(blocks, blockInfo)
			continue
		}

		// Calcular la posición del bloque
		blockPos := int64(sb.S_block_start) + int64(i*blockSize)

		// Determinar el tipo del bloque según el inodo
		if inode.I_type[0] == '0' {
			// Bloque de carpeta
			folderBlock := &structures.FolderBlock{}
			err = folderBlock.Deserialize(diskPath, blockPos)
			if err != nil {
				fmt.Println("DEBUG: Error al deserializar FolderBlock en el bloque", i, ":", err)
				blockInfo.Type = "desconocido"
				blockInfo.Content = "Error al leer bloque de carpeta"
			} else {
				blockInfo.Type = "carpeta"
				content := ""
				fmt.Println("DEBUG: Entradas crudas del FolderBlock", i, ":")
				for j, entry := range folderBlock.B_content {
					name := strings.Trim(string(entry.B_name[:]), "\x00")
					fmt.Printf("DEBUG: Entrada %d - Nombre: %q, Inodo: %d\n", j, name, entry.B_inodo)
					if name == "-" || name == "" {
						continue
					}
					content += fmt.Sprintf("%s %d\\l", name, entry.B_inodo)
				}
				blockInfo.Content = content
				// Nueva lógica para calcular conexiones
				connections := make([]int, 0)
				for _, entry := range folderBlock.B_content {
					inodo := entry.B_inodo
					if inodo == -1 {
						continue
					}
					targetInode := &structures.Inode{}
					err = targetInode.Deserialize(diskPath, int64(sb.S_inode_start+inodo*sb.S_inode_size))
					if err != nil {
						fmt.Println("DEBUG: Error al leer inodo", inodo, ":", err)
						continue
					}
					for _, targetBlockPos := range targetInode.I_block {
						if targetBlockPos != -1 {
							targetBlockIndex := (targetBlockPos - sb.S_block_start) / sb.S_block_size
							// No incluir conexiones a sí mismo
							if int(targetBlockIndex) != i {
								// Solo agregar la conexión si el bloque destino está ocupado
								if targetBlockIndex >= 0 && int(targetBlockIndex) < len(bmBlock) && bmBlock[targetBlockIndex] == '1' {
									// Evitar duplicados en las conexiones
									alreadyConnected := false
									for _, conn := range connections {
										if conn == int(targetBlockIndex) {
											alreadyConnected = true
											break
										}
									}
									if !alreadyConnected {
										connections = append(connections, int(targetBlockIndex))
									}
								}
							}
						}
					}
				}
				blockInfo.Connections = connections
			}
		} else if inode.I_type[0] == '1' {
			// Bloque de archivo
			fileBlock := &structures.FileBlock{}
			err = fileBlock.Deserialize(diskPath, blockPos)
			if err != nil {
				fmt.Println("DEBUG: Error al deserializar FileBlock en el bloque", i, ":", err)
				blockInfo.Type = "desconocido"
				blockInfo.Content = "Error al leer bloque de archivo"
			} else {
				blockInfo.Type = "archivo"
				content := strings.Trim(string(fileBlock.B_content[:]), "\x00")
				if content == "" {
					content = "Contenido vacío"
				}
				blockInfo.Content = content
				// No hay conexiones directas desde un bloque de archivo
			}
		} else {
			// Tipo desconocido (podría ser un bloque de apuntadores)
			isPointerBlock := false
			for _, blockPos := range inode.I_block {
				blockIndex := (blockPos - sb.S_block_start) / sb.S_block_size
				if blockIndex == int32(i) {
					// Si este bloque está en un apuntador indirecto (por ejemplo, I_block[12], I_block[13], I_block[14])
					for j := 12; j < 15; j++ {
						targetBlockIndex := (inode.I_block[j] - sb.S_block_start) / sb.S_block_size
						if targetBlockIndex == int32(i) {
							isPointerBlock = true
							break
						}
					}
					break
				}
			}
			if isPointerBlock {
				pointerBlock := &structures.PointerBlock{}
				err = pointerBlock.Deserialize(diskPath, blockPos)
				if err != nil {
					fmt.Println("DEBUG: Error al deserializar PointerBlock en el bloque", i, ":", err)
					blockInfo.Type = "desconocido"
					blockInfo.Content = "Error al leer bloque de apuntadores"
				} else {
					blockInfo.Type = "apuntadores"
					indices := make([]int, len(pointerBlock.P_pointers))
					for j, ptr := range pointerBlock.P_pointers {
						indices[j] = int(ptr)
					}
					content := strings.Join(strings.Fields(strings.Trim(strings.Replace(fmt.Sprint(indices), " ", ", ", -1), "[]")), ", ")
					blockInfo.Content = content
					for _, idx := range indices {
						if idx != -1 {
							blockInfo.Connections = append(blockInfo.Connections, idx)
						}
					}
				}
			} else {
				blockInfo.Type = "desconocido"
				blockInfo.Content = "Tipo de inodo desconocido"
			}
		}

		blocks = append(blocks, blockInfo)
		fmt.Println("DEBUG: Bloque", i, "- Tipo:", blockInfo.Type, "Contenido:", blockInfo.Content, "Conexiones:", blockInfo.Connections)
	}

	// Generar el archivo .dot para Graphviz
	dotFilePath := outputPath + ".dot"
	dotFile, err := os.Create(dotFilePath)
	if err != nil {
		return fmt.Errorf("error al crear archivo .dot: %v", err)
	}
	defer dotFile.Close()

	// Escribir el contenido del archivo .dot
	dotContent := "digraph BlockReport {\n"
	dotContent += "    rankdir=LR;\n"
	dotContent += "    node [shape=record, fontname=\"Arial\"];\n"

	// Agregar los nodos para cada bloque
	for _, block := range blocks {
		label := ""
		switch block.Type {
		case "carpeta":
			label = fmt.Sprintf("Bloque Carpeta %d | %s", block.Index, block.Content)
		case "archivo":
			label = fmt.Sprintf("Bloque Archivo %d | %s", block.Index, block.Content)
		case "apuntadores":
			label = fmt.Sprintf("Bloque Apuntadores %d | %s", block.Index, block.Content)
		default:
			label = fmt.Sprintf("Bloque Desconocido %d | %s", block.Index, block.Content)
		}
		dotContent += fmt.Sprintf("    block%d [label=\"%s\"];\n", block.Index, label)
	}

	// Agregar las conexiones entre bloques
	for _, block := range blocks {
		for _, conn := range block.Connections {
			// Solo agregar la conexión si el bloque destino existe
			exists := false
			for _, b := range blocks {
				if b.Index == conn {
					exists = true
					break
				}
			}
			if exists {
				dotContent += fmt.Sprintf("    block%d -> block%d;\n", block.Index, conn)
			}
		}
	}

	dotContent += "}\n"

	// Escribir el contenido en el archivo .dot
	_, err = dotFile.WriteString(dotContent)
	if err != nil {
		return fmt.Errorf("error al escribir archivo .dot: %v", err)
	}

	// Generar el JPG usando Graphviz
	cmd := exec.Command("dot", "-Tjpg", dotFilePath, "-o", outputPath)
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("error al generar JPG con Graphviz: %v (asegúrate de que Graphviz esté instalado)", err)
	}

	// Opcional: Eliminar el archivo .dot después de generar el JPG
	err = os.Remove(dotFilePath)
	if err != nil {
		fmt.Println("Advertencia: no se pudo eliminar el archivo .dot:", err)
	}

	fmt.Println("DEBUG: Reporte BLOCK generado exitosamente en", outputPath)
	return nil
}

// Función auxiliar para obtener el mínimo
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

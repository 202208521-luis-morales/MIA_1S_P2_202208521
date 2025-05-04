package reports

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	structures "backend/structures"
	utils "backend/utils"
)

// ReportTree genera un reporte en forma de árbol del sistema de archivos EXT2
func ReportTree(superblock *structures.SuperBlock, diskPath string, path string) error {
	fmt.Println("\nDEBUG: Iniciando ReportTree - DiskPath:", diskPath, "OutputPath:", path)

	// Crear las carpetas padre si no existen
	err := utils.CreateParentDirs(path)
	if err != nil {
		return fmt.Errorf("error al crear directorios padre: %v", err)
	}

	// Obtener el nombre base del archivo sin la extensión
	dotFileName, outputImage := utils.GetFileNames(path)

	// Iniciar el contenido DOT con rankdir=TB para disposición vertical
	dotContent := `digraph G {
        rankdir=TB;
        node [shape=plaintext]
    `

	// Mapa para evitar ciclos (inodos ya procesados)
	processedInodes := make(map[int32]bool)

	// Generar el árbol comenzando desde el inodo raíz (inodo 0)
	var nodes, edges strings.Builder
	err = generateTreeNode(&nodes, &edges, diskPath, superblock, 0, "/", processedInodes)
	if err != nil {
		return fmt.Errorf("error al generar el árbol: %v", err)
	}

	// Agregar los nodos y las conexiones al contenido DOT
	dotContent += nodes.String()
	dotContent += edges.String()

	// Cerrar el contenido DOT
	dotContent += "}"

	// Crear el archivo DOT
	dotFile, err := os.Create(dotFileName)
	if err != nil {
		return fmt.Errorf("error al crear el archivo DOT: %v", err)
	}
	defer dotFile.Close()

	// Escribir el contenido DOT en el archivo
	_, err = dotFile.WriteString(dotContent)
	if err != nil {
		return fmt.Errorf("error al escribir el archivo DOT: %v", err)
	}

	// Generar la imagen con Graphviz (usar formato JPG)
	cmd := exec.Command("dot", "-Tjpg", dotFileName, "-o", outputImage)
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("error al generar la imagen con Graphviz: %v", err)
	}

	fmt.Println("DEBUG: Reporte Tree generado exitosamente en", outputImage)
	return nil
}

// generateTreeNode genera recursivamente el árbol para un inodo dado
func generateTreeNode(nodes, edges *strings.Builder, diskPath string, superblock *structures.SuperBlock, inodeIndex int32, inodeName string, processedInodes map[int32]bool) error {
	// Evitar ciclos
	if processedInodes[inodeIndex] {
		return nil
	}
	processedInodes[inodeIndex] = true

	// Cargar el inodo
	inode := &structures.Inode{}
	err := inode.Deserialize(diskPath, int64(superblock.S_inode_start+(inodeIndex*superblock.S_inode_size)))
	if err != nil {
		return fmt.Errorf("error al cargar inodo %d: %v", inodeIndex, err)
	}
	fmt.Println("DEBUG: Inodo", inodeIndex, "cargado - Tipo:", string(inode.I_type[:]), "Permisos:", string(inode.I_perm[:]))

	// Generar el nodo del inodo (fondo azul claro)
	nodes.WriteString(fmt.Sprintf(`inode%d [label=<
        <table border="0" cellborder="1" cellspacing="0" bgcolor="lightblue">
            <tr><td colspan="2">Inodo %d</td></tr>
            <tr><td>Nombre</td><td>%s</td></tr>
            <tr><td>Tipo</td><td>%s</td></tr>
            <tr><td>Permisos</td><td>%s</td></tr>
            <tr><td>Bloques</td><td>%v</td></tr>
        </table>>];
    `, inodeIndex, inodeIndex, inodeName, string(inode.I_type[:]), string(inode.I_perm[:]), getUsedBlocks(inode.I_block)))

	// Procesar los bloques del inodo
	for i, blockIndex := range inode.I_block {
		if blockIndex == -1 {
			continue // Bloque no usado
		}
		fmt.Println("DEBUG: Inodo", inodeIndex, "apunta al bloque", i, "(posición absoluta:", blockIndex, ")")

		if inode.I_type[0] == '0' { // Carpeta
			// Cargar el bloque de carpeta
			folderBlock := &structures.FolderBlock{}
			err := folderBlock.Deserialize(diskPath, int64(blockIndex))
			if err != nil {
				return fmt.Errorf("error al cargar bloque de carpeta %d: %v", blockIndex, err)
			}

			// Generar el nodo del bloque de carpeta (fondo verde claro)
			var entries []string
			for j := 0; j < 4; j++ {
				name := strings.Trim(string(folderBlock.B_content[j].B_name[:]), "\x00")
				if name == "" {
					name = "(vacío)"
				}
				entries = append(entries, fmt.Sprintf("<tr><td>%s</td><td>%d</td></tr>", name, folderBlock.B_content[j].B_inodo))
				fmt.Println("DEBUG: Entrada", j, "- Nombre:", name, "Inodo:", folderBlock.B_content[j].B_inodo)
			}
			nodes.WriteString(fmt.Sprintf(`block%d [label=<
                <table border="0" cellborder="1" cellspacing="0" bgcolor="lightgreen">
                    <tr><td colspan="2">Bloque %d (Carpeta)</td></tr>
                    %s
                </table>>];
            `, blockIndex, blockIndex, strings.Join(entries, "")))

			// Conectar el inodo al bloque
			edges.WriteString(fmt.Sprintf("inode%d -> block%d;\n", inodeIndex, blockIndex))

			// Procesar las entradas del bloque de carpeta
			for j := 0; j < 4; j++ {
				entryInode := folderBlock.B_content[j].B_inodo
				if entryInode == -1 || entryInode == inodeIndex || string(folderBlock.B_content[j].B_name[:]) == "." || string(folderBlock.B_content[j].B_name[:]) == ".." {
					continue // Ignorar entradas vacías, . y ..
				}
				name := strings.Trim(string(folderBlock.B_content[j].B_name[:]), "\x00")
				// Generar recursivamente el subárbol para el inodo referenciado
				err = generateTreeNode(nodes, edges, diskPath, superblock, entryInode, name, processedInodes)
				if err != nil {
					return err
				}
				// Conectar el bloque de carpeta al inodo referenciado
				edges.WriteString(fmt.Sprintf("block%d -> inode%d;\n", blockIndex, entryInode))
			}
		} else if inode.I_type[0] == '1' { // Archivo
			// Cargar el bloque de archivo
			fileBlock := &structures.FileBlock{}
			err := fileBlock.Deserialize(diskPath, int64(blockIndex))
			if err != nil {
				return fmt.Errorf("error al cargar bloque de archivo %d: %v", blockIndex, err)
			}

			// Generar el nodo del bloque de archivo (fondo amarillo claro)
			content := strings.Trim(string(fileBlock.B_content[:]), "\x00")
			nodes.WriteString(fmt.Sprintf(`block%d [label=<
                <table border="0" cellborder="1" cellspacing="0" bgcolor="lightyellow">
                    <tr><td colspan="2">Bloque %d (Archivo)</td></tr>
                    <tr><td>Contenido</td><td>%s</td></tr>
                </table>>];
            `, blockIndex, blockIndex, content))

			// Conectar el inodo al bloque
			edges.WriteString(fmt.Sprintf("inode%d -> block%d;\n", inodeIndex, blockIndex))
		}
	}

	return nil
}

// getUsedBlocks devuelve los índices de bloques usados en un inodo
func getUsedBlocks(blocks [15]int32) []int32 {
	var usedBlocks []int32
	for _, block := range blocks {
		if block != -1 {
			usedBlocks = append(usedBlocks, block)
		}
	}
	return usedBlocks
}

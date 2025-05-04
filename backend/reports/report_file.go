package reports

import (
	stores "backend/stores"
	structures "backend/structures"
	utils "backend/utils"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ReportFile genera un reporte de tipo "file" que muestra el nombre y contenido de un archivo
// ReportFile genera un reporte de tipo "file" que muestra el nombre y contenido de un archivo
func ReportFile(sb *structures.SuperBlock, diskPath, filePath, outputPath string) error {
	fmt.Println("\nDEBUG: Iniciando ReportFile")
	// Verificar que haya una sesión activa
	if stores.ActiveSession == nil {
		return fmt.Errorf("debe iniciar sesión primero para leer archivos")
	}
	fmt.Println("DEBUG: Sesión activa - User:", stores.ActiveSession.Username, "ID:", stores.ActiveSession.PartitionID)

	// Crear las carpetas padre si no existen
	err := utils.CreateParentDirs(outputPath)
	if err != nil {
		return err
	}

	// Normalizar la ruta del archivo
	filePath = filepath.Clean(filePath)
	if !strings.HasPrefix(filePath, "/") {
		return fmt.Errorf("la ruta debe ser absoluta (comenzar con /): %s", filePath)
	}
	fmt.Println("DEBUG: Ruta normalizada:", filePath)

	// Separar el directorio padre y el nombre del archivo
	dir, fileName := filepath.Split(filePath)
	if fileName == "" {
		return fmt.Errorf("el nombre del archivo no puede estar vacío: %s", filePath)
	}
	fmt.Println("DEBUG: Directorio padre:", dir, "Nombre del archivo:", fileName)

	// Buscar el inodo del directorio padre
	parentInode, _, parentIndex, err := findOrCreateParentDir(sb, nil, diskPath, dir, false)
	if err != nil {
		return fmt.Errorf("error al acceder al directorio padre de %s: %v", filePath, err)
	}
	fmt.Println("DEBUG: Directorio padre encontrado - Inodo:", parentIndex)

	// Buscar el inodo del archivo en todos los bloques del inodo padre
	var inodeIndex int32 = -1
	for blockIdx := 0; blockIdx < len(parentInode.I_block); blockIdx++ {
		if parentInode.I_block[blockIdx] == -1 {
			continue // Saltar bloques no asignados
		}
		currentBlock := &structures.FolderBlock{}
		err = currentBlock.Deserialize(diskPath, int64(parentInode.I_block[blockIdx]))
		if err != nil {
			return fmt.Errorf("error al cargar bloque %d del inodo padre: %v", blockIdx, err)
		}
		fmt.Println("DEBUG: Bloque cargado del inodo padre - Posición:", parentInode.I_block[blockIdx])
		for j, content := range currentBlock.B_content {
			name := strings.Trim(string(content.B_name[:]), "\x00")
			fmt.Printf("DEBUG: Revisando entrada %d en bloque %d del inodo padre - Nombre: %q, Inodo: %d\n", j, blockIdx, name, content.B_inodo)
			if name == fileName {
				inodeIndex = content.B_inodo
				break
			}
		}
		if inodeIndex != -1 {
			break // Salir del bucle si encontramos el archivo
		}
	}

	if inodeIndex == -1 {
		return fmt.Errorf("el archivo no existe: %s", filePath)
	}
	fmt.Println("DEBUG: Archivo encontrado - Inodo:", inodeIndex)

	// Cargar el inodo del archivo
	fileInode := &structures.Inode{}
	err = fileInode.Deserialize(diskPath, int64(sb.S_inode_start+inodeIndex*sb.S_inode_size))
	if err != nil {
		return fmt.Errorf("error al cargar inodo de %s: %v", filePath, err)
	}
	fmt.Println("DEBUG: Inodo cargado - Size:", fileInode.I_size, "Type:", string(fileInode.I_type[:]), "Bloque[0]:", fileInode.I_block[0])

	// Verificar permisos de lectura
	if stores.ActiveSession.Username != "root" {
		hasPermission := checkReadPermission(fileInode)
		fmt.Println("DEBUG: Verificando permisos de lectura - UserID:", stores.ActiveSession.UserID, "GroupID:", stores.ActiveSession.GroupID, "Permisos:", string(fileInode.I_perm[:]), "Resultado:", hasPermission)
		if !hasPermission {
			return fmt.Errorf("permiso denegado: no tiene permisos de lectura para %s", filePath)
		}
	} else {
		fmt.Println("DEBUG: Usuario root, permisos de lectura otorgados automáticamente")
	}

	// Leer el contenido del archivo
	content, err := readFileContent(sb, diskPath, fileInode)
	if err != nil {
		return fmt.Errorf("error al leer el contenido de %s: %v", filePath, err)
	}
	fmt.Println("DEBUG: Contenido leído del archivo:", content)

	// Crear el contenido del reporte
	reportContent := fmt.Sprintf("Reporte de ARCHIVO\n#Archivo: %s\n\n%s", filePath, content)

	// Escribir el reporte en el archivo de salida
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("error al crear el archivo de reporte: %v", err)
	}
	defer file.Close()

	_, err = file.WriteString(reportContent)
	if err != nil {
		return fmt.Errorf("error al escribir en el archivo de reporte: %v", err)
	}

	fmt.Println("DEBUG: Reporte de archivo generado:", outputPath)
	return nil
}

// readFileContent lee el contenido de un archivo desde sus bloques
func readFileContent(sb *structures.SuperBlock, path string, inode *structures.Inode) (string, error) {
	fmt.Println("DEBUG: Iniciando readFileContent - Size:", inode.I_size)
	if inode.I_size == 0 {
		fmt.Println("DEBUG: Archivo vacío")
		return "", nil
	}

	var content strings.Builder
	blockSize := int(sb.S_block_size) // 64 bytes
	numBlocks := (int(inode.I_size) + blockSize - 1) / blockSize
	fmt.Println("DEBUG: Bloques necesarios:", numBlocks)

	for i := 0; i < 15 && inode.I_block[i] != -1; i++ {
		fmt.Println("DEBUG: Leyendo bloque", i, "- Posición:", inode.I_block[i])
		fileBlock := &structures.FileBlock{}
		err := fileBlock.Deserialize(path, int64(inode.I_block[i]))
		if err != nil {
			return "", fmt.Errorf("error al cargar bloque %d: %v", i, err)
		}

		remainingSize := int(inode.I_size) - (i * blockSize)
		if remainingSize > blockSize {
			remainingSize = blockSize
		}
		data := strings.TrimRight(string(fileBlock.B_content[:remainingSize]), "\x00")
		fmt.Printf("DEBUG: Bloque %d leído - Contenido: %q, Tamaño: %d\n", i, data, len(data))
		content.WriteString(data)
	}

	finalContent := content.String()
	fmt.Println("DEBUG: Contenido total leído:", finalContent)
	return finalContent, nil
}

// checkReadPermission verifica si el usuario tiene permisos de lectura
func checkReadPermission(inode *structures.Inode) bool {
	fmt.Println("DEBUG: Iniciando checkReadPermission")
	if stores.ActiveSession.Username == "root" {
		fmt.Println("DEBUG: Usuario root, permiso concedido")
		return true
	}

	userPerm := int(inode.I_perm[0] - '0')
	groupPerm := int(inode.I_perm[1] - '0')
	otherPerm := int(inode.I_perm[2] - '0')
	fmt.Println("DEBUG: Permisos - User:", userPerm, "Group:", groupPerm, "Other:", otherPerm)

	if stores.ActiveSession.UserID == inode.I_uid {
		hasPerm := userPerm&4 != 0
		fmt.Println("DEBUG: Permiso de usuario - Lectura:", hasPerm)
		return hasPerm
	}
	if stores.ActiveSession.GroupID == inode.I_gid {
		hasPerm := groupPerm&4 != 0
		fmt.Println("DEBUG: Permiso de grupo - Lectura:", hasPerm)
		return hasPerm
	}
	hasPerm := otherPerm&4 != 0
	fmt.Println("DEBUG: Permiso de otros - Lectura:", hasPerm)
	return hasPerm
}

/*
// findOrCreateParentDir encuentra o crea el directorio padre de una ruta dada

	func findOrCreateParentDir(sb *structures.SuperBlock, partition *structures.Partition, diskPath, dir string, create bool) (*structures.Inode, *structures.FolderBlock, int32, error) {
		fmt.Println("DEBUG: Iniciando findOrCreateParentDir - Dir:", dir, "Create:", create)

		// Normalizar la ruta del directorio
		dir = filepath.Clean(dir)
		if dir == "/" {
			// Si el directorio es la raíz, devolver el inodo raíz (inodo 0)
			rootInode := &structures.Inode{}
			err := rootInode.Deserialize(diskPath, int64(sb.S_inode_start))
			if err != nil {
				return nil, nil, 0, fmt.Errorf("error al cargar inodo raíz: %v", err)
			}
			fmt.Println("DEBUG: Directorio raíz encontrado - Inodo 0")

			// Cargar el bloque de directorio raíz (primer bloque del inodo raíz)
			if rootInode.I_block[0] == -1 {
				return nil, nil, 0, fmt.Errorf("el inodo raíz no tiene bloques asignados")
			}
			folderBlock := &structures.FolderBlock{}
			err = folderBlock.Deserialize(diskPath, int64(rootInode.I_block[0]))
			if err != nil {
				return nil, nil, 0, fmt.Errorf("error al cargar bloque de directorio raíz: %v", err)
			}
			fmt.Println("DEBUG: Bloque de directorio raíz cargado - Bloque:", rootInode.I_block[0])

			return rootInode, folderBlock, 0, nil
		}

		// Dividir la ruta en componentes
		components := strings.Split(strings.Trim(dir, "/"), "/")
		fmt.Println("DEBUG: Componentes de la ruta:", components)

		// Comenzar desde el inodo raíz (inodo 0)
		currentInode := &structures.Inode{}
		err := currentInode.Deserialize(diskPath, int64(sb.S_inode_start))
		if err != nil {
			return nil, nil, 0, fmt.Errorf("error al cargar inodo raíz: %v", err)
		}
		currentInodeIndex := int32(0)
		fmt.Println("DEBUG: Comenzando desde el inodo raíz - Inodo:", currentInodeIndex)

		// Iterar sobre cada componente de la ruta
		for i, component := range components {
			if component == "" {
				continue
			}
			fmt.Println("DEBUG: Procesando componente", i, "- Nombre:", component)

			// Cargar el bloque de directorio actual
			if currentInode.I_block[0] == -1 {
				if !create {
					return nil, nil, 0, fmt.Errorf("el directorio %s no existe", dir)
				}
				// Aquí deberíamos crear un nuevo bloque si create es true, pero para report_file no necesitamos crear directorios
				return nil, nil, 0, fmt.Errorf("el directorio %s no existe y no se permite crear", dir)
			}

			folderBlock := &structures.FolderBlock{}
			err = folderBlock.Deserialize(diskPath, int64(currentInode.I_block[0]))
			if err != nil {
				return nil, nil, 0, fmt.Errorf("error al cargar bloque de directorio: %v", err)
			}
			fmt.Println("DEBUG: Bloque de directorio cargado - Bloque:", currentInode.I_block[0])

			// Buscar el componente en el bloque de directorio
			var nextInodeIndex int32 = -1
			for j, content := range folderBlock.B_content {
				name := strings.Trim(string(content.B_name[:]), "\x00")
				fmt.Printf("DEBUG: Revisando entrada %d - Nombre: %q, Inodo: %d\n", j, name, content.B_inodo)
				if name == component {
					nextInodeIndex = content.B_inodo
					break
				}
			}

			if nextInodeIndex == -1 {
				if !create {
					return nil, nil, 0, fmt.Errorf("el directorio %s no existe", dir)
				}
				// Aquí deberíamos crear el directorio si create es true, pero para report_file no necesitamos crear directorios
				return nil, nil, 0, fmt.Errorf("el directorio %s no existe y no se permite crear", dir)
			}

			// Cargar el siguiente inodo
			currentInode = &structures.Inode{}
			err = currentInode.Deserialize(diskPath, int64(sb.S_inode_start+nextInodeIndex*sb.S_inode_size))
			if err != nil {
				return nil, nil, 0, fmt.Errorf("error al cargar inodo %d: %v", nextInodeIndex, err)
			}
			currentInodeIndex = nextInodeIndex
			fmt.Println("DEBUG: Inodo siguiente cargado - Inodo:", currentInodeIndex)
		}

		// Cargar el bloque de directorio del inodo actual
		if currentInode.I_block[0] == -1 {
			return nil, nil, 0, fmt.Errorf("el inodo %d no tiene bloques asignados", currentInodeIndex)
		}
		folderBlock := &structures.FolderBlock{}
		err = folderBlock.Deserialize(diskPath, int64(currentInode.I_block[0]))
		if err != nil {
			return nil, nil, 0, fmt.Errorf("error al cargar bloque de directorio: %v", err)
		}
		fmt.Println("DEBUG: Bloque de directorio final cargado - Bloque:", currentInode.I_block[0])

		return currentInode, folderBlock, currentInodeIndex, nil
	}
*/
func findOrCreateParentDir(sb *structures.SuperBlock, partition *structures.Partition, path string, dir string, recursive bool) (*structures.Inode, *structures.FolderBlock, int32, error) {
	fmt.Println("DEBUG: Iniciando findOrCreateParentDir - Dir:", dir, "Recursive:", recursive)
	currentInode := &structures.Inode{}
	err := currentInode.Deserialize(path, int64(sb.S_inode_start))
	if err != nil {
		return nil, nil, -1, fmt.Errorf("error al cargar inodo raíz: %v", err)
	}
	fmt.Println("DEBUG: Inodo raíz cargado - Bloque[0]:", currentInode.I_block[0])
	currentIndex := int32(0)

	components := strings.Split(strings.Trim(dir, "/"), "/")
	fmt.Println("DEBUG: Componentes de la ruta:", components)
	if len(components) == 0 || components[0] == "" {
		folderBlock := &structures.FolderBlock{}
		err = folderBlock.Deserialize(path, int64(currentInode.I_block[0]))
		if err != nil {
			return nil, nil, -1, fmt.Errorf("error al cargar bloque raíz: %v", err)
		}
		fmt.Println("DEBUG: Bloque raíz cargado para dir '/'")
		return currentInode, folderBlock, currentIndex, nil
	}

	// Declarar lastFolderBlock fuera del bucle
	var lastFolderBlock *structures.FolderBlock

	for _, component := range components {
		fmt.Println("DEBUG: Buscando componente:", component, "- Inodo actual:", currentIndex)
		var nextInodeIndex int32 = -1

		// Buscar el componente en todos los bloques del inodo actual
		for i := 0; i < len(currentInode.I_block); i++ {
			if currentInode.I_block[i] == -1 {
				continue // Saltar bloques no asignados
			}
			folderBlock := &structures.FolderBlock{}
			err = folderBlock.Deserialize(path, int64(currentInode.I_block[i]))
			if err != nil {
				return nil, nil, -1, fmt.Errorf("error al cargar bloque %d para %s: %v", i, component, err)
			}
			fmt.Println("DEBUG: Bloque cargado para", component, "- Posición:", currentInode.I_block[i])

			// Revisar las entradas del bloque
			for j, content := range folderBlock.B_content {
				name := strings.Trim(string(content.B_name[:]), "\x00")
				fmt.Printf("DEBUG: Revisando entrada %d en bloque %d - Nombre: %q, Inodo: %d\n", j, i, name, content.B_inodo)
				if name == component {
					nextInodeIndex = content.B_inodo
					lastFolderBlock = folderBlock
					break
				}
			}
			if nextInodeIndex != -1 {
				break // Salir del bucle si encontramos el componente
			}
		}

		if nextInodeIndex == -1 {
			if !recursive {
				return nil, nil, -1, fmt.Errorf("la carpeta %s no existe y no se especificó -p (si usas mkdir) ni -r (si usas mkfile)", component)
			}
			newIndex, err := createFolder(sb, path, component, currentIndex)
			if err != nil {
				return nil, nil, -1, err
			}
			currentIndex = newIndex
			currentInode = &structures.Inode{}
			err = currentInode.Deserialize(path, int64(sb.S_inode_start+currentIndex*sb.S_inode_size))
			if err != nil {
				return nil, nil, -1, fmt.Errorf("error al cargar inodo %d: %v", currentIndex, err)
			}
			fmt.Println("DEBUG: Carpeta creada - Nuevo inodo:", currentIndex)
			// Cargar el bloque del nuevo inodo creado
			lastFolderBlock = &structures.FolderBlock{}
			err = lastFolderBlock.Deserialize(path, int64(currentInode.I_block[0]))
			if err != nil {
				return nil, nil, -1, fmt.Errorf("error al cargar bloque del nuevo inodo %d: %v", currentIndex, err)
			}
		} else {
			currentIndex = nextInodeIndex
			currentInode = &structures.Inode{}
			err = currentInode.Deserialize(path, int64(sb.S_inode_start+currentIndex*sb.S_inode_size))
			if err != nil {
				return nil, nil, -1, fmt.Errorf("error al cargar inodo %d: %v", currentIndex, err)
			}
			fmt.Println("DEBUG: Carpeta encontrada - Nuevo inodo:", currentIndex, "Bloque[0]:", currentInode.I_block[0])
		}
	}

	// Usar el último bloque encontrado o creado
	folderBlock := lastFolderBlock
	if folderBlock == nil {
		// Esto no debería ocurrir, pero por seguridad cargamos el primer bloque del inodo actual
		folderBlock = &structures.FolderBlock{}
		err = folderBlock.Deserialize(path, int64(currentInode.I_block[0]))
		if err != nil {
			return nil, nil, -1, fmt.Errorf("error al cargar bloque final: %v", err)
		}
	}
	fmt.Println("DEBUG: Directorio padre encontrado - Inodo:", currentIndex, "Bloque[0]:", currentInode.I_block[0])
	for i, content := range folderBlock.B_content {
		name := strings.Trim(string(content.B_name[:]), "\x00")
		fmt.Printf("DEBUG: Entrada %d en bloque padre - Nombre: %q, Inodo: %d\n", i, name, content.B_inodo)
	}

	return currentInode, folderBlock, currentIndex, nil
}

func createFolder(sb *structures.SuperBlock, path string, folderName string, parentInodeIndex int32) (int32, error) {
	fmt.Println("\nInicia createFolder:")

	// Crear un nuevo inodo para la carpeta
	newInode := &structures.Inode{
		I_uid:   1,
		I_gid:   1,
		I_size:  0,
		I_atime: float32(time.Now().Unix()),
		I_ctime: float32(time.Now().Unix()),
		I_mtime: float32(time.Now().Unix()),
		I_type:  [1]byte{'0'}, // Carpeta
		I_perm:  [3]byte{'6', '6', '4'},
	}
	for i := 0; i < 15; i++ {
		newInode.I_block[i] = -1
	}
	newInode.I_block[0] = sb.S_first_blo

	// Asignar el índice del nuevo inodo
	newInodeIndex := sb.S_first_ino
	fmt.Println("DEBUG: Nuevo inodo creado - Index:", newInodeIndex, "Bloque[0]:", newInode.I_block[0])

	// Crear el bloque de la carpeta
	newBlock := &structures.FolderBlock{}
	copy(newBlock.B_content[0].B_name[:], ".")
	newBlock.B_content[0].B_inodo = newInodeIndex
	copy(newBlock.B_content[1].B_name[:], "..")
	newBlock.B_content[1].B_inodo = parentInodeIndex
	newBlock.B_content[2].B_inodo = -1
	newBlock.B_content[3].B_inodo = -1
	fmt.Println("DEBUG: Nuevo bloque de carpeta - .:", newBlock.B_content[0].B_inodo, "..:", newBlock.B_content[1].B_inodo)

	// Actualizar bitmaps
	err := sb.UpdateBitmapInode(path, newInodeIndex)
	if err != nil {
		return 0, fmt.Errorf("error al actualizar bitmap de inodos: %v", err)
	}
	err = sb.SetBitmapBlock(path, newInode.I_block[0], '1')
	if err != nil {
		return 0, fmt.Errorf("error al actualizar bitmap de bloques: %v", err)
	}
	fmt.Println("DEBUG: Bitmaps actualizados")

	// Actualizar contadores después de usar S_first_ino
	sb.S_inodes_count++
	sb.S_free_inodes_count--
	sb.S_blocks_count++
	sb.S_free_blocks_count--
	sb.S_first_ino++ // Incrementar S_first_ino después de usarlo
	sb.S_first_blo += sb.S_block_size
	fmt.Println("DEBUG: Actualizando contadores - First Ino:", sb.S_first_ino, "Inodes Count:", sb.S_inodes_count, "Free Inodes:", sb.S_free_inodes_count, "First Blo:", sb.S_first_blo, "Free Blocks:", sb.S_free_blocks_count)

	// Serializar el nuevo inodo y bloque
	err = newInode.Serialize(path, int64(sb.S_inode_start+(newInodeIndex*sb.S_inode_size)))
	if err != nil {
		return 0, fmt.Errorf("error al serializar nuevo inodo: %v", err)
	}
	err = newBlock.Serialize(path, int64(newInode.I_block[0]))
	if err != nil {
		return 0, fmt.Errorf("error al serializar nuevo bloque: %v", err)
	}
	fmt.Println("DEBUG: Nuevo inodo y bloque serializados")

	// Verificar las entradas del bloque después de serializar
	for i := 0; i < 4; i++ {
		fmt.Println("DEBUG: Verificación entrada", i, "después de serializar - Nombre:", string(newBlock.B_content[i].B_name[:]), "Inodo:", newBlock.B_content[i].B_inodo)
	}

	fmt.Println("sb.S_inode_start+(parentInodeIndex*sb.S_inode_size = ", sb.S_inode_start+(parentInodeIndex*sb.S_inode_size))

	// Actualizar el bloque padre
	parentInode := &structures.Inode{}
	err = parentInode.Deserialize(path, int64(sb.S_inode_start+(parentInodeIndex*sb.S_inode_size)))
	if err != nil {
		return 0, fmt.Errorf("error al cargar inodo padre: %v", err)
	}

	// Buscar si la carpeta ya existe en alguno de los bloques del inodo padre
	existingInodeIndex := int32(-1)
	for i := 0; i < len(parentInode.I_block); i++ {
		if parentInode.I_block[i] == -1 {
			continue
		}
		// Cargar el bloque
		currentBlock := &structures.FolderBlock{}
		err = currentBlock.Deserialize(path, int64(parentInode.I_block[i]))
		if err != nil {
			return 0, fmt.Errorf("error al cargar bloque %d del inodo padre: %v", i, err)
		}
		// Revisar las entradas del bloque
		for j := 0; j < 4; j++ {
			name := strings.Trim(string(currentBlock.B_content[j].B_name[:]), "\x00")
			fmt.Println("DEBUG: Revisando entrada", j, "en bloque", i, "del inodo padre - Nombre:", name, "Inodo:", currentBlock.B_content[j].B_inodo)
			if name == folderName {
				existingInodeIndex = currentBlock.B_content[j].B_inodo
				break
			}
		}
		if existingInodeIndex != -1 {
			break
		}
	}

	// Si la carpeta ya existe, no debemos crearla de nuevo
	if existingInodeIndex != -1 {
		fmt.Println("DEBUG: La carpeta", folderName, "ya existe con inodo", existingInodeIndex)
		// Deserializar el inodo existente para verificar su tipo
		existingInode := &structures.Inode{}
		err = existingInode.Deserialize(path, int64(sb.S_inode_start+(existingInodeIndex*sb.S_inode_size)))
		if err != nil {
			return 0, fmt.Errorf("error al cargar inodo existente para %s: %v", folderName, err)
		}
		if existingInode.I_type[0] != '0' {
			return 0, fmt.Errorf("el nombre %s ya existe y no es una carpeta", folderName)
		}
		// Revertir los cambios en el superbloque y bitmaps, ya que no necesitamos el nuevo inodo ni el bloque
		sb.S_inodes_count--
		sb.S_free_inodes_count++
		sb.S_blocks_count--
		sb.S_free_blocks_count++
		sb.S_first_ino--
		sb.S_first_blo -= sb.S_block_size
		err = sb.SetBitmapInode(path, newInodeIndex, '0')
		if err != nil {
			return 0, fmt.Errorf("error al revertir bitmap de inodos: %v", err)
		}
		err = sb.SetBitmapBlock(path, newInode.I_block[0], '0')
		if err != nil {
			return 0, fmt.Errorf("error al revertir bitmap de bloques: %v", err)
		}
		fmt.Println("DEBUG: Cambios revertidos - First Ino:", sb.S_first_ino, "Inodes Count:", sb.S_inodes_count, "Free Inodes:", sb.S_free_inodes_count, "First Blo:", sb.S_first_blo, "Free Blocks:", sb.S_free_blocks_count)
		return existingInodeIndex, nil
	}

	// Si la carpeta no existe, procedemos a crearla
	// Actualizar el bloque padre
	parentBlock := &structures.FolderBlock{}
	err = parentBlock.Deserialize(path, int64(parentInode.I_block[0]))
	if err != nil {
		return 0, fmt.Errorf("error al cargar bloque padre: %v", err)
	}

	// Buscar una entrada libre en el bloque padre
	found := false
	for i := 0; i < 4; i++ {
		name := strings.Trim(string(parentBlock.B_content[i].B_name[:]), "\x00")
		fmt.Println("DEBUG: Revisando entrada", i, "en bloque padre antes de actualizar - Nombre:", string(parentBlock.B_content[i].B_name[:]), "Inodo:", parentBlock.B_content[i].B_inodo)
		if name == "" || parentBlock.B_content[i].B_inodo == -1 {
			copy(parentBlock.B_content[i].B_name[:], folderName)
			parentBlock.B_content[i].B_inodo = newInodeIndex
			fmt.Println("DEBUG: Actualizando entrada", i, "en bloque padre - Nombre:", folderName, "Inodo:", newInodeIndex)
			found = true
			break
		}
	}

	// Abrir el archivo para escritura y sincronización
	file, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return 0, fmt.Errorf("error al abrir el archivo para sincronizar: %v", err)
	}
	defer file.Close()

	// Si no hay espacio en el bloque padre actual, asignar un nuevo bloque
	if !found {
		fmt.Println("DEBUG: Se entró a !found para la carpeta: ", folderName)
		// Buscar un índice libre en I_block del inodo padre (empezando desde I_block[1])
		newBlockIndex := int32(-1)
		for i := 1; i < len(parentInode.I_block); i++ {
			if parentInode.I_block[i] == -1 {
				// Verificar si hay bloques disponibles
				if sb.S_free_blocks_count == 0 {
					return 0, fmt.Errorf("no hay bloques disponibles para asignar al directorio padre")
				}
				// Asignar un nuevo bloque usando S_first_blo
				newBlockIndex = sb.S_first_blo
				parentInode.I_block[i] = newBlockIndex
				fmt.Println("DEBUG: Nuevo bloque asignado al inodo padre - I_block[", i, "]:", newBlockIndex)

				// Actualizar bitmap y contadores para el nuevo bloque
				err = sb.SetBitmapBlock(path, newBlockIndex, '1')
				if err != nil {
					return 0, fmt.Errorf("error al actualizar bitmap de bloques para el nuevo bloque padre: %v", err)
				}
				sb.S_blocks_count++
				sb.S_free_blocks_count--
				sb.S_first_blo += sb.S_block_size
				fmt.Println("DEBUG: Contadores actualizados para nuevo bloque - First Blo:", sb.S_first_blo, "Blocks Count:", sb.S_blocks_count, "Free Blocks:", sb.S_free_blocks_count)
				break
			}
		}

		// Si no hay índices disponibles en I_block
		if newBlockIndex == -1 {
			return 0, fmt.Errorf("no hay índices disponibles en el inodo padre para asignar un nuevo bloque")
		}

		// Crear un nuevo bloque para el directorio padre
		newParentBlock := &structures.FolderBlock{}
		newParentBlock.B_content[0].B_name = [12]byte{}
		copy(newParentBlock.B_content[0].B_name[:], folderName)
		newParentBlock.B_content[0].B_inodo = newInodeIndex
		newParentBlock.B_content[1].B_inodo = -1
		newParentBlock.B_content[2].B_inodo = -1
		newParentBlock.B_content[3].B_inodo = -1
		fmt.Println("DEBUG: Nuevo bloque creado para el directorio padre - Entrada 0:", folderName, "Inodo:", newInodeIndex)

		// Serializar el nuevo bloque
		serializePos := int64(newBlockIndex)
		err = newParentBlock.Serialize(path, serializePos)
		if err != nil {
			return 0, fmt.Errorf("error al serializar nuevo bloque padre: %v", err)
		}
		fmt.Println("DEBUG: Nuevo bloque padre serializado en posición:", serializePos)

		// Verificar el contenido del bloque después de serializar
		verifyBlock := &structures.FolderBlock{}
		err = verifyBlock.Deserialize(path, serializePos)
		if err != nil {
			return 0, fmt.Errorf("error al verificar nuevo bloque padre después de serializar: %v", err)
		}
		for i := 0; i < 4; i++ {
			fmt.Println("DEBUG: Verificación nuevo bloque padre - Entrada", i, "Nombre:", string(verifyBlock.B_content[i].B_name[:]), "Inodo:", verifyBlock.B_content[i].B_inodo)
		}

		// Actualizar el inodo padre con el nuevo bloque
		err = parentInode.Serialize(path, int64(sb.S_inode_start+(parentInodeIndex*sb.S_inode_size)))
		if err != nil {
			return 0, fmt.Errorf("error al actualizar inodo padre con nuevo bloque: %v", err)
		}
		fmt.Println("DEBUG: Inodo padre actualizado con nuevo bloque - I_block:", parentInode.I_block)

		// Verificar el inodo padre después de serializar
		verifyInode := &structures.Inode{}
		err = verifyInode.Deserialize(path, int64(sb.S_inode_start+(parentInodeIndex*sb.S_inode_size)))
		if err != nil {
			return 0, fmt.Errorf("error al verificar inodo padre después de serializar: %v", err)
		}
		fmt.Println("DEBUG: Verificación inodo padre - I_block:", verifyInode.I_block)

		// Sincronizar el archivo para asegurar que los cambios se escriban en disco
		err = file.Sync()
		if err != nil {
			return 0, fmt.Errorf("error al sincronizar el archivo después de actualizar el nuevo bloque padre: %v", err)
		}
		fmt.Println("DEBUG: Archivo sincronizado después de actualizar el nuevo bloque padre")
	} else {
		// Si se encontró espacio, serializar el bloque padre actualizado
		serializePos := int64(parentInode.I_block[0])
		err = parentBlock.Serialize(path, serializePos)
		if err != nil {
			return 0, fmt.Errorf("error al actualizar bloque padre: %v", err)
		}
		fmt.Println("DEBUG: Bloque padre serializado en posición:", serializePos)

		// Verificar el contenido del bloque después de serializar
		verifyBlock := &structures.FolderBlock{}
		err = verifyBlock.Deserialize(path, serializePos)
		if err != nil {
			return 0, fmt.Errorf("error al verificar bloque padre después de serializar: %v", err)
		}
		for i := 0; i < 4; i++ {
			fmt.Println("DEBUG: Verificación bloque padre - Entrada", i, "Nombre:", string(verifyBlock.B_content[i].B_name[:]), "Inodo:", verifyBlock.B_content[i].B_inodo)
		}

		// Sincronizar el archivo para asegurar que los cambios se escriban en disco
		err = file.Sync()
		if err != nil {
			return 0, fmt.Errorf("error al sincronizar el archivo después de actualizar el bloque padre: %v", err)
		}
		fmt.Println("DEBUG: Archivo sincronizado después de actualizar el bloque padre")
	}

	// Log del estado final del superbloque
	fmt.Println("DEBUG: Estado final del superbloque - Inodes Count:", sb.S_inodes_count, "Blocks Count:", sb.S_blocks_count, "Free Inodes:", sb.S_free_inodes_count, "Free Blocks:", sb.S_free_blocks_count, "First Ino:", sb.S_first_ino, "First Blo:", sb.S_first_blo)

	return newInodeIndex, nil
}

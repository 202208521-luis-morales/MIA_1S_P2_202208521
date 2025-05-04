package commands

import (
	stores "backend/stores"
	structures "backend/structures"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// MKFILE estructura que representa el comando mkfile con sus parámetros
type MKFILE struct {
	path string // Ruta del archivo a crear
	r    bool   // Crear carpetas padres recursivamente
	size int32  // Tamaño en bytes del archivo
	cont string // Ruta del archivo local con contenido
}

// ParseMkfile (sin cambios, pero con debug)
// ParseMkfile parsea el comando mkfile y crea un archivo
func ParseMkfile(tokens []string) (string, error) {
	fmt.Println("DEBUG: Iniciando ParseMkfile - Tokens:", tokens)
	cmd := &MKFILE{size: 0}
	args := strings.Join(tokens, " ")
	// Expresión regular para capturar los parámetros válidos, insensible a mayúsculas/minúsculas
	re := regexp.MustCompile(`(?i)-path="[^"]+"|(?i)-path=[^\s]+|(?i)-r(?:\s|$)|(?i)-size=[^\s]+|(?i)-cont="[^"]+"|(?i)-cont=[^\s]+`)
	matches := re.FindAllString(args, -1)

	// Validar que no haya parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	allowedParams := map[string]bool{
		"-path": true,
		"-r":    true,
		"-size": true,
		"-cont": true,
	}

	for _, param := range allParams {
		// Extraer solo el nombre del parámetro (sin el valor) y convertir a minúsculas
		paramName := strings.ToLower(strings.SplitN(param, "=", 2)[0])
		if !allowedParams[strings.TrimSpace(paramName)] {
			return "", fmt.Errorf("parámetro no permitido: %s", paramName)
		}
	}

	for _, match := range matches {
		match = strings.TrimSpace(match)
		fmt.Println("DEBUG: Procesando match:", match)
		kv := strings.SplitN(match, "=", 2)
		key := strings.ToLower(kv[0])
		var value string
		if len(kv) == 2 {
			// Eliminar comillas si existen
			value = kv[1]
			if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
				value = strings.Trim(value, "\"")
			}
		}

		switch key {
		case "-path":
			if value == "" {
				return "", errors.New("la ruta no puede estar vacía")
			}
			cmd.path = value
		case "-r":
			if len(kv) > 1 {
				return "", errors.New("el parámetro -r no debe recibir valor")
			}
			cmd.r = true
		case "-size":
			if value == "" {
				return "", errors.New("el tamaño no puede estar vacío")
			}
			size, err := parseInt32(value)
			if err != nil || size < 0 {
				return "", errors.New("el tamaño debe ser un número entero no negativo")
			}
			cmd.size = size
		case "-cont":
			if value == "" {
				return "", errors.New("la ruta del contenido no puede estar vacía")
			}
			cmd.cont = value
		}
	}

	if cmd.path == "" {
		return "", errors.New("faltan parámetros requeridos: -path")
	}
	fmt.Println("DEBUG: Comando MKFILE parseado - Path:", cmd.path, "R:", cmd.r, "Size:", cmd.size, "Cont:", cmd.cont)

	overwrite, err := commandMkfile(cmd)
	if err != nil {
		return "", err
	}
	if overwrite {
		return fmt.Sprintf("MKFILE: Archivo sobreescrito exitosamente\n-> Ruta: %s", cmd.path), nil
	}
	return fmt.Sprintf("MKFILE: Archivo creado exitosamente\n-> Ruta: %s", cmd.path), nil
}

// generateContent genera una cadena de longitud size con un patrón repetitivo de caracteres
func generateContent(size int) string {
	pattern := "0123456789" // Patrón base que se repetirá
	patternLength := len(pattern)
	result := make([]byte, size)

	for i := 0; i < size; i++ {
		result[i] = pattern[i%patternLength]
	}

	fmt.Println("DEBUG: Contenido generado - Longitud:", len(result))
	return string(result)
}

// commandMkfile ejecuta la lógica para crear un archivo en la partición
func commandMkfile(mkfile *MKFILE) (bool, error) {
	fmt.Println("\nDEBUG: Iniciando commandMkfile - Path:", mkfile.path, "R:", mkfile.r, "Size:", mkfile.size, "Cont:", mkfile.cont)

	if stores.ActiveSession == nil {
		return false, errors.New("debe iniciar sesión primero para crear un archivo")
	}

	fmt.Println("DEBUG: Obteniendo superbloque y partición para ID:", stores.ActiveSession.PartitionID)
	sb, partition, path, err := stores.GetMountedPartitionSuperblock(stores.ActiveSession.PartitionID)
	if err != nil {
		return false, fmt.Errorf("error al obtener partición: %v", err)
	}
	fmt.Println("DEBUG: Partición encontrada - Path:", path, "Part_start:", partition.Part_start)
	fmt.Println("DEBUG: SuperBlock - Inodes Count:", sb.S_inodes_count, "Blocks Count:", sb.S_blocks_count, "Free Inodes:", sb.S_free_inodes_count, "Free Blocks:", sb.S_free_blocks_count)

	mkfile.path = filepath.Clean(mkfile.path)
	if !strings.HasPrefix(mkfile.path, "/") {
		return false, errors.New("la ruta debe ser absoluta (comenzar con /)")
	}
	fmt.Println("DEBUG: Ruta normalizada:", mkfile.path)

	dir, fileName := filepath.Split(mkfile.path)
	if fileName == "" {
		return false, errors.New("el nombre del archivo no puede estar vacío")
	}
	fmt.Println("DEBUG: Directorio padre:", dir, "Nombre del archivo:", fileName)

	parentInode, parentBlock, parentIndex, err := findOrCreateParentDir(sb, partition, path, dir, mkfile.r)
	if err != nil {
		return false, fmt.Errorf("error al buscar/crear directorio padre: %v", err)
	}
	fmt.Println("DEBUG: Directorio padre encontrado - Inodo:", parentIndex, "Bloque[0]:", parentInode.I_block[0])

	if stores.ActiveSession.Username != "root" {
		hasPermission := checkWritePermission(parentInode)
		fmt.Println("DEBUG: Verificando permisos de escritura - UserID:", stores.ActiveSession.UserID, "GroupID:", stores.ActiveSession.GroupID, "Permisos:", string(parentInode.I_perm[:]), "Resultado:", hasPermission)
		if !hasPermission {
			return false, errors.New("permiso denegado: no tiene permisos de escritura en la carpeta padre")
		}
	}

	// Buscar si el archivo ya existe en todos los bloques del inodo padre
	var existingInodeIndex int32 = -1
	var blockIndexToUse int32 = -1 // Para saber en qué bloque del inodo padre insertar el nuevo archivo
	for i := 0; i < len(parentInode.I_block); i++ {
		if parentInode.I_block[i] == -1 {
			continue
		}
		currentBlock := &structures.FolderBlock{}
		err = currentBlock.Deserialize(path, int64(parentInode.I_block[i]))
		if err != nil {
			return false, fmt.Errorf("error al cargar bloque %d del inodo padre: %v", i, err)
		}
		for j, content := range currentBlock.B_content {
			name := strings.Trim(string(content.B_name[:]), "\x00")
			fmt.Printf("DEBUG: Revisando entrada %d en bloque %d del inodo padre - Nombre: %q, Inodo: %d\n", j, i, name, content.B_inodo)
			if name == fileName {
				existingInodeIndex = content.B_inodo
				parentBlock = currentBlock // Actualizar parentBlock al bloque donde encontramos el archivo
				blockIndexToUse = parentInode.I_block[i]
				break
			}
		}
		if existingInodeIndex != -1 {
			break
		}
		// Si no encontramos el archivo, pero este bloque tiene espacio, lo usaremos para insertar el nuevo archivo
		if blockIndexToUse == -1 {
			for _, content := range currentBlock.B_content {
				name := strings.Trim(string(content.B_name[:]), "\x00")
				if name == "" || content.B_inodo == -1 {
					parentBlock = currentBlock
					blockIndexToUse = parentInode.I_block[i]
					break
				}
			}
		}
	}

	if existingInodeIndex != -1 {
		fmt.Println("DEBUG: Archivo existente encontrado en inodo:", existingInodeIndex)
		existingInode := &structures.Inode{}
		err = existingInode.Deserialize(path, int64(sb.S_inode_start+existingInodeIndex*sb.S_inode_size))
		if err != nil {
			return false, fmt.Errorf("error al cargar inodo existente: %v", err)
		}
		fmt.Println("DEBUG: Inodo existente cargado - Size:", existingInode.I_size, "Bloque[0]:", existingInode.I_block[0])

		err = clearFileBlocks(sb, partition, path, existingInode)
		if err != nil {
			return false, fmt.Errorf("error al limpiar bloques existentes: %v", err)
		}
		fmt.Println("DEBUG: Bloques antiguos limpiados")

		// Leer el contenido de -cont si está presente, o generar contenido con -size
		content := ""
		if mkfile.cont != "" {
			// Leer el contenido del archivo especificado en -cont
			var err error
			content, err = readContentFromFile(mkfile.cont)
			if err != nil {
				return false, fmt.Errorf("error al leer el contenido de -cont: %v", err)
			}
		} else if mkfile.size > 0 {
			content = generateContent(int(mkfile.size)) // Convertir mkfile.size a int
		}

		fmt.Println("DEBUG: Iniciando writeFileContent - Size:", len(content), "Cont:", content)
		err = writeFileContent(sb, partition, path, existingInode, content)
		if err != nil {
			return false, fmt.Errorf("error al escribir contenido: %v", err)
		}
		fmt.Println("DEBUG: Contenido escrito en inodo existente - Nuevo tamaño:", existingInode.I_size)

		err = existingInode.Serialize(path, int64(sb.S_inode_start+existingInodeIndex*sb.S_inode_size))
		if err != nil {
			return false, fmt.Errorf("error al serializar inodo existente: %v", err)
		}
		fmt.Println("DEBUG: Inodo existente actualizado")

		err = sb.Serialize(path, int64(partition.Part_start))
		if err != nil {
			return false, fmt.Errorf("error al actualizar superbloque: %v", err)
		}
		fmt.Println("DEBUG: Superbloque actualizado tras sobreescritura")
		return true, nil
	}

	// Si no hay espacio en los bloques actuales del inodo padre, asignar un nuevo bloque
	if blockIndexToUse == -1 {
		fmt.Println("DEBUG: No hay espacio en los bloques actuales del inodo padre, asignando un nuevo bloque")
		newBlockIndex := int32(-1)
		for i := 0; i < len(parentInode.I_block); i++ {
			if parentInode.I_block[i] == -1 {
				if sb.S_free_blocks_count == 0 {
					return false, fmt.Errorf("no hay bloques disponibles para asignar al directorio padre")
				}
				newBlockIndex = sb.S_first_blo
				parentInode.I_block[i] = newBlockIndex
				fmt.Println("DEBUG: Nuevo bloque asignado al inodo padre - I_block[", i, "]:", newBlockIndex)

				// Actualizar bitmap y contadores para el nuevo bloque
				err = sb.SetBitmapBlock(path, newBlockIndex, '1')
				if err != nil {
					return false, fmt.Errorf("error al actualizar bitmap de bloques para el nuevo bloque padre: %v", err)
				}
				sb.S_blocks_count++
				sb.S_free_blocks_count--
				sb.S_first_blo += sb.S_block_size
				fmt.Println("DEBUG: Contadores actualizados para nuevo bloque - First Blo:", sb.S_first_blo, "Blocks Count:", sb.S_blocks_count, "Free Blocks:", sb.S_free_blocks_count)

				// Crear un nuevo bloque para el directorio padre
				parentBlock = &structures.FolderBlock{}
				for j := 0; j < 4; j++ {
					parentBlock.B_content[j].B_inodo = -1
				}
				err = parentBlock.Serialize(path, int64(newBlockIndex))
				if err != nil {
					return false, fmt.Errorf("error al serializar nuevo bloque padre: %v", err)
				}
				fmt.Println("DEBUG: Nuevo bloque padre creado y serializado - Posición:", newBlockIndex)

				// Actualizar el inodo padre
				err = parentInode.Serialize(path, int64(sb.S_inode_start+parentIndex*sb.S_inode_size))
				if err != nil {
					return false, fmt.Errorf("error al serializar inodo padre: %v", err)
				}
				fmt.Println("DEBUG: Inodo padre actualizado con nuevo bloque")
				blockIndexToUse = newBlockIndex
				break
			}
		}
		if newBlockIndex == -1 {
			return false, fmt.Errorf("no hay índices disponibles en el inodo padre para asignar un nuevo bloque")
		}
	}

	fmt.Println("DEBUG: Creando nuevo archivo")
	newInodeIndex, err := createNewFile(sb, path, fileName, parentIndex)
	if err != nil {
		return false, fmt.Errorf("error al crear nuevo archivo: %v", err)
	}
	fmt.Println("DEBUG: Nuevo archivo creado - Inodo:", newInodeIndex)

	// Insertar la entrada del nuevo archivo en el bloque padre
	found := false
	for i := 0; i < 4; i++ {
		name := strings.Trim(string(parentBlock.B_content[i].B_name[:]), "\x00")
		if name == "" || parentBlock.B_content[i].B_inodo == -1 {
			copy(parentBlock.B_content[i].B_name[:], fileName)
			parentBlock.B_content[i].B_inodo = newInodeIndex
			fmt.Println("DEBUG: Entrada creada en bloque padre - Nombre:", fileName, "Inodo:", newInodeIndex)
			found = true
			break
		}
	}
	if !found {
		return false, fmt.Errorf("no hay espacio en el bloque padre para insertar el nuevo archivo (esto no debería ocurrir)")
	}

	// Serializar el bloque padre actualizado
	err = parentBlock.Serialize(path, int64(blockIndexToUse))
	if err != nil {
		return false, fmt.Errorf("error al serializar bloque padre: %v", err)
	}
	fmt.Println("DEBUG: Bloque padre actualizado - Posición:", blockIndexToUse)

	// Escribir el contenido en el nuevo archivo
	content := ""

	if mkfile.cont != "" {
		// Leer el contenido del archivo especificado en -cont
		var err error
		content, err = readContentFromFile(mkfile.cont)
		if err != nil {
			return false, fmt.Errorf("error al leer el contenido de -cont: %v", err)
		}
	} else if mkfile.size > 0 {
		content = generateContent(int(mkfile.size)) // Convertir mkfile.size a int
	}

	fmt.Println("DEBUG: Iniciando writeFileContent - Size:", len(content), "Cont:", content)
	newInode := &structures.Inode{}
	err = newInode.Deserialize(path, int64(sb.S_inode_start+newInodeIndex*sb.S_inode_size))
	if err != nil {
		return false, fmt.Errorf("error al cargar nuevo inodo: %v", err)
	}
	err = writeFileContent(sb, partition, path, newInode, content)
	if err != nil {
		return false, fmt.Errorf("error al escribir contenido: %v", err)
	}
	fmt.Println("DEBUG: Contenido escrito en nuevo inodo - Tamaño:", newInode.I_size)

	// Serializar el inodo actualizado
	err = newInode.Serialize(path, int64(sb.S_inode_start+newInodeIndex*sb.S_inode_size))
	if err != nil {
		return false, fmt.Errorf("error al serializar nuevo inodo: %v", err)
	}
	fmt.Println("DEBUG: Nuevo inodo actualizado")

	// Actualizar el superbloque
	err = sb.Serialize(path, int64(partition.Part_start))
	if err != nil {
		return false, fmt.Errorf("error al actualizar superbloque: %v", err)
	}
	fmt.Println("DEBUG: Superbloque actualizado")
	fmt.Println("DEBUG: Nuevo archivo creado exitosamente")

	return false, nil // Cambiado a false para indicar creación, no sobreescritura
}

// findOrCreateParentDir con debug
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

// createNewFile crea un nuevo inodo para un archivo, pero no actualiza el bloque padre
func createNewFile(sb *structures.SuperBlock, path string, fileName string, parentInodeIndex int32) (int32, error) {
	fmt.Println("DEBUG: Iniciando createNewFile")
	// Crear un nuevo inodo para el archivo
	newInode := &structures.Inode{
		I_uid:   1,
		I_gid:   1,
		I_size:  0,
		I_atime: float32(time.Now().Unix()),
		I_ctime: float32(time.Now().Unix()),
		I_mtime: float32(time.Now().Unix()),
		I_type:  [1]byte{'1'}, // Archivo
		I_perm:  [3]byte{'6', '6', '4'},
	}
	for i := 0; i < 15; i++ {
		newInode.I_block[i] = -1
	}

	// Asignar el índice del nuevo inodo
	newInodeIndex := sb.S_first_ino
	fmt.Println("DEBUG: Nuevo inodo creado - Index:", newInodeIndex)

	// Actualizar el bitmap de inodos
	err := sb.UpdateBitmapInode(path, newInodeIndex)
	if err != nil {
		return 0, fmt.Errorf("error al actualizar bitmap de inodos: %v", err)
	}

	// Actualizar contadores después de usar S_first_ino
	sb.S_inodes_count++
	sb.S_free_inodes_count--
	sb.S_first_ino++
	fmt.Println("DEBUG: Bitmap de inodos actualizado - First Ino:", sb.S_first_ino, "Inodes Count:", sb.S_inodes_count, "Free Inodes:", sb.S_free_inodes_count)

	// Serializar el nuevo inodo
	err = newInode.Serialize(path, int64(sb.S_inode_start+(newInodeIndex*sb.S_inode_size)))
	if err != nil {
		return 0, fmt.Errorf("error al serializar nuevo inodo: %v", err)
	}
	fmt.Println("DEBUG: Nuevo inodo serializado")

	return newInodeIndex, nil
}

// writeFileContent con debug
func writeFileContent(sb *structures.SuperBlock, partition *structures.Partition, path string, inode *structures.Inode, content string) error {
	fmt.Println("DEBUG: Iniciando writeFileContent - Size:", len(content), "Cont:", content)

	// Calcular cuántos bloques se necesitan
	blockSize := int(sb.S_block_size) // 64 bytes por bloque
	numBlocksNeeded := (len(content) + blockSize - 1) / blockSize
	if numBlocksNeeded > 15 { // I_block tiene 15 entradas
		return fmt.Errorf("el contenido excede el límite de bloques soportados (15)")
	}
	fmt.Println("DEBUG: Bloques necesarios:", numBlocksNeeded)

	// Reutilizar bloques existentes y asignar nuevos si es necesario
	contentBytes := []byte(content)
	for i := 0; i < numBlocksNeeded; i++ {
		start := i * blockSize
		end := start + blockSize
		if end > len(contentBytes) {
			end = len(contentBytes)
		}

		fileBlock := &structures.FileBlock{}
		if inode.I_block[i] != -1 {
			// Reutilizar bloque existente
			err := fileBlock.Deserialize(path, int64(inode.I_block[i]))
			if err != nil {
				return fmt.Errorf("error al cargar bloque %d: %v", i, err)
			}
		} else {
			// Asignar un nuevo bloque
			if sb.S_free_blocks_count <= 0 {
				return fmt.Errorf("no hay bloques libres para el archivo")
			}
			inode.I_block[i] = sb.S_first_blo
			sb.S_first_blo += sb.S_block_size
			sb.S_free_blocks_count--
			sb.S_blocks_count++
			err := sb.UpdateBitmapBlockAt(path, inode.I_block[i], '1')
			if err != nil {
				return fmt.Errorf("error al actualizar bitmap de bloques: %v", err)
			}
			fmt.Println("DEBUG: Nuevo bloque asignado - Posición:", inode.I_block[i], "First Blo:", sb.S_first_blo, "Free Blocks:", sb.S_free_blocks_count)
		}

		// Copiar el segmento del contenido al bloque
		copy(fileBlock.B_content[:], contentBytes[start:end])
		err := fileBlock.Serialize(path, int64(inode.I_block[i]))
		if err != nil {
			return fmt.Errorf("error al escribir bloque %d: %v", i, err)
		}
		fmt.Println("DEBUG: Bloque", i, "escrito - Contenido:", string(fileBlock.B_content[:]))
	}

	// Liberar bloques sobrantes si el contenido se redujo
	for i := numBlocksNeeded; i < 15 && inode.I_block[i] != -1; i++ {
		err := sb.UpdateBitmapBlockAt(path, inode.I_block[i], '0')
		if err != nil {
			return fmt.Errorf("error al liberar bloque %d: %v", i, err)
		}
		inode.I_block[i] = -1
		sb.S_free_blocks_count++
		sb.S_blocks_count--
	}

	// Actualizar el tamaño del inodo
	inode.I_size = int32(len(content))
	inode.I_mtime = float32(time.Now().Unix())
	fmt.Println("DEBUG: Contenido escrito - Tamaño:", inode.I_size)

	return nil
}

// clearFileBlocks con debug
func clearFileBlocks(sb *structures.SuperBlock, partition *structures.Partition, path string, inode *structures.Inode) error {
	fmt.Println("DEBUG: Iniciando clearFileBlocks")
	for i := 0; i < 15 && inode.I_block[i] != -1; i++ {
		err := sb.UpdateBitmapBlockAt(path, inode.I_block[i], '0')
		if err != nil {
			return fmt.Errorf("error al liberar bloque: %v", err)
		}
		fmt.Println("DEBUG: Bloque liberado - Posición:", inode.I_block[i])
		inode.I_block[i] = -1
		sb.S_free_blocks_count++
	}
	inode.I_size = 0
	fmt.Println("DEBUG: Todos los bloques limpiados - Nuevo tamaño:", inode.I_size)
	return nil
}

// checkWritePermission y parseInt32 sin cambios
func checkWritePermission(inode *structures.Inode) bool {
	if stores.ActiveSession.Username == "root" {
		return true
	}
	userPerm := int(inode.I_perm[0] - '0')
	groupPerm := int(inode.I_perm[1] - '0')
	otherPerm := int(inode.I_perm[2] - '0')

	if stores.ActiveSession.UserID == inode.I_uid {
		return userPerm&2 != 0
	}
	if stores.ActiveSession.GroupID == inode.I_gid {
		return groupPerm&2 != 0
	}
	return otherPerm&2 != 0
}

func parseInt32(s string) (int32, error) {
	var n int32
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// readContentFromFile lee el contenido de un archivo local
func readContentFromFile(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("error al leer el archivo de contenido: %v", err)
	}
	fmt.Println("DEBUG: Contenido leído desde archivo - Ruta:", filePath, "Longitud:", len(data))
	return string(data), nil
}

package commands

import (
	stores "backend/stores"
	structures "backend/structures"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// CAT estructura que representa el comando cat con sus parámetros
type CAT struct {
	files []string // Lista de rutas de archivos a leer
}

// ParseCat parsea el comando cat y muestra el contenido de los archivos
func ParseCat(tokens []string) (string, error) {
	fmt.Println("\nDEBUG: Iniciando ParseCat - Tokens:", tokens)
	cmd := &CAT{}
	args := strings.Join(tokens, " ")
	// Expresión regular para capturar los parámetros válidos, insensible a mayúsculas/minúsculas para "file"
	re := regexp.MustCompile(`(?i)-file\d+=[^\s]+|(?i)-file\d+="[^"]+"`)
	matches := re.FindAllString(args, -1)

	// Validar que no haya parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+[0-9]*(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	allowedParams := map[string]bool{}
	// Permitir -fileN dinámicamente (file1, file2, etc.)
	for _, param := range allParams {
		paramName := strings.ToLower(strings.SplitN(param, "=", 2)[0])
		if !strings.HasPrefix(paramName, "-file") || !regexp.MustCompile(`-file\d+`).MatchString(paramName) {
			return "", fmt.Errorf("parámetro no permitido: %s", paramName)
		}
		allowedParams[paramName] = true
	}

	fmt.Println("DEBUG: Matches encontrados:", matches)
	if len(matches) == 0 {
		return "", errors.New("faltan parámetros requeridos: al menos un -fileN")
	}

	for _, match := range matches {
		fmt.Println("DEBUG: Procesando match:", match)
		kv := strings.SplitN(match, "=", 2)
		if len(kv) != 2 {
			return "", fmt.Errorf("formato de parámetro inválido: %s", match)
		}
		value := strings.Trim(kv[1], "\"")
		if value == "" {
			return "", errors.New("la ruta del archivo no puede estar vacía")
		}
		cmd.files = append(cmd.files, value)
		fmt.Println("DEBUG: Archivo añadido a la lista - File:", value)
	}

	fmt.Println("DEBUG: Comando CAT parseado - Files:", cmd.files)
	content, err := commandCat(cmd)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("CAT: Contenido de los archivos:\n%s", content), nil
}

// commandCat lee el contenido de los archivos y lo concatena
// commandCat lee y concatena el contenido de los archivos especificados
func commandCat(cat *CAT) (string, error) {
	fmt.Println("DEBUG: Iniciando commandCat - Files:", cat.files)

	if stores.ActiveSession == nil {
		return "", errors.New("debe iniciar sesión primero para leer archivos")
	}
	fmt.Println("DEBUG: Sesión activa - User:", stores.ActiveSession.Username, "ID:", stores.ActiveSession.PartitionID)

	sb, partition, path, err := stores.GetMountedPartitionSuperblock(stores.ActiveSession.PartitionID)
	if err != nil {
		return "", fmt.Errorf("error al obtener partición: %v", err)
	}
	fmt.Println("DEBUG: Partición encontrada - Path:", path, "Part_start:", partition.Part_start)
	fmt.Println("DEBUG: SuperBlock - Inodes Count:", sb.S_inodes_count, "Blocks Count:", sb.S_blocks_count, "Free Inodes:", sb.S_free_inodes_count, "Free Blocks:", sb.S_free_blocks_count)

	var result strings.Builder
	for i, filePath := range cat.files {
		fmt.Println("DEBUG: Procesando archivo", i+1, "- Ruta:", filePath)
		filePath = filepath.Clean(filePath)
		if !strings.HasPrefix(filePath, "/") {
			return "", fmt.Errorf("la ruta debe ser absoluta (comenzar con /): %s", filePath)
		}
		fmt.Println("DEBUG: Ruta normalizada:", filePath)

		dir, fileName := filepath.Split(filePath)
		if fileName == "" {
			return "", fmt.Errorf("el nombre del archivo no puede estar vacío: %s", filePath)
		}
		fmt.Println("DEBUG: Directorio padre:", dir, "Nombre del archivo:", fileName)

		// Obtener el inodo padre y el bloque padre
		parentInode, _, parentIndex, err := findOrCreateParentDir(sb, partition, path, dir, false)
		if err != nil {
			return "", fmt.Errorf("error al acceder al directorio padre de %s: %v", filePath, err)
		}
		fmt.Println("DEBUG: Directorio padre encontrado - Inodo:", parentIndex)

		// Buscar el archivo en todos los bloques del inodo padre
		var inodeIndex int32 = -1
		for blockIdx := 0; blockIdx < len(parentInode.I_block); blockIdx++ {
			if parentInode.I_block[blockIdx] == -1 {
				continue // Saltar bloques no asignados
			}
			currentBlock := &structures.FolderBlock{}
			err = currentBlock.Deserialize(path, int64(parentInode.I_block[blockIdx]))
			if err != nil {
				return "", fmt.Errorf("error al cargar bloque %d del inodo padre: %v", blockIdx, err)
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
			return "", fmt.Errorf("el archivo no existe: %s", filePath)
		}
		fmt.Println("DEBUG: Archivo encontrado - Inodo:", inodeIndex)

		fileInode := &structures.Inode{}
		err = fileInode.Deserialize(path, int64(sb.S_inode_start+inodeIndex*sb.S_inode_size))
		if err != nil {
			return "", fmt.Errorf("error al cargar inodo de %s: %v", filePath, err)
		}
		fmt.Println("DEBUG: Inodo cargado - Size:", fileInode.I_size, "Type:", string(fileInode.I_type[:]), "Bloque[0]:", fileInode.I_block[0])

		if stores.ActiveSession.Username != "root" {
			hasPermission := checkReadPermission(fileInode)
			fmt.Println("DEBUG: Verificando permisos de lectura - UserID:", stores.ActiveSession.UserID, "GroupID:", stores.ActiveSession.GroupID, "Permisos:", string(fileInode.I_perm[:]), "Resultado:", hasPermission)
			if !hasPermission {
				return "", fmt.Errorf("permiso denegado: no tiene permisos de lectura para %s", filePath)
			}
		} else {
			fmt.Println("DEBUG: Usuario root, permisos de lectura otorgados automáticamente")
		}

		content, err := readFileContent(sb, path, fileInode)
		if err != nil {
			return "", fmt.Errorf("error al leer el contenido de %s: %v", filePath, err)
		}
		fmt.Println("DEBUG: Contenido leído del archivo:", content)

		result.WriteString(content)
		if i < len(cat.files)-1 {
			result.WriteString("\n")
			fmt.Println("DEBUG: Añadiendo salto de línea entre archivos")
		}
	}

	finalContent := result.String()
	fmt.Println("DEBUG: Contenido concatenado final:", finalContent)
	return finalContent, nil
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

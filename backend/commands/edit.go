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
)

// EDIT estructura que representa el comando edit con sus parámetros
type EDIT struct {
	path      string // Ruta del archivo a editar
	contenido string // Ruta del archivo en el sistema operativo con el nuevo contenido
}

// ParseEdit parsea el comando edit y devuelve el resultado de la edición
func ParseEdit(tokens []string) (string, error) {
	cmd := &EDIT{}
	args := strings.Join(tokens, " ")
	fmt.Println("DEBUG: Iniciando ParseEdit - Tokens:", tokens)
	fmt.Println("DEBUG: Args combinados:", args)

	re := regexp.MustCompile(`(?i)-path="[^"]+"|(?i)-path=[^\s]+|(?i)-contenido="[^"]+"|(?i)-contenido=[^\s]+`)
	matches := re.FindAllString(args, -1)
	fmt.Println("DEBUG: Coincidencias encontradas:", matches)

	// Validar parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	fmt.Println("DEBUG: Todos los parámetros encontrados:", allParams)
	allowedParams := map[string]bool{
		"-path":      true,
		"-contenido": true,
	}

	for _, param := range allParams {
		paramName := strings.ToLower(strings.SplitN(param, "=", 2)[0])
		fmt.Println("DEBUG: Verificando parámetro:", paramName)
		if !allowedParams[paramName] {
			fmt.Println("DEBUG: Error - Parámetro no permitido:", paramName)
			return "", fmt.Errorf("parámetro no permitido: %s", paramName)
		}
	}

	for _, match := range matches {
		fmt.Println("DEBUG: Procesando coincidencia:", match)
		kv := strings.SplitN(match, "=", 2)
		if len(kv) != 2 {
			fmt.Println("DEBUG: Error - Formato de parámetro inválido:", match)
			return "", fmt.Errorf("formato de parámetro inválido: %s", match)
		}
		key := strings.ToLower(kv[0])
		value := strings.Trim(kv[1], "\"")
		fmt.Println("DEBUG: Clave:", key, "Valor:", value)

		switch key {
		case "-path":
			if value == "" {
				fmt.Println("DEBUG: Error - La ruta no puede estar vacía")
				return "", errors.New("la ruta no puede estar vacía")
			}
			cmd.path = value
			fmt.Println("DEBUG: Ruta establecida:", cmd.path)
		case "-contenido":
			if value == "" {
				fmt.Println("DEBUG: Error - La ruta del contenido no puede estar vacía")
				return "", errors.New("la ruta del contenido no puede estar vacía")
			}
			cmd.contenido = value
			fmt.Println("DEBUG: Ruta del contenido establecida:", cmd.contenido)
		}
	}

	if cmd.path == "" || cmd.contenido == "" {
		fmt.Println("DEBUG: Error - Falta alguno de los parámetros requeridos: -path o -contenido")
		return "", fmt.Errorf("faltan parámetros requeridos: -path y -contenido son obligatorios")
	}

	fmt.Println("DEBUG: Ejecutando commandEdit con cmd:", cmd)
	err := commandEdit(cmd)
	if err != nil {
		fmt.Println("DEBUG: Error en commandEdit:", err)
		return "", err
	}
	fmt.Println("DEBUG: commandEdit ejecutado exitosamente")
	return fmt.Sprintf("EDIT: Archivo editado exitosamente\n-> Ruta: %s", cmd.path), nil
}

// commandEdit edita el contenido de un archivo
func commandEdit(edit *EDIT) error {
	fmt.Println("\nDEBUG: Iniciando commandEdit - Path:", edit.path, "Contenido:", edit.contenido)

	// Verificar sesión activa
	fmt.Println("DEBUG: Verificando sesión activa...")
	if stores.ActiveSession == nil {
		fmt.Println("DEBUG: Error - No hay sesión activa")
		return errors.New("debe iniciar sesión primero para editar un archivo")
	}
	fmt.Println("DEBUG: Sesión activa encontrada - PartitionID:", stores.ActiveSession.PartitionID)

	// Obtener superbloque y partición montada
	fmt.Println("DEBUG: Obteniendo superbloque y partición...")
	sb, partition, path, err := stores.GetMountedPartitionSuperblock(stores.ActiveSession.PartitionID)
	if err != nil {
		fmt.Println("DEBUG: Error al obtener partición:", err)
		return fmt.Errorf("error al obtener partición: %v", err)
	}
	fmt.Println("DEBUG: Superbloque y partición obtenidos - Path:", path, "Part_start:", partition.Part_start)
	fmt.Println("DEBUG: SuperBlock - S_inode_start:", sb.S_inode_start, "S_block_start:", sb.S_block_start)

	// Normalizar la ruta
	fmt.Println("DEBUG: Normalizando ruta:", edit.path)
	edit.path = filepath.Clean(edit.path)
	fmt.Println("DEBUG: Ruta normalizada:", edit.path)
	if !strings.HasPrefix(edit.path, "/") {
		fmt.Println("DEBUG: Error - La ruta no es absoluta")
		return errors.New("la ruta debe ser absoluta (comenzar con /)")
	}

	// Separar directorio padre y nombre del archivo
	fmt.Println("DEBUG: Separando directorio padre y nombre del archivo...")
	dir, fileName := filepath.Split(edit.path)
	fmt.Println("DEBUG: Directorio padre:", dir, "Nombre del archivo:", fileName)
	if fileName == "" {
		fmt.Println("DEBUG: Error - Nombre del archivo vacío")
		return errors.New("el nombre del archivo no puede estar vacío")
	}

	// Buscar el directorio padre
	fmt.Println("DEBUG: Buscando directorio padre:", dir)
	_, parentBlock, _, err := findParentDir(sb, partition, path, dir)
	if err != nil {
		fmt.Println("DEBUG: Error al buscar directorio padre:", err)
		return fmt.Errorf("error al buscar directorio padre: %v", err)
	}
	fmt.Println("DEBUG: Directorio padre encontrado - Entrada 0:", string(parentBlock.B_content[0].B_name[:]))

	// Buscar el archivo en el directorio padre
	fmt.Println("DEBUG: Buscando archivo en el directorio padre:", fileName)
	var targetInodeIndex int32 = -1
	for i, content := range parentBlock.B_content {
		name := strings.Trim(string(content.B_name[:]), "\x00")
		fmt.Printf("DEBUG: Entrada %d - Nombre: %q, Inodo: %d\n", i, name, content.B_inodo)
		if name == fileName {
			targetInodeIndex = content.B_inodo
			fmt.Println("DEBUG: Archivo encontrado - Inodo:", targetInodeIndex)
			break
		}
	}

	if targetInodeIndex == -1 {
		fmt.Println("DEBUG: Error - Archivo no encontrado:", fileName)
		return fmt.Errorf("el archivo %s no existe en la ruta especificada", fileName)
	}

	// Leer el inodo del archivo
	fmt.Println("DEBUG: Leyendo inodo del archivo - Inodo:", targetInodeIndex)
	inode := &structures.Inode{}
	inodePosition := int64(sb.S_inode_start) + int64(targetInodeIndex)*int64(sb.S_inode_size)
	fmt.Println("DEBUG: Deserializando inodo - Posición:", inodePosition)
	err = inode.Deserialize(path, inodePosition)
	if err != nil {
		fmt.Println("DEBUG: Error al leer inodo del archivo:", err)
		return fmt.Errorf("error al leer inodo del archivo: %v", err)
	}
	fmt.Println("DEBUG: Inodo deserializado - I_type:", string(inode.I_type[:]), "I_size:", inode.I_size)

	// Verificar que sea un archivo (no una carpeta)
	fmt.Println("DEBUG: Verificando que sea un archivo...")
	if inode.I_type[0] != '1' { // '1' para archivos
		fmt.Println("DEBUG: Error - La ruta no corresponde a un archivo")
		return errors.New("la ruta especificada no corresponde a un archivo")
	}

	// Verificar permisos de lectura
	fmt.Println("DEBUG: Verificando permisos de lectura...")
	if stores.ActiveSession.Username != "root" {
		hasReadPermission := checkReadPermission(inode)
		fmt.Println("DEBUG: Permiso de lectura:", hasReadPermission)
		if !hasReadPermission {
			fmt.Println("DEBUG: Error - Permiso de lectura denegado")
			return errors.New("permiso denegado: no tiene permisos de lectura en el archivo")
		}
	} else {
		fmt.Println("DEBUG: Usuario es root, permisos de lectura no verificados")
	}

	// Verificar permisos de escritura
	fmt.Println("DEBUG: Verificando permisos de escritura...")
	if stores.ActiveSession.Username != "root" {
		hasWritePermission := checkWritePermission(inode)
		fmt.Println("DEBUG: Permiso de escritura:", hasWritePermission)
		if !hasWritePermission {
			fmt.Println("DEBUG: Error - Permiso de escritura denegado")
			return errors.New("permiso denegado: no tiene permisos de escritura en el archivo")
		}
	} else {
		fmt.Println("DEBUG: Usuario es root, permisos de escritura no verificados")
	}

	// Leer el nuevo contenido desde el archivo del sistema operativo
	fmt.Println("DEBUG: Leyendo nuevo contenido desde:", edit.contenido)
	newContent, err := os.ReadFile(edit.contenido)
	if err != nil {
		fmt.Println("DEBUG: Error al leer el archivo de contenido:", err)
		return fmt.Errorf("error al leer el archivo de contenido: %v", err)
	}
	fmt.Println("DEBUG: Contenido leído - Longitud:", len(newContent))

	// Liberar los bloques actuales del archivo
	fmt.Println("DEBUG: Liberando bloques actuales del archivo...")
	for i, blockIndex := range inode.I_block {
		if blockIndex != -1 {
			fmt.Println("DEBUG: Liberando bloque - Índice:", blockIndex)
			err := freeBlock(sb, path, blockIndex)
			if err != nil {
				fmt.Println("DEBUG: Error al liberar bloque:", err)
				return fmt.Errorf("error al liberar bloque: %v", err)
			}
			inode.I_block[i] = -1
			fmt.Println("DEBUG: Bloque liberado - Nuevo valor de I_block[", i, "]:", inode.I_block[i])
		}
	}

	// Asignar nuevos bloques para el nuevo contenido
	contentLen := len(newContent)
	fmt.Println("DEBUG: Longitud del nuevo contenido:", contentLen)
	blocksNeeded := (contentLen + 63) / 64 // Cada FileBlock almacena 64 bytes
	fmt.Println("DEBUG: Bloques necesarios:", blocksNeeded)
	if blocksNeeded > 15 {
		fmt.Println("DEBUG: Error - Contenido demasiado grande:", blocksNeeded)
		return fmt.Errorf("el contenido es demasiado grande: se necesitan %d bloques, pero el inodo solo soporta 15", blocksNeeded)
	}

	// Actualizar el tamaño del archivo
	fmt.Println("DEBUG: Actualizando tamaño del archivo a:", contentLen)
	inode.I_size = int32(contentLen)

	// Escribir el nuevo contenido en los bloques
	fmt.Println("DEBUG: Escribiendo nuevo contenido en bloques...")
	for i := 0; i < blocksNeeded; i++ {
		// Asignar un nuevo bloque
		fmt.Println("DEBUG: Asignando bloque", i)
		blockIndex, err := allocateBlock(sb, path)
		if err != nil {
			fmt.Println("DEBUG: Error al asignar bloque:", err)
			return fmt.Errorf("error al asignar bloque: %v", err)
		}
		inode.I_block[i] = blockIndex
		fmt.Println("DEBUG: Bloque asignado - Índice:", blockIndex)

		// Preparar el contenido del bloque
		start := i * 64
		end := start + 64
		if end > contentLen {
			end = contentLen
		}
		blockContent := make([]byte, 64)
		copy(blockContent, newContent[start:end])
		fmt.Println("DEBUG: Contenido del bloque", i, "- Longitud:", len(blockContent), "Inicio:", start, "Fin:", end)

		// Escribir el bloque
		fileBlock := &structures.FileBlock{}
		copy(fileBlock.B_content[:], blockContent)
		// Usar blockIndex como posición absoluta
		fmt.Println("DEBUG: Serializando bloque - Posición:", blockIndex)
		err = fileBlock.Serialize(path, int64(blockIndex))
		if err != nil {
			fmt.Println("DEBUG: Error al escribir bloque:", err)
			return fmt.Errorf("error al escribir bloque: %v", err)
		}
		fmt.Println("DEBUG: Bloque", i, "escrito exitosamente")
	}

	// Serializar el inodo actualizado
	fmt.Println("DEBUG: Serializando inodo actualizado - Posición:", inodePosition)
	err = inode.Serialize(path, inodePosition)
	if err != nil {
		fmt.Println("DEBUG: Error al actualizar inodo:", err)
		return fmt.Errorf("error al actualizar inodo: %v", err)
	}
	fmt.Println("DEBUG: Inodo actualizado exitosamente")

	// Actualizar el superbloque
	fmt.Println("DEBUG: Serializando superbloque - Posición:", partition.Part_start)
	err = sb.Serialize(path, int64(partition.Part_start))
	if err != nil {
		fmt.Println("DEBUG: Error al actualizar superbloque:", err)
		return fmt.Errorf("error al actualizar superbloque: %v", err)
	}
	fmt.Println("DEBUG: Superbloque actualizado exitosamente")

	return nil
}

// allocateBlock asigna un nuevo bloque
func allocateBlock(sb *structures.SuperBlock, path string) (int32, error) {
	fmt.Println("\nDEBUG: Iniciando allocateBlock")

	if sb.S_free_blocks_count == 0 {
		fmt.Println("DEBUG: Error - No hay bloques libres disponibles")
		return -1, errors.New("no hay bloques libres disponibles")
	}

	// Buscar un bloque libre en el bitmap
	fmt.Println("DEBUG: Abriendo archivo para leer bitmap - Path:", path)
	file, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		fmt.Println("DEBUG: Error al abrir archivo:", err)
		return -1, err
	}
	defer file.Close()
	fmt.Println("DEBUG: Archivo abierto exitosamente")

	bitmapSize := int(sb.S_blocks_count / 8)
	if sb.S_blocks_count%8 != 0 {
		bitmapSize++
	}
	fmt.Println("DEBUG: Tamaño del bitmap:", bitmapSize)

	bitmapData := make([]byte, bitmapSize)
	fmt.Println("DEBUG: Posicionando puntero en S_bm_block_start:", sb.S_bm_block_start)
	_, err = file.Seek(int64(sb.S_bm_block_start), 0)
	if err != nil {
		fmt.Println("DEBUG: Error al posicionar puntero:", err)
		return -1, err
	}
	fmt.Println("DEBUG: Leyendo bitmap...")
	_, err = file.Read(bitmapData)
	if err != nil {
		fmt.Println("DEBUG: Error al leer bitmap:", err)
		return -1, err
	}
	fmt.Println("DEBUG: Bitmap leído - Longitud:", len(bitmapData))

	// Buscar un bit libre (0)
	var blockIndex int32 = -1
	for byteIdx := 0; byteIdx < bitmapSize; byteIdx++ {
		for bitIdx := 0; bitIdx < 8; bitIdx++ {
			if int32(byteIdx*8+bitIdx) >= sb.S_blocks_count {
				fmt.Println("DEBUG: Límite de bloques alcanzado")
				break
			}
			if (bitmapData[byteIdx] & (1 << bitIdx)) == 0 {
				blockIndex = int32(byteIdx*8 + bitIdx)
				fmt.Println("DEBUG: Bloque libre encontrado - Índice:", blockIndex)
				bitmapData[byteIdx] |= (1 << bitIdx) // Marcar como ocupado
				fmt.Println("DEBUG: Bit marcado como ocupado en posición:", byteIdx*8+bitIdx)
				break
			}
		}
		if blockIndex != -1 {
			break
		}
	}

	if blockIndex == -1 {
		fmt.Println("DEBUG: Error - No se encontró un bloque libre")
		return -1, errors.New("no se encontró un bloque libre")
	}

	// Escribir el bitmap actualizado
	fmt.Println("DEBUG: Posicionando puntero para escribir bitmap - S_bm_block_start:", sb.S_bm_block_start)
	_, err = file.Seek(int64(sb.S_bm_block_start), 0)
	if err != nil {
		fmt.Println("DEBUG: Error al posicionar puntero:", err)
		return -1, err
	}
	fmt.Println("DEBUG: Escribiendo bitmap actualizado...")
	_, err = file.Write(bitmapData)
	if err != nil {
		fmt.Println("DEBUG: Error al escribir bitmap:", err)
		return -1, err
	}
	fmt.Println("DEBUG: Bitmap escrito exitosamente")

	// Decrementar el conteo de bloques libres
	fmt.Println("DEBUG: Decrementando S_free_blocks_count:", sb.S_free_blocks_count)
	sb.S_free_blocks_count--
	fmt.Println("DEBUG: Nuevo S_free_blocks_count:", sb.S_free_blocks_count)

	return blockIndex, nil
}

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

// REMOVE estructura que representa el comando remove con sus parámetros
type REMOVE struct {
	path string // Ruta del archivo o carpeta a eliminar
}

// ParseRemove parsea el comando remove y devuelve el resultado de la eliminación
func ParseRemove(tokens []string) (string, error) {
	cmd := &REMOVE{}
	args := strings.Join(tokens, " ")
	fmt.Println("DEBUG: Iniciando ParseRemove - Tokens:", tokens)
	fmt.Println("DEBUG: Args combinados:", args)

	// Expresión regular para capturar parámetros válidos
	re := regexp.MustCompile(`(?i)-path="[^"]+"|(?i)-path=[^\s]+`)
	matches := re.FindAllString(args, -1)
	fmt.Println("DEBUG: Coincidencias encontradas:", matches)

	// Validar parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	fmt.Println("DEBUG: Todos los parámetros encontrados:", allParams)
	allowedParams := map[string]bool{
		"-path": true,
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
		}
	}

	if cmd.path == "" {
		fmt.Println("DEBUG: Error - Falta el parámetro requerido: -path")
		return "", fmt.Errorf("faltan parámetros requeridos: -path")
	}

	fmt.Println("DEBUG: Ejecutando commandRemove con cmd:", cmd)
	err := commandRemove(cmd)
	if err != nil {
		fmt.Println("DEBUG: Error en commandRemove:", err)
		return "", err
	}
	fmt.Println("DEBUG: commandRemove ejecutado exitosamente")
	return fmt.Sprintf("REMOVE: Elemento eliminado exitosamente\n-> Ruta: %s", cmd.path), nil
}

// commandRemove elimina el archivo o carpeta especificado
func commandRemove(remove *REMOVE) error {
	fmt.Println("\nDEBUG: Iniciando commandRemove - Path:", remove.path)

	// Verificar sesión activa
	fmt.Println("DEBUG: Verificando sesión activa...")
	if stores.ActiveSession == nil {
		fmt.Println("DEBUG: Error - No hay sesión activa")
		return errors.New("debe iniciar sesión primero para eliminar un elemento")
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
	fmt.Println("DEBUG: Normalizando ruta:", remove.path)
	remove.path = filepath.Clean(remove.path)
	fmt.Println("DEBUG: Ruta normalizada:", remove.path)
	if !strings.HasPrefix(remove.path, "/") {
		fmt.Println("DEBUG: Error - La ruta no es absoluta")
		return errors.New("la ruta debe ser absoluta (comenzar con /)")
	}

	// Separar directorio padre y nombre del elemento
	fmt.Println("DEBUG: Separando directorio padre y nombre del elemento...")
	dir, elementName := filepath.Split(remove.path)
	fmt.Println("DEBUG: Directorio padre:", dir, "Nombre del elemento:", elementName)
	if elementName == "" {
		fmt.Println("DEBUG: Error - Nombre del elemento vacío")
		return errors.New("el nombre del elemento no puede estar vacío")
	}

	// Buscar el directorio padre
	fmt.Println("DEBUG: Buscando directorio padre:", dir)
	parentInode, parentBlock, parentInodeIndex, err := findParentDir(sb, partition, path, dir)
	if err != nil {
		fmt.Println("DEBUG: Error al buscar directorio padre:", err)
		return fmt.Errorf("error al buscar directorio padre: %v", err)
	}
	fmt.Println("DEBUG: Directorio padre encontrado - Inodo:", parentInodeIndex, "I_type:", string(parentInode.I_type[:]), "I_perm:", string(parentInode.I_perm[:]))

	// Verificar permisos de escritura en el directorio padre
	fmt.Println("DEBUG: Verificando permisos de escritura en el directorio padre...")
	if stores.ActiveSession.Username != "root" {
		hasPermission := checkWritePermission(parentInode)
		fmt.Println("DEBUG: Permiso de escritura en directorio padre:", hasPermission)
		if !hasPermission {
			fmt.Println("DEBUG: Error - Permiso denegado en directorio padre")
			return errors.New("permiso denegado: no tiene permisos de escritura en la carpeta padre")
		}
	} else {
		fmt.Println("DEBUG: Usuario es root, permisos no verificados")
	}

	// Buscar el elemento en el directorio padre
	fmt.Println("DEBUG: Buscando elemento en el directorio padre:", elementName)
	var targetInodeIndex int32 = -1
	var targetEntryIndex int
	for i, content := range parentBlock.B_content {
		name := strings.Trim(string(content.B_name[:]), "\x00")
		fmt.Printf("DEBUG: Entrada %d - Nombre: %q, Inodo: %d\n", i, name, content.B_inodo)
		if name == elementName {
			targetInodeIndex = content.B_inodo
			targetEntryIndex = i
			fmt.Println("DEBUG: Elemento encontrado - Inodo:", targetInodeIndex, "Entrada:", targetEntryIndex)
			break
		}
	}

	if targetInodeIndex == -1 {
		fmt.Println("DEBUG: Error - Elemento no encontrado:", elementName)
		return fmt.Errorf("el elemento %s no existe en la ruta especificada", elementName)
	}

	// Verificar permisos de escritura en el elemento y su contenido (si es carpeta)
	fmt.Println("DEBUG: Verificando permisos de escritura en el elemento y su contenido...")
	canDelete, err := checkElementPermissions(sb, path, targetInodeIndex)
	if err != nil {
		fmt.Println("DEBUG: Error al verificar permisos:", err)
		return fmt.Errorf("error al verificar permisos: %v", err)
	}
	if !canDelete {
		fmt.Println("DEBUG: Error - No se puede eliminar por falta de permisos")
		return errors.New("no se puede eliminar: falta permiso de escritura en el elemento o su contenido")
	}
	fmt.Println("DEBUG: Permisos verificados - Se puede eliminar")

	// Eliminar el elemento (archivo o carpeta)
	fmt.Println("DEBUG: Eliminando elemento con inodo:", targetInodeIndex)
	err = deleteElement(sb, path, targetInodeIndex)
	if err != nil {
		fmt.Println("DEBUG: Error al eliminar elemento:", err)
		return fmt.Errorf("error al eliminar elemento: %v", err)
	}
	fmt.Println("DEBUG: Elemento eliminado exitosamente")

	// Eliminar la entrada del directorio padre
	fmt.Println("DEBUG: Eliminando entrada del directorio padre - Entrada:", targetEntryIndex)
	parentBlock.B_content[targetEntryIndex] = structures.FolderContent{}
	fmt.Println("DEBUG: Serializando bloque del padre - Posición:")
	fmt.Println(int64(sb.S_block_start) + int64(parentInode.I_block[0])*int64(sb.S_block_size))
	err = parentBlock.Serialize(path, int64(sb.S_block_start)+int64(parentInode.I_block[0])*int64(sb.S_block_size))
	if err != nil {
		fmt.Println("DEBUG: Error al serializar bloque del padre:", err)
		return fmt.Errorf("error al actualizar bloque del padre: %v", err)
	}
	fmt.Println("DEBUG: Bloque del padre actualizado exitosamente")

	// Actualizar el superbloque
	fmt.Println("DEBUG: Serializando superbloque - Posición:", partition.Part_start)
	err = sb.Serialize(path, int64(partition.Part_start))
	if err != nil {
		fmt.Println("DEBUG: Error al serializar superbloque:", err)
		return fmt.Errorf("error al actualizar superbloque: %v", err)
	}
	fmt.Println("DEBUG: Superbloque actualizado exitosamente")

	return nil
}

// findParentDir busca el directorio padre de la ruta especificada
func findParentDir(sb *structures.SuperBlock, partition *structures.Partition, path string, dir string) (*structures.Inode, *structures.FolderBlock, int32, error) {
	fmt.Println("\nDEBUG: Iniciando findParentDir - Directorio:", dir)

	if dir == "/" {
		fmt.Println("DEBUG: Directorio es raíz (/)")
		inode := &structures.Inode{}
		fmt.Println("DEBUG: Deserializando inodo raíz - Posición:", sb.S_inode_start)
		err := inode.Deserialize(path, int64(sb.S_inode_start))
		if err != nil {
			fmt.Println("DEBUG: Error al deserializar inodo raíz:", err)
			return nil, nil, 0, err
		}
		fmt.Println("DEBUG: Inodo raíz deserializado - I_block[0]:", inode.I_block[0])

		block := &structures.FolderBlock{}
		// Usar directamente I_block[0] como posición absoluta
		blockPosition := int64(inode.I_block[0])
		fmt.Println("DEBUG: Deserializando bloque raíz - Posición:", blockPosition)
		if blockPosition >= int64(partition.Part_start)+int64(partition.Part_size) {
			fmt.Println("DEBUG: Error - Posición del bloque excede el tamaño de la partición:", blockPosition)
			return nil, nil, 0, fmt.Errorf("posición del bloque fuera de límites: %d", blockPosition)
		}
		err = block.Deserialize(path, blockPosition)
		if err != nil {
			fmt.Println("DEBUG: Error al deserializar bloque raíz:", err)
			return nil, nil, 0, err
		}
		fmt.Println("DEBUG: Bloque raíz deserializado - Entrada 0:", string(block.B_content[0].B_name[:]), "Inodo:", block.B_content[0].B_inodo)
		return inode, block, 0, nil
	}

	fmt.Println("DEBUG: Dividiendo directorio en partes:", dir)
	parts := strings.Split(strings.Trim(dir, "/"), "/")
	fmt.Println("DEBUG: Partes del directorio:", parts)
	currentInodeIndex := int32(0)
	currentInode := &structures.Inode{}
	inodePosition := int64(sb.S_inode_start) + int64(currentInodeIndex)*int64(sb.S_inode_size)
	fmt.Println("DEBUG: Deserializando inodo inicial - Posición:", inodePosition)
	err := currentInode.Deserialize(path, inodePosition)
	if err != nil {
		fmt.Println("DEBUG: Error al deserializar inodo inicial:", err)
		return nil, nil, 0, err
	}
	fmt.Println("DEBUG: Inodo inicial deserializado - I_block[0]:", currentInode.I_block[0])

	for i, part := range parts {
		if part == "" {
			fmt.Println("DEBUG: Parte vacía, saltando - Índice:", i)
			continue
		}
		fmt.Println("DEBUG: Buscando parte:", part)
		block := &structures.FolderBlock{}
		// Usar directamente I_block[0] como posición absoluta
		blockPosition := int64(currentInode.I_block[0])
		fmt.Println("DEBUG: Deserializando bloque - Posición:", blockPosition)
		if blockPosition >= int64(partition.Part_start)+int64(partition.Part_size) {
			fmt.Println("DEBUG: Error - Posición del bloque excede el tamaño de la partición:", blockPosition)
			return nil, nil, 0, fmt.Errorf("posición del bloque fuera de límites: %d", blockPosition)
		}
		err := block.Deserialize(path, blockPosition)
		if err != nil {
			fmt.Println("DEBUG: Error al deserializar bloque:", err)
			return nil, nil, 0, err
		}
		fmt.Println("DEBUG: Bloque deserializado")

		found := false
		for j, content := range block.B_content {
			name := strings.Trim(string(content.B_name[:]), "\x00")
			fmt.Printf("DEBUG: Entrada %d - Nombre: %q, Inodo: %d\n", j, name, content.B_inodo)
			if name == part {
				currentInodeIndex = content.B_inodo
				fmt.Println("DEBUG: Parte encontrada - Nuevo inodo:", currentInodeIndex)
				currentInode = &structures.Inode{}
				inodePosition = int64(sb.S_inode_start) + int64(currentInodeIndex)*int64(sb.S_inode_size)
				fmt.Println("DEBUG: Deserializando nuevo inodo - Posición:", inodePosition)
				err = currentInode.Deserialize(path, inodePosition)
				if err != nil {
					fmt.Println("DEBUG: Error al deserializar nuevo inodo:", err)
					return nil, nil, 0, err
				}
				fmt.Println("DEBUG: Nuevo inodo deserializado - I_block[0]:", currentInode.I_block[0])
				found = true
				break
			}
		}
		if !found {
			fmt.Println("DEBUG: Error - Directorio no encontrado:", part)
			return nil, nil, 0, fmt.Errorf("directorio padre %s no encontrado", dir)
		}
	}

	block := &structures.FolderBlock{}
	// Usar directamente I_block[0] como posición absoluta
	var blockPosition = int64(currentInode.I_block[0])
	fmt.Println("DEBUG: Deserializando bloque final del padre - Posición:", blockPosition)
	if blockPosition >= int64(partition.Part_start)+int64(partition.Part_size) {
		fmt.Println("DEBUG: Error - Posición del bloque excede el tamaño de la partición:", blockPosition)
		return nil, nil, 0, fmt.Errorf("posición del bloque fuera de límites: %d", blockPosition)
	}
	err = block.Deserialize(path, blockPosition)
	if err != nil {
		fmt.Println("DEBUG: Error al deserializar bloque final:", err)
		return nil, nil, 0, err
	}
	fmt.Println("DEBUG: Bloque final del padre deserializado")
	return currentInode, block, currentInodeIndex, nil
}

// checkElementPermissions verifica recursivamente los permisos de escritura
func checkElementPermissions(sb *structures.SuperBlock, path string, inodeIndex int32) (bool, error) {
	fmt.Println("\nDEBUG: Iniciando checkElementPermissions - Inodo:", inodeIndex)

	inode := &structures.Inode{}
	inodePosition := int64(sb.S_inode_start) + int64(inodeIndex)*int64(sb.S_inode_size)
	fmt.Println("DEBUG: Deserializando inodo - Posición:", inodePosition)
	err := inode.Deserialize(path, inodePosition)
	if err != nil {
		fmt.Println("DEBUG: Error al deserializar inodo:", err)
		return false, err
	}
	fmt.Println("DEBUG: Inodo deserializado - I_type:", string(inode.I_type[:]), "I_perm:", string(inode.I_perm[:]))

	// Verificar permisos de escritura en el elemento
	fmt.Println("DEBUG: Verificando permisos de escritura en el elemento...")
	if stores.ActiveSession.Username != "root" {
		hasPermission := checkWritePermission(inode)
		fmt.Println("DEBUG: Permiso de escritura en el elemento:", hasPermission)
		if !hasPermission {
			fmt.Println("DEBUG: Permiso denegado en el elemento")
			return false, nil
		}
	} else {
		fmt.Println("DEBUG: Usuario es root, permisos no verificados")
	}

	// Si es una carpeta, verificar permisos en su contenido
	if inode.I_type[0] == '0' { // '0' para carpetas (corregido)
		fmt.Println("DEBUG: El elemento es una carpeta, verificando contenido...")
		for i, blockIndex := range inode.I_block {
			if blockIndex == -1 {
				fmt.Println("DEBUG: Bloque", i, "no existe, saltando")
				continue
			}
			fmt.Println("DEBUG: Procesando bloque - Índice:", blockIndex)
			block := &structures.FolderBlock{}
			blockPosition := int64(blockIndex) // Usar directamente blockIndex como posición absoluta
			fmt.Println("DEBUG: Deserializando bloque - Posición:", blockPosition)
			err := block.Deserialize(path, blockPosition)
			if err != nil {
				fmt.Println("DEBUG: Error al deserializar bloque:", err)
				return false, err
			}
			fmt.Println("DEBUG: Bloque deserializado")

			for j, content := range block.B_content {
				name := strings.Trim(string(content.B_name[:]), "\x00")
				fmt.Printf("DEBUG: Entrada %d - Nombre: %q, Inodo: %d\n", j, name, content.B_inodo)
				if name == "." || name == ".." || content.B_inodo == -1 {
					fmt.Println("DEBUG: Entrada ignorada (., .. o inodo -1)")
					continue
				}
				fmt.Println("DEBUG: Verificando permisos recursivamente - Inodo:", content.B_inodo)
				canDelete, err := checkElementPermissions(sb, path, content.B_inodo)
				if err != nil {
					fmt.Println("DEBUG: Error al verificar permisos recursivamente:", err)
					return false, err
				}
				if !canDelete {
					fmt.Println("DEBUG: Permiso denegado en contenido del elemento")
					return false, nil
				}
			}
		}
	} else {
		fmt.Println("DEBUG: El elemento no es una carpeta, no se verifica contenido")
	}

	fmt.Println("DEBUG: Todos los permisos verificados - Se puede eliminar")
	return true, nil
}

// deleteElement elimina un elemento (archivo o carpeta) y su contenido
func deleteElement(sb *structures.SuperBlock, path string, inodeIndex int32) error {
	fmt.Println("\nDEBUG: Iniciando deleteElement - Inodo:", inodeIndex)

	inode := &structures.Inode{}
	inodePosition := int64(sb.S_inode_start) + int64(inodeIndex)*int64(sb.S_inode_size)
	fmt.Println("DEBUG: Deserializando inodo - Posición:", inodePosition)
	err := inode.Deserialize(path, inodePosition)
	if err != nil {
		fmt.Println("DEBUG: Error al deserializar inodo:", err)
		return err
	}
	fmt.Println("DEBUG: Inodo deserializado - I_type:", string(inode.I_type[:]), "I_block:", inode.I_block)

	// Si es una carpeta, eliminar su contenido recursivamente
	if inode.I_type[0] == '0' { // '0' para carpetas (corregido)
		fmt.Println("DEBUG: El elemento es una carpeta, eliminando contenido...")
		for i, blockIndex := range inode.I_block {
			if blockIndex == -1 {
				fmt.Println("DEBUG: Bloque", i, "no existe, saltando")
				continue
			}
			fmt.Println("DEBUG: Procesando bloque - Índice:", blockIndex)
			block := &structures.FolderBlock{}
			blockPosition := int64(blockIndex) // Usar directamente blockIndex como posición absoluta
			fmt.Println("DEBUG: Deserializando bloque - Posición:", blockPosition)
			err := block.Deserialize(path, blockPosition)
			if err != nil {
				fmt.Println("DEBUG: Error al deserializar bloque:", err)
				return err
			}
			fmt.Println("DEBUG: Bloque deserializado")

			for j, content := range block.B_content {
				name := strings.Trim(string(content.B_name[:]), "\x00")
				fmt.Printf("DEBUG: Entrada %d - Nombre: %q, Inodo: %d\n", j, name, content.B_inodo)
				if name == "." || name == ".." || content.B_inodo == -1 {
					fmt.Println("DEBUG: Entrada ignorada (., .. o inodo -1)")
					continue
				}
				fmt.Println("DEBUG: Eliminando elemento recursivamente - Inodo:", content.B_inodo)
				err = deleteElement(sb, path, content.B_inodo)
				if err != nil {
					fmt.Println("DEBUG: Error al eliminar elemento recursivamente:", err)
					return err
				}
				fmt.Println("DEBUG: Limpiando entrada", j)
				block.B_content[j] = structures.FolderContent{}
			}
			fmt.Println("DEBUG: Serializando bloque actualizado - Posición:", blockPosition)
			err = block.Serialize(path, blockPosition)
			if err != nil {
				fmt.Println("DEBUG: Error al serializar bloque:", err)
				return err
			}
			fmt.Println("DEBUG: Bloque actualizado exitosamente")
		}
	} else {
		fmt.Println("DEBUG: El elemento no es una carpeta, no se elimina contenido")
	}

	// Liberar los bloques del inodo
	fmt.Println("DEBUG: Liberando bloques del inodo...")
	for i, blockIndex := range inode.I_block {
		if blockIndex != -1 {
			fmt.Println("DEBUG: Liberando bloque - Índice:", blockIndex)
			err := freeBlock(sb, path, blockIndex)
			if err != nil {
				fmt.Println("DEBUG: Error al liberar bloque:", err)
				return err
			}
			inode.I_block[i] = -1
			fmt.Println("DEBUG: Bloque liberado - Nuevo valor de I_block[", i, "]:", inode.I_block[i])
		}
	}

	// Serializar el inodo actualizado
	fmt.Println("DEBUG: Serializando inodo actualizado - Posición:", inodePosition)
	err = inode.Serialize(path, inodePosition)
	if err != nil {
		fmt.Println("DEBUG: Error al serializar inodo:", err)
		return err
	}
	fmt.Println("DEBUG: Inodo actualizado exitosamente")

	// Liberar el inodo
	fmt.Println("DEBUG: Liberando inodo - Índice:", inodeIndex)
	err = freeInode(sb, path, inodeIndex)
	if err != nil {
		fmt.Println("DEBUG: Error al liberar inodo:", err)
		return err
	}
	fmt.Println("DEBUG: Inodo liberado exitosamente")

	return nil
}

// freeInode libera un inodo
func freeInode(sb *structures.SuperBlock, path string, inodeIndex int32) error {
	fmt.Println("\nDEBUG: Iniciando freeInode - Inodo:", inodeIndex)

	// Incrementar el conteo de inodos libres
	sb.S_free_inodes_count++
	fmt.Println("DEBUG: Incrementado S_free_inodes_count:", sb.S_free_inodes_count)

	// Actualizar el bitmap de inodos
	bitmapOffset := int64(sb.S_bm_inode_start) + int64(inodeIndex/8)
	bitPosition := inodeIndex % 8
	fmt.Println("DEBUG: Actualizando bitmap de inodos - Offset:", bitmapOffset, "Bit Position:", bitPosition)

	fmt.Println("DEBUG: Abriendo archivo para escritura:", path)
	file, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		fmt.Println("DEBUG: Error al abrir archivo:", err)
		return err
	}
	defer file.Close()
	fmt.Println("DEBUG: Archivo abierto exitosamente")

	// Leer el byte del bitmap
	fmt.Println("DEBUG: Posicionando puntero - Offset:", bitmapOffset)
	_, err = file.Seek(bitmapOffset, 0)
	if err != nil {
		fmt.Println("DEBUG: Error al posicionar puntero:", err)
		return err
	}
	byteData := make([]byte, 1)
	fmt.Println("DEBUG: Leyendo byte del bitmap...")
	_, err = file.Read(byteData)
	if err != nil {
		fmt.Println("DEBUG: Error al leer byte:", err)
		return err
	}
	fmt.Println("DEBUG: Byte leído:", byteData[0])

	// Limpiar el bit correspondiente (marcar como libre: 0)
	byteData[0] &^= (1 << bitPosition)
	fmt.Println("DEBUG: Byte actualizado (bit limpiado):", byteData[0])

	// Escribir el byte actualizado
	fmt.Println("DEBUG: Posicionando puntero para escritura - Offset:", bitmapOffset)
	_, err = file.Seek(bitmapOffset, 0)
	if err != nil {
		fmt.Println("DEBUG: Error al posicionar puntero:", err)
		return err
	}
	fmt.Println("DEBUG: Escribiendo byte actualizado...")
	_, err = file.Write(byteData)
	if err != nil {
		fmt.Println("DEBUG: Error al escribir byte:", err)
		return err
	}
	fmt.Println("DEBUG: Byte escrito exitosamente")

	return nil
}

// freeBlock libera un bloque
func freeBlock(sb *structures.SuperBlock, path string, blockIndex int32) error {
	fmt.Println("\nDEBUG: Iniciando freeBlock - Bloque:", blockIndex)

	// Incrementar el conteo de bloques libres
	sb.S_free_blocks_count++
	fmt.Println("DEBUG: Incrementado S_free_blocks_count:", sb.S_free_blocks_count)

	// Actualizar el bitmap de bloques
	bitmapOffset := int64(sb.S_bm_block_start) + int64(blockIndex/8)
	bitPosition := blockIndex % 8
	fmt.Println("DEBUG: Actualizando bitmap de bloques - Offset:", bitmapOffset, "Bit Position:", bitPosition)

	fmt.Println("DEBUG: Abriendo archivo para escritura:", path)
	file, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		fmt.Println("DEBUG: Error al abrir archivo:", err)
		return err
	}
	defer file.Close()
	fmt.Println("DEBUG: Archivo abierto exitosamente")

	// Leer el byte del bitmap
	fmt.Println("DEBUG: Posicionando puntero - Offset:", bitmapOffset)
	_, err = file.Seek(bitmapOffset, 0)
	if err != nil {
		fmt.Println("DEBUG: Error al posicionar puntero:", err)
		return err
	}
	byteData := make([]byte, 1)
	fmt.Println("DEBUG: Leyendo byte del bitmap...")
	_, err = file.Read(byteData)
	if err != nil {
		fmt.Println("DEBUG: Error al leer byte:", err)
		return err
	}
	fmt.Println("DEBUG: Byte leído:", byteData[0])

	// Limpiar el bit correspondiente (marcar como libre: 0)
	byteData[0] &^= (1 << bitPosition)
	fmt.Println("DEBUG: Byte actualizado (bit limpiado):", byteData[0])

	// Escribir el byte actualizado
	fmt.Println("DEBUG: Posicionando puntero para escritura - Offset:", bitmapOffset)
	_, err = file.Seek(bitmapOffset, 0)
	if err != nil {
		fmt.Println("DEBUG: Error al posicionar puntero:", err)
		return err
	}
	fmt.Println("DEBUG: Escribiendo byte actualizado...")
	_, err = file.Write(byteData)
	if err != nil {
		fmt.Println("DEBUG: Error al escribir byte:", err)
		return err
	}
	fmt.Println("DEBUG: Byte escrito exitosamente")

	return nil
}

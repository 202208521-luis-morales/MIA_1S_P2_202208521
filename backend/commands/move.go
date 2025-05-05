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

// MOVE estructura que representa el comando move con sus parámetros
type MOVE struct {
	path    string // Ruta del archivo/carpeta a mover
	destino string // Ruta del destino (debe ser una carpeta)
}

// ParseMove parsea el comando move y devuelve el resultado del movimiento
func ParseMove(tokens []string) (string, error) {
	cmd := &MOVE{}
	args := strings.Join(tokens, " ")
	fmt.Println("DEBUG: Iniciando ParseMove - Tokens:", tokens)
	fmt.Println("DEBUG: Args combinados:", args)

	re := regexp.MustCompile(`(?i)-path="[^"]+"|(?i)-path=[^\s]+|(?i)-destino="[^"]+"|(?i)-destino=[^\s]+`)
	matches := re.FindAllString(args, -1)
	fmt.Println("DEBUG: Coincidencias encontradas:", matches)

	// Validar parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	fmt.Println("DEBUG: Todos los parámetros encontrados:", allParams)
	allowedParams := map[string]bool{
		"-path":    true,
		"-destino": true,
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
		case "-destino":
			if value == "" {
				fmt.Println("DEBUG: Error - La ruta de destino no puede estar vacía")
				return "", errors.New("la ruta de destino no puede estar vacía")
			}
			cmd.destino = value
			fmt.Println("DEBUG: Ruta de destino establecida:", cmd.destino)
		}
	}

	if cmd.path == "" || cmd.destino == "" {
		fmt.Println("DEBUG: Error - Falta alguno de los parámetros requeridos: -path o -destino")
		return "", fmt.Errorf("faltan parámetros requeridos: -path y -destino son obligatorios")
	}

	fmt.Println("DEBUG: Ejecutando commandMove con cmd:", cmd)
	err := commandMove(cmd)
	if err != nil {
		fmt.Println("DEBUG: Error en commandMove:", err)
		return "", err
	}
	fmt.Println("DEBUG: commandMove ejecutado exitosamente")
	return fmt.Sprintf("MOVE: Elemento movido exitosamente\n-> Origen: %s\n-> Destino: %s", cmd.path, cmd.destino), nil
}

// commandMove realiza el movimiento del archivo o carpeta
func commandMove(cmd *MOVE) error {
	fmt.Println("\nDEBUG: Iniciando commandMove - Path:", cmd.path, "Destino:", cmd.destino)

	// Verificar sesión activa
	fmt.Println("DEBUG: Verificando sesión activa...")
	if stores.ActiveSession == nil {
		fmt.Println("DEBUG: Error - No hay sesión activa")
		return errors.New("debe iniciar sesión primero para mover un elemento")
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

	// Normalizar las rutas
	fmt.Println("DEBUG: Normalizando rutas - Path:", cmd.path, "Destino:", cmd.destino)
	cmd.path = filepath.Clean(cmd.path)
	cmd.destino = filepath.Clean(cmd.destino)
	fmt.Println("DEBUG: Rutas normalizadas - Path:", cmd.path, "Destino:", cmd.destino)
	if !strings.HasPrefix(cmd.path, "/") || !strings.HasPrefix(cmd.destino, "/") {
		fmt.Println("DEBUG: Error - Las rutas no son absolutas")
		return errors.New("las rutas deben ser absolutas (comenzar con /)")
	}

	// Separar directorio padre y nombre del elemento a mover
	fmt.Println("DEBUG: Separando directorio padre y nombre del origen...")
	srcDir, srcName := filepath.Split(cmd.path)
	fmt.Println("DEBUG: Directorio padre origen:", srcDir, "Nombre del elemento:", srcName)
	if srcName == "" {
		fmt.Println("DEBUG: Error - Nombre del elemento a mover vacío")
		return errors.New("el nombre del elemento a mover no puede estar vacío")
	}

	// Verificar que el elemento fuente exista
	fmt.Println("DEBUG: Buscando directorio padre del origen:", srcDir)
	srcParentInode, srcParentBlock, _, err := findParentDir(sb, partition, path, srcDir)
	if err != nil {
		fmt.Println("DEBUG: Error al buscar directorio padre del origen:", err)
		return fmt.Errorf("error al buscar directorio padre del origen: %v", err)
	}
	fmt.Println("DEBUG: Directorio padre del origen encontrado - Entrada 0:", string(srcParentBlock.B_content[0].B_name[:]))

	var srcInodeIndex int32 = -1
	var srcEntryIndex int
	for i, content := range srcParentBlock.B_content {
		name := strings.Trim(string(content.B_name[:]), "\x00")
		fmt.Printf("DEBUG: Entrada %d - Nombre: %q, Inodo: %d\n", i, name, content.B_inodo)
		if name == srcName {
			srcInodeIndex = content.B_inodo
			srcEntryIndex = i
			fmt.Println("DEBUG: Elemento origen encontrado - Inodo:", srcInodeIndex, "Índice de entrada:", srcEntryIndex)
			break
		}
	}

	if srcInodeIndex == -1 {
		fmt.Println("DEBUG: Error - Elemento no encontrado:", srcName)
		return fmt.Errorf("el elemento %s no existe en la ruta especificada", cmd.path)
	}

	// Leer el inodo del elemento fuente
	fmt.Println("DEBUG: Leyendo inodo del origen - Inodo:", srcInodeIndex)
	srcInode := &structures.Inode{}
	inodePosition := int64(sb.S_inode_start) + int64(srcInodeIndex)*int64(sb.S_inode_size)
	fmt.Println("DEBUG: Deserializando inodo - Posición:", inodePosition)
	err = srcInode.Deserialize(path, inodePosition)
	if err != nil {
		fmt.Println("DEBUG: Error al leer inodo del origen:", err)
		return fmt.Errorf("error al leer inodo del origen: %v", err)
	}
	fmt.Println("DEBUG: Inodo deserializado - I_type:", string(srcInode.I_type[:]), "I_size:", srcInode.I_size)

	// Verificar permisos de escritura en el elemento fuente
	fmt.Println("DEBUG: Verificando permisos de escritura en el origen...")
	if stores.ActiveSession.Username != "root" {
		hasWritePermission := checkWritePermission(srcInode)
		fmt.Println("DEBUG: Permiso de escritura:", hasWritePermission)
		if !hasWritePermission {
			fmt.Println("DEBUG: Error - Permiso de escritura denegado en el origen")
			return fmt.Errorf("permiso denegado: no tiene permisos de escritura en %s", cmd.path)
		}
	} else {
		fmt.Println("DEBUG: Usuario es root, permisos de escritura no verificados")
	}

	// Verificar que el destino sea una carpeta existente
	fmt.Println("DEBUG: Verificando que el destino sea una carpeta existente:", cmd.destino)
	destParentInode, destParentBlock, destParentInodeIndex, err := findParentDir(sb, partition, path, cmd.destino)
	if err != nil {
		fmt.Println("DEBUG: Error - Carpeta destino no encontrada:", err)
		return fmt.Errorf("error: la carpeta destino %s no existe", cmd.destino)
	}
	fmt.Println("DEBUG: Directorio destino encontrado - Entrada 0:", string(destParentBlock.B_content[0].B_name[:]))

	// Verificar que el destino sea una carpeta (I_type == '0')
	if destParentInode.I_type[0] != '0' {
		fmt.Println("DEBUG: Error - El destino no es una carpeta")
		return fmt.Errorf("error: el destino %s debe ser una carpeta", cmd.destino)
	}

	// Verificar permisos de escritura en la carpeta destino
	fmt.Println("DEBUG: Verificando permisos de escritura en el destino...")
	if stores.ActiveSession.Username != "root" {
		hasWritePermission := checkWritePermission(destParentInode)
		fmt.Println("DEBUG: Permiso de escritura:", hasWritePermission)
		if !hasWritePermission {
			fmt.Println("DEBUG: Error - Permiso de escritura denegado en el destino")
			return fmt.Errorf("permiso denegado: no tiene permisos de escritura en %s", cmd.destino)
		}
	} else {
		fmt.Println("DEBUG: Usuario es root, permisos de escritura no verificados")
	}

	// Verificar si ya existe un elemento con el mismo nombre en el destino
	destName := srcName
	fmt.Println("DEBUG: Verificando si el nombre ya existe en el destino:", destName)
	for i, content := range destParentBlock.B_content {
		name := strings.Trim(string(content.B_name[:]), "\x00")
		fmt.Printf("DEBUG: Entrada %d - Nombre: %q, Inodo: %d\n", i, name, content.B_inodo)
		if name == destName {
			fmt.Println("DEBUG: Error - Nombre ya existe en el destino")
			return fmt.Errorf("ya existe un elemento con el nombre %s en %s", destName, cmd.destino)
		}
	}

	// Si el elemento es una carpeta, actualizar las entradas "." y ".."
	if srcInode.I_type[0] == '0' { // Carpeta (corregido)
		fmt.Println("DEBUG: El elemento es una carpeta, actualizando entradas . y ..")
		for i, blockIndex := range srcInode.I_block {
			if blockIndex == -1 {
				fmt.Println("DEBUG: Bloque", i, "no existe, saltando")
				continue
			}
			fmt.Println("DEBUG: Procesando bloque - Índice:", blockIndex)
			block := &structures.FolderBlock{}
			err := block.Deserialize(path, int64(blockIndex)) // Usar posición absoluta
			if err != nil {
				fmt.Println("DEBUG: Error al leer bloque de carpeta:", err)
				return fmt.Errorf("error al leer bloque de carpeta: %v", err)
			}
			fmt.Println("DEBUG: Bloque de carpeta leído - Entrada 0:", string(block.B_content[0].B_name[:]))
			for j, content := range block.B_content {
				name := strings.Trim(string(content.B_name[:]), "\x00")
				fmt.Printf("DEBUG: Entrada %d - Nombre: %q, Inodo: %d\n", j, name, content.B_inodo)
				if name == ".." {
					block.B_content[j].B_inodo = destParentInodeIndex
					fmt.Println("DEBUG: Entrada .. actualizada - Nuevo inodo padre:", destParentInodeIndex)
					break
				}
			}
			fmt.Println("DEBUG: Serializando bloque actualizado - Posición:", blockIndex)
			err = block.Serialize(path, int64(blockIndex)) // Usar posición absoluta
			if err != nil {
				fmt.Println("DEBUG: Error al actualizar bloque de carpeta:", err)
				return fmt.Errorf("error al actualizar bloque de carpeta: %v", err)
			}
			fmt.Println("DEBUG: Bloque de carpeta actualizado exitosamente")
		}
	} else {
		fmt.Println("DEBUG: El elemento es un archivo, no se actualizan entradas . y ..")
	}

	// Eliminar la referencia del directorio padre origen
	fmt.Println("DEBUG: Eliminando referencia del directorio padre origen - Índice:", srcEntryIndex)
	srcParentBlock.B_content[srcEntryIndex] = structures.FolderContent{}
	fmt.Println("DEBUG: Serializando bloque del padre origen - Posición:", srcParentInode.I_block[0])
	err = srcParentBlock.Serialize(path, int64(srcParentInode.I_block[0])) // Usar posición absoluta
	if err != nil {
		fmt.Println("DEBUG: Error al actualizar bloque del origen:", err)
		return fmt.Errorf("error al actualizar bloque del origen: %v", err)
	}
	fmt.Println("DEBUG: Bloque del padre origen actualizado exitosamente")

	// Agregar la nueva entrada al directorio destino
	fmt.Println("DEBUG: Agregando nueva entrada al directorio destino...")
	newEntry := structures.FolderContent{
		B_inodo: srcInodeIndex,
	}
	copy(newEntry.B_name[:], []byte(destName))
	for i := range destParentBlock.B_content {
		if destParentBlock.B_content[i].B_inodo == -1 {
			destParentBlock.B_content[i] = newEntry
			fmt.Println("DEBUG: Entrada agregada - Índice:", i)
			break
		}
		if i == len(destParentBlock.B_content)-1 {
			fmt.Println("DEBUG: Error - No hay espacio en el bloque del directorio destino")
			return errors.New("no hay espacio en el bloque del directorio destino para agregar la nueva entrada")
		}
	}

	// Serializar el bloque del directorio destino
	fmt.Println("DEBUG: Serializando bloque del directorio destino - Posición:", destParentInode.I_block[0])
	err = destParentBlock.Serialize(path, int64(destParentInode.I_block[0])) // Usar posición absoluta
	if err != nil {
		fmt.Println("DEBUG: Error al actualizar bloque del destino:", err)
		return fmt.Errorf("error al actualizar bloque del destino: %v", err)
	}
	fmt.Println("DEBUG: Bloque del directorio destino actualizado exitosamente")

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

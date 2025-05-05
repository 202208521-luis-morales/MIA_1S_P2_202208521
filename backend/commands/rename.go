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

// RENAME estructura que representa el comando rename con sus parámetros
type RENAME struct {
	path string // Ruta del archivo/carpeta a renombrar
	name string // Nuevo nombre del archivo/carpeta
}

// ParseRename parsea el comando rename y devuelve el resultado del renombrado
func ParseRename(tokens []string) (string, error) {
	cmd := &RENAME{}
	args := strings.Join(tokens, " ")
	fmt.Println("DEBUG: Iniciando ParseRename - Tokens:", tokens)
	fmt.Println("DEBUG: Args combinados:", args)

	re := regexp.MustCompile(`(?i)-path="[^"]+"|(?i)-path=[^\s]+|(?i)-name="[^"]+"|(?i)-name=[^\s]+`)
	matches := re.FindAllString(args, -1)
	fmt.Println("DEBUG: Coincidencias encontradas:", matches)

	// Validar parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	fmt.Println("DEBUG: Todos los parámetros encontrados:", allParams)
	allowedParams := map[string]bool{
		"-path": true,
		"-name": true,
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
		case "-name":
			if value == "" {
				fmt.Println("DEBUG: Error - El nombre no puede estar vacío")
				return "", errors.New("el nombre no puede estar vacío")
			}
			cmd.name = value
			fmt.Println("DEBUG: Nombre establecido:", cmd.name)
		}
	}

	if cmd.path == "" || cmd.name == "" {
		fmt.Println("DEBUG: Error - Falta alguno de los parámetros requeridos: -path o -name")
		return "", fmt.Errorf("faltan parámetros requeridos: -path y -name son obligatorios")
	}

	fmt.Println("DEBUG: Ejecutando commandRename con cmd:", cmd)
	err := commandRename(cmd)
	if err != nil {
		fmt.Println("DEBUG: Error en commandRename:", err)
		return "", err
	}
	fmt.Println("DEBUG: commandRename ejecutado exitosamente")
	return fmt.Sprintf("RENAME: Elemento renombrado exitosamente\n-> Ruta: %s\n-> Nuevo nombre: %s", cmd.path, cmd.name), nil
}

// commandRename cambia el nombre de un archivo o carpeta
func commandRename(rename *RENAME) error {
	fmt.Println("\nDEBUG: Iniciando commandRename - Path:", rename.path, "Name:", rename.name)

	// Verificar sesión activa
	fmt.Println("DEBUG: Verificando sesión activa...")
	if stores.ActiveSession == nil {
		fmt.Println("DEBUG: Error - No hay sesión activa")
		return errors.New("debe iniciar sesión primero para renombrar un elemento")
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
	fmt.Println("DEBUG: Normalizando ruta:", rename.path)
	rename.path = filepath.Clean(rename.path)
	fmt.Println("DEBUG: Ruta normalizada:", rename.path)
	if !strings.HasPrefix(rename.path, "/") {
		fmt.Println("DEBUG: Error - La ruta no es absoluta")
		return errors.New("la ruta debe ser absoluta (comenzar con /)")
	}

	// Separar directorio padre y nombre del elemento
	fmt.Println("DEBUG: Separando directorio padre y nombre del elemento...")
	dir, elementName := filepath.Split(rename.path)
	fmt.Println("DEBUG: Directorio padre:", dir, "Nombre del elemento:", elementName)
	if elementName == "" {
		fmt.Println("DEBUG: Error - Nombre del elemento vacío")
		return errors.New("el nombre del elemento no puede estar vacío")
	}

	// Validar el nuevo nombre
	fmt.Println("DEBUG: Validando nuevo nombre:", rename.name)
	if len(rename.name) > 13 {
		fmt.Println("DEBUG: Error - Nuevo nombre demasiado largo")
		return errors.New("el nuevo nombre es demasiado largo (máximo 12 caracteres)")
	}

	// Buscar el directorio padre
	fmt.Println("DEBUG: Buscando directorio padre:", dir)
	parentInode, parentBlock, _, err := findParentDir(sb, partition, path, dir)
	if err != nil {
		fmt.Println("DEBUG: Error al buscar directorio padre:", err)
		return fmt.Errorf("error al buscar directorio padre: %v", err)
	}
	fmt.Println("DEBUG: Directorio padre encontrado - Entrada 0:", string(parentBlock.B_content[0].B_name[:]))

	// Buscar el elemento en el directorio padre y verificar si el nuevo nombre ya existe
	fmt.Println("DEBUG: Buscando elemento en el directorio padre:", elementName)
	var targetInodeIndex int32 = -1
	var targetEntryIndex int
	nameExists := false
	for i, content := range parentBlock.B_content {
		name := strings.Trim(string(content.B_name[:]), "\x00")
		fmt.Printf("DEBUG: Entrada %d - Nombre: %q, Inodo: %d\n", i, name, content.B_inodo)
		if name == elementName {
			targetInodeIndex = content.B_inodo
			targetEntryIndex = i
			fmt.Println("DEBUG: Elemento encontrado - Inodo:", targetInodeIndex, "Índice de entrada:", targetEntryIndex)
		}
		if name == rename.name {
			nameExists = true
			fmt.Println("DEBUG: Nuevo nombre ya existe:", rename.name)
		}
	}

	if targetInodeIndex == -1 {
		fmt.Println("DEBUG: Error - Elemento no encontrado:", elementName)
		return fmt.Errorf("el elemento %s no existe en la ruta especificada", elementName)
	}

	if nameExists {
		fmt.Println("DEBUG: Error - Nombre ya existe:", rename.name)
		return fmt.Errorf("ya existe un elemento con el nombre %s en la ruta especificada", rename.name)
	}

	// Leer el inodo del elemento
	fmt.Println("DEBUG: Leyendo inodo del elemento - Inodo:", targetInodeIndex)
	inode := &structures.Inode{}
	inodePosition := int64(sb.S_inode_start) + int64(targetInodeIndex)*int64(sb.S_inode_size)
	fmt.Println("DEBUG: Deserializando inodo - Posición:", inodePosition)
	err = inode.Deserialize(path, inodePosition)
	if err != nil {
		fmt.Println("DEBUG: Error al leer inodo del elemento:", err)
		return fmt.Errorf("error al leer inodo del elemento: %v", err)
	}
	fmt.Println("DEBUG: Inodo deserializado - I_type:", string(inode.I_type[:]), "I_perm:", string(inode.I_perm[:]))

	// Verificar permisos de escritura
	fmt.Println("DEBUG: Verificando permisos de escritura...")
	if stores.ActiveSession.Username != "root" {
		hasPermission := checkWritePermission(inode)
		fmt.Println("DEBUG: Permiso de escritura:", hasPermission)
		if !hasPermission {
			fmt.Println("DEBUG: Error - Permiso de escritura denegado")
			return errors.New("permiso denegado: no tiene permisos de escritura en el elemento")
		}
	} else {
		fmt.Println("DEBUG: Usuario es root, permisos de escritura no verificados")
	}

	// Actualizar el nombre en el directorio padre
	fmt.Println("DEBUG: Actualizando nombre en el directorio padre - Nuevo nombre:", rename.name)
	newNameBytes := [12]byte{}
	copy(newNameBytes[:], rename.name)
	parentBlock.B_content[targetEntryIndex].B_name = newNameBytes
	fmt.Println("DEBUG: Entrada actualizada - Índice:", targetEntryIndex, "Nuevo nombre:", string(parentBlock.B_content[targetEntryIndex].B_name[:]))

	// Serializar el bloque del directorio padre
	fmt.Println("DEBUG: Serializando bloque del directorio padre - Posición:", parentInode.I_block[0])
	err = parentBlock.Serialize(path, int64(parentInode.I_block[0])) // Usar posición absoluta
	if err != nil {
		fmt.Println("DEBUG: Error al actualizar bloque del padre:", err)
		return fmt.Errorf("error al actualizar bloque del padre: %v", err)
	}
	fmt.Println("DEBUG: Bloque del directorio padre actualizado exitosamente")

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

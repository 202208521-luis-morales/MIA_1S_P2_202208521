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

// CHMOD estructura que representa el comando chmod con sus parámetros
type CHMOD struct {
	path string // Ruta del archivo/carpeta
	ugo  string // Permisos en formato UGO (ejemplo: "764")
	r    bool   // Indica si el cambio es recursivo
}

// ParseChmod parsea el comando chmod y cambia los permisos del archivo/carpeta
func ParseChmod(tokens []string) (string, error) {
	cmd := &CHMOD{}
	args := strings.Join(tokens, " ")
	re := regexp.MustCompile(`(?i)-path="[^"]+"|(?i)-path=[^\s]+|(?i)-ugo=[^\s]+|(?i)-r\b`)
	matches := re.FindAllString(args, -1)

	// Validar parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)|\b-r\b`)
	allParams := reAllParams.FindAllString(args, -1)
	allowedParams := map[string]bool{
		"-path": true,
		"-ugo":  true,
		"-r":    true,
	}

	for _, param := range allParams {
		paramName := strings.ToLower(param)
		if paramName != "-r" {
			paramName = strings.ToLower(strings.SplitN(param, "=", 2)[0])
		}
		if !allowedParams[paramName] {
			return "", fmt.Errorf("parámetro no permitido: %s", paramName)
		}
	}

	for _, match := range matches {
		if match == "-r" || match == "-R" {
			cmd.r = true
			continue
		}
		kv := strings.SplitN(match, "=", 2)
		if len(kv) != 2 {
			return "", fmt.Errorf("formato de parámetro inválido: %s", match)
		}
		key, value := strings.ToLower(kv[0]), kv[1]

		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			value = strings.Trim(value, "\"")
		}

		switch key {
		case "-path":
			if value == "" {
				return "", errors.New("la ruta no puede estar vacía")
			}
			cmd.path = value
		case "-ugo":
			if value == "" {
				return "", errors.New("el valor de ugo no puede estar vacío")
			}
			// Validar formato de ugo (tres dígitos entre 0 y 7)
			if !regexp.MustCompile(`^[0-7]{3}$`).MatchString(value) {
				return "", errors.New("el valor de ugo debe ser un número de tres dígitos entre 0 y 7 (ejemplo: 764)")
			}
			cmd.ugo = value
		}
	}

	if cmd.path == "" || cmd.ugo == "" {
		return "", fmt.Errorf("faltan parámetros requeridos: -path y -ugo son obligatorios")
	}

	err := commandChmod(cmd)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("CHMOD: Permisos cambiados exitosamente\n-> Ruta: %s\n-> Nuevos permisos: %s", cmd.path, cmd.ugo), nil
}

// commandChmod cambia los permisos del archivo/carpeta
func commandChmod(cmd *CHMOD) error {
	// Verificar sesión activa
	if stores.ActiveSession == nil {
		return errors.New("debe iniciar sesión primero para cambiar permisos")
	}

	// Obtener superbloque y partición montada
	sb, partition, path, err := stores.GetMountedPartitionSuperblock(stores.ActiveSession.PartitionID)
	if err != nil {
		return fmt.Errorf("error al obtener partición: %v", err)
	}

	// Normalizar la ruta
	cmd.path = filepath.Clean(cmd.path)
	if !strings.HasPrefix(cmd.path, "/") {
		return errors.New("la ruta debe ser absoluta (comenzar con /)")
	}

	// Separar directorio padre y nombre del elemento
	dir, elementName := filepath.Split(cmd.path)
	if elementName == "" {
		return errors.New("el nombre del elemento no puede estar vacío")
	}

	// Buscar el directorio padre
	_, parentBlock, _, err := findParentDir(sb, partition, path, dir)
	if err != nil {
		return fmt.Errorf("error al buscar directorio padre: %v", err)
	}

	// Buscar el elemento en el directorio padre
	var targetInodeIndex int32 = -1
	for _, content := range parentBlock.B_content {
		name := strings.Trim(string(content.B_name[:]), "\x00")
		if name == elementName {
			targetInodeIndex = content.B_inodo
			break
		}
	}

	if targetInodeIndex == -1 {
		return fmt.Errorf("el elemento %s no existe en la ruta especificada", cmd.path)
	}

	// Leer el inodo del elemento
	inode := &structures.Inode{}
	err = inode.Deserialize(path, int64(sb.S_inode_start)+int64(targetInodeIndex)*int64(sb.S_inode_size))
	if err != nil {
		return fmt.Errorf("error al leer inodo del elemento: %v", err)
	}

	// Verificar permisos: el usuario actual debe ser el propietario
	ownerID := inode.I_uid
	currentUserID := stores.ActiveSession.UserID
	if ownerID != currentUserID {
		return fmt.Errorf("permiso denegado: solo el propietario puede cambiar los permisos del archivo/carpeta")
	}

	// Convertir los permisos UGO a bytes
	ugoBytes := [3]byte{}
	for i := 0; i < 3; i++ {
		digit := cmd.ugo[i] - '0' // Convertir el carácter a su valor numérico (0-7)
		ugoBytes[i] = digit
	}

	// Cambiar los permisos del elemento
	err = changePermissions(sb, path, inode, targetInodeIndex, ugoBytes, cmd.r, currentUserID)
	if err != nil {
		return fmt.Errorf("error al cambiar permisos: %v", err)
	}

	// Actualizar el superbloque
	err = sb.Serialize(path, int64(partition.Part_start))
	if err != nil {
		return fmt.Errorf("error al actualizar superbloque: %v", err)
	}

	return nil
}

// changePermissions cambia los permisos de un inodo y, si es recursivo, de sus hijos
func changePermissions(sb *structures.SuperBlock, path string, inode *structures.Inode, inodeIndex int32, newPerms [3]byte, recursive bool, userID int32) error {
	// Solo cambiar permisos si el usuario actual es el propietario
	if inode.I_uid == userID {
		inode.I_perm = newPerms
		// Serializar el inodo actualizado
		err := inode.Serialize(path, int64(sb.S_inode_start)+int64(inodeIndex)*int64(sb.S_inode_size))
		if err != nil {
			return fmt.Errorf("error al actualizar inodo: %v", err)
		}
	}

	// Si no es recursivo o no es una carpeta, terminar
	if !recursive || inode.I_type[0] != '1' {
		return nil
	}

	// Si es una carpeta, procesar recursivamente
	for _, blockIndex := range inode.I_block {
		if blockIndex == -1 {
			continue
		}
		block := &structures.FolderBlock{}
		err := block.Deserialize(path, int64(sb.S_block_start)+int64(blockIndex)*int64(sb.S_block_size))
		if err != nil {
			return fmt.Errorf("error al leer bloque de carpeta: %v", err)
		}

		for _, content := range block.B_content {
			name := strings.Trim(string(content.B_name[:]), "\x00")
			if name == "." || name == ".." || content.B_inodo == -1 {
				continue
			}

			// Leer el inodo del elemento hijo
			childInode := &structures.Inode{}
			err := childInode.Deserialize(path, int64(sb.S_inode_start)+int64(content.B_inodo)*int64(sb.S_inode_size))
			if err != nil {
				return fmt.Errorf("error al leer inodo hijo: %v", err)
			}

			// Cambiar permisos recursivamente
			err = changePermissions(sb, path, childInode, content.B_inodo, newPerms, recursive, userID)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

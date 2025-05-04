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

// CHOWN estructura que representa el comando chown con sus parámetros
type CHOWN struct {
	path    string // Ruta del archivo/carpeta
	usuario string // Nombre del nuevo propietario
	r       bool   // Indica si el cambio es recursivo
}

// ParseChown parsea el comando chown y cambia el propietario del archivo/carpeta
func ParseChown(tokens []string) (string, error) {
	cmd := &CHOWN{}
	args := strings.Join(tokens, " ")
	re := regexp.MustCompile(`(?i)-path="[^"]+"|(?i)-path=[^\s]+|(?i)-usuario=[^\s]+|(?i)-r\b`)
	matches := re.FindAllString(args, -1)

	// Validar parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)|\b-r\b`)
	allParams := reAllParams.FindAllString(args, -1)
	allowedParams := map[string]bool{
		"-path":    true,
		"-usuario": true,
		"-r":       true,
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
		case "-usuario":
			if value == "" {
				return "", errors.New("el nombre del usuario no puede estar vacío")
			}
			if len(value) > 10 {
				return "", errors.New("el nombre del usuario no puede exceder 10 caracteres")
			}
			cmd.usuario = value
		}
	}

	if cmd.path == "" || cmd.usuario == "" {
		return "", fmt.Errorf("faltan parámetros requeridos: -path y -usuario son obligatorios")
	}

	err := commandChown(cmd)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("CHOWN: Propietario cambiado exitosamente\n-> Ruta: %s\n-> Nuevo propietario: %s", cmd.path, cmd.usuario), nil
}

// commandChown cambia el propietario del archivo/carpeta
func commandChown(cmd *CHOWN) error {
	// Verificar sesión activa
	if stores.ActiveSession == nil {
		return errors.New("debe iniciar sesión primero para cambiar el propietario")
	}

	// Obtener superbloque y partición montada
	sb, partition, path, err := stores.GetMountedPartitionSuperblock(stores.ActiveSession.PartitionID)
	if err != nil {
		return fmt.Errorf("error al obtener partición: %v", err)
	}

	// Verificar que el usuario especificado exista en users.txt y obtener su ID
	usersText, err := sb.ReadUsersFile(partition, path)
	if err != nil {
		return fmt.Errorf("error al leer users.txt: %v", err)
	}
	userID := int32(-1)
	lines := strings.Split(usersText, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) != 5 {
			continue
		}
		if fields[1] == "U" && strings.TrimSpace(fields[2]) == cmd.usuario {
			userID, err = parseInt32(fields[0]) // ID del nuevo propietario
			if err != nil {
				return fmt.Errorf("error al parsear ID del usuario %s: %v", cmd.usuario, err)
			}
			break
		}
	}
	if userID == -1 {
		return fmt.Errorf("el usuario %s no existe en la partición", cmd.usuario)
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

	// Verificar permisos para cambiar el propietario
	if stores.ActiveSession.Username != "root" {
		// Si no es root, el usuario actual debe ser el propietario del archivo/carpeta
		ownerID := inode.I_uid
		currentUserID := stores.ActiveSession.UserID
		if ownerID != currentUserID {
			return fmt.Errorf("permiso denegado: solo puede cambiar el propietario de sus propios archivos")
		}
	}

	// Cambiar el propietario del elemento
	err = changeOwner(sb, path, inode, targetInodeIndex, userID, cmd.r)
	if err != nil {
		return fmt.Errorf("error al cambiar propietario: %v", err)
	}

	// Actualizar el superbloque
	err = sb.Serialize(path, int64(partition.Part_start))
	if err != nil {
		return fmt.Errorf("error al actualizar superbloque: %v", err)
	}

	return nil
}

// changeOwner cambia el propietario de un inodo y, si es recursivo, de sus hijos
func changeOwner(sb *structures.SuperBlock, path string, inode *structures.Inode, inodeIndex int32, newOwnerID int32, recursive bool) error {
	// Actualizar el propietario del inodo actual
	inode.I_uid = newOwnerID

	// Serializar el inodo actualizado
	err := inode.Serialize(path, int64(sb.S_inode_start)+int64(inodeIndex)*int64(sb.S_inode_size))
	if err != nil {
		return fmt.Errorf("error al actualizar inodo: %v", err)
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

			// Cambiar el propietario recursivamente
			err = changeOwner(sb, path, childInode, content.B_inodo, newOwnerID, recursive)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

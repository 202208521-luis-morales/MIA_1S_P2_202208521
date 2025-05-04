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
	re := regexp.MustCompile(`(?i)-path="[^"]+"|(?i)-path=[^\s]+|(?i)-name="[^"]+"|(?i)-name=[^\s]+`)
	matches := re.FindAllString(args, -1)

	// Validar parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	allowedParams := map[string]bool{
		"-path": true,
		"-name": true,
	}

	for _, param := range allParams {
		paramName := strings.ToLower(strings.SplitN(param, "=", 2)[0])
		if !allowedParams[paramName] {
			return "", fmt.Errorf("parámetro no permitido: %s", paramName)
		}
	}

	for _, match := range matches {
		kv := strings.SplitN(match, "=", 2)
		if len(kv) != 2 {
			return "", fmt.Errorf("formato de parámetro inválido: %s", match)
		}
		key := strings.ToLower(kv[0])
		value := strings.Trim(kv[1], "\"")

		switch key {
		case "-path":
			if value == "" {
				return "", errors.New("la ruta no puede estar vacía")
			}
			cmd.path = value
		case "-name":
			if value == "" {
				return "", errors.New("el nombre no puede estar vacío")
			}
			cmd.name = value
		}
	}

	if cmd.path == "" || cmd.name == "" {
		return "", fmt.Errorf("faltan parámetros requeridos: -path y -name son obligatorios")
	}

	err := commandRename(cmd)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("RENAME: Elemento renombrado exitosamente\n-> Ruta: %s\n-> Nuevo nombre: %s", cmd.path, cmd.name), nil
}

// commandRename cambia el nombre de un archivo o carpeta
func commandRename(rename *RENAME) error {
	// Verificar sesión activa
	if stores.ActiveSession == nil {
		return errors.New("debe iniciar sesión primero para renombrar un elemento")
	}

	// Obtener superbloque y partición montada
	sb, partition, path, err := stores.GetMountedPartitionSuperblock(stores.ActiveSession.PartitionID)
	if err != nil {
		return fmt.Errorf("error al obtener partición: %v", err)
	}

	// Normalizar la ruta
	rename.path = filepath.Clean(rename.path)
	if !strings.HasPrefix(rename.path, "/") {
		return errors.New("la ruta debe ser absoluta (comenzar con /)")
	}

	// Separar directorio padre y nombre del elemento
	dir, elementName := filepath.Split(rename.path)
	if elementName == "" {
		return errors.New("el nombre del elemento no puede estar vacío")
	}

	// Validar el nuevo nombre
	if len(rename.name) > 12 {
		return errors.New("el nuevo nombre es demasiado largo (máximo 12 caracteres)")
	}

	// Buscar el directorio padre
	parentInode, parentBlock, _, err := findParentDir(sb, partition, path, dir)
	if err != nil {
		return fmt.Errorf("error al buscar directorio padre: %v", err)
	}

	// Buscar el elemento en el directorio padre y verificar si el nuevo nombre ya existe
	var targetInodeIndex int32 = -1
	var targetEntryIndex int
	nameExists := false
	for i, content := range parentBlock.B_content {
		name := strings.Trim(string(content.B_name[:]), "\x00")
		if name == elementName {
			targetInodeIndex = content.B_inodo
			targetEntryIndex = i
		}
		if name == rename.name {
			nameExists = true
		}
	}

	if targetInodeIndex == -1 {
		return fmt.Errorf("el elemento %s no existe en la ruta especificada", elementName)
	}

	if nameExists {
		return fmt.Errorf("ya existe un elemento con el nombre %s en la ruta especificada", rename.name)
	}

	// Leer el inodo del elemento
	inode := &structures.Inode{}
	err = inode.Deserialize(path, int64(sb.S_inode_start)+int64(targetInodeIndex)*int64(sb.S_inode_size))
	if err != nil {
		return fmt.Errorf("error al leer inodo del elemento: %v", err)
	}

	// Verificar permisos de escritura
	if stores.ActiveSession.Username != "root" {
		hasPermission := checkWritePermission(inode)
		if !hasPermission {
			return errors.New("permiso denegado: no tiene permisos de escritura en el elemento")
		}
	}

	// Actualizar el nombre en el directorio padre
	newNameBytes := [12]byte{}
	copy(newNameBytes[:], rename.name)
	parentBlock.B_content[targetEntryIndex].B_name = newNameBytes

	// Serializar el bloque del directorio padre
	err = parentBlock.Serialize(path, int64(sb.S_block_start)+int64(parentInode.I_block[0])*int64(sb.S_block_size))
	if err != nil {
		return fmt.Errorf("error al actualizar bloque del padre: %v", err)
	}

	// Actualizar el superbloque
	err = sb.Serialize(path, int64(partition.Part_start))
	if err != nil {
		return fmt.Errorf("error al actualizar superbloque: %v", err)
	}

	return nil
}

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
	destino string // Ruta del destino
}

// ParseMove parsea el comando move y devuelve el resultado del movimiento
func ParseMove(tokens []string) (string, error) {
	cmd := &MOVE{}
	args := strings.Join(tokens, " ")
	re := regexp.MustCompile(`(?i)-path="[^"]+"|(?i)-path=[^\s]+|(?i)-destino="[^"]+"|(?i)-destino=[^\s]+`)
	matches := re.FindAllString(args, -1)

	// Validar parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	allowedParams := map[string]bool{
		"-path":    true,
		"-destino": true,
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
		case "-destino":
			if value == "" {
				return "", errors.New("la ruta de destino no puede estar vacía")
			}
			cmd.destino = value
		}
	}

	if cmd.path == "" || cmd.destino == "" {
		return "", fmt.Errorf("faltan parámetros requeridos: -path y -destino son obligatorios")
	}

	err := commandMove(cmd)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("MOVE: Elemento movido exitosamente\n-> Origen: %s\n-> Destino: %s", cmd.path, cmd.destino), nil
}

// commandMove realiza el movimiento del archivo o carpeta
func commandMove(cmd *MOVE) error {
	// Verificar sesión activa
	if stores.ActiveSession == nil {
		return errors.New("debe iniciar sesión primero para mover un elemento")
	}

	// Obtener superbloque y partición montada
	sb, partition, path, err := stores.GetMountedPartitionSuperblock(stores.ActiveSession.PartitionID)
	if err != nil {
		return fmt.Errorf("error al obtener partición: %v", err)
	}

	// Normalizar las rutas
	cmd.path = filepath.Clean(cmd.path)
	cmd.destino = filepath.Clean(cmd.destino)
	if !strings.HasPrefix(cmd.path, "/") || !strings.HasPrefix(cmd.destino, "/") {
		return errors.New("las rutas deben ser absolutas (comenzar con /)")
	}

	// Separar directorio padre y nombre del elemento a mover
	srcDir, srcName := filepath.Split(cmd.path)
	if srcName == "" {
		return errors.New("el nombre del elemento a mover no puede estar vacío")
	}

	// Verificar que el elemento fuente exista
	srcParentInode, srcParentBlock, _, err := findParentDir(sb, partition, path, srcDir)
	if err != nil {
		return fmt.Errorf("error al buscar directorio padre del origen: %v", err)
	}

	var srcInodeIndex int32 = -1
	var srcEntryIndex int
	for i, content := range srcParentBlock.B_content {
		name := strings.Trim(string(content.B_name[:]), "\x00")
		if name == srcName {
			srcInodeIndex = content.B_inodo
			srcEntryIndex = i
			break
		}
	}

	if srcInodeIndex == -1 {
		return fmt.Errorf("el elemento %s no existe en la ruta especificada", cmd.path)
	}

	// Leer el inodo del elemento fuente
	srcInode := &structures.Inode{}
	err = srcInode.Deserialize(path, int64(sb.S_inode_start)+int64(srcInodeIndex)*int64(sb.S_inode_size))
	if err != nil {
		return fmt.Errorf("error al leer inodo del origen: %v", err)
	}

	// Verificar permisos de escritura en el elemento fuente
	if stores.ActiveSession.Username != "root" {
		hasWritePermission := checkWritePermission(srcInode)
		if !hasWritePermission {
			return fmt.Errorf("permiso denegado: no tiene permisos de escritura en %s", cmd.path)
		}
	}

	// Verificar la carpeta destino
	destDir, _ := filepath.Split(cmd.destino)
	destParentInode, destParentBlock, destParentInodeIndex, err := findParentDir(sb, partition, path, destDir)
	if err != nil {
		return fmt.Errorf("error al buscar carpeta destino: %v", err)
	}

	// Verificar permisos de escritura en la carpeta destino
	if stores.ActiveSession.Username != "root" {
		hasWritePermission := checkWritePermission(destParentInode)
		if !hasWritePermission {
			return fmt.Errorf("permiso denegado: no tiene permisos de escritura en %s", destDir)
		}
	}

	// Verificar si ya existe un elemento con el mismo nombre en el destino
	destName := srcName
	for _, content := range destParentBlock.B_content {
		name := strings.Trim(string(content.B_name[:]), "\x00")
		if name == destName {
			return fmt.Errorf("ya existe un elemento con el nombre %s en %s", destName, destDir)
		}
	}

	// Si el elemento es una carpeta, actualizar las entradas "." y ".."
	if srcInode.I_type[0] == '1' {
		for _, blockIndex := range srcInode.I_block {
			if blockIndex == -1 {
				continue
			}
			block := &structures.FolderBlock{}
			err := block.Deserialize(path, int64(sb.S_block_start)+int64(blockIndex)*int64(sb.S_block_size))
			if err != nil {
				return fmt.Errorf("error al leer bloque de carpeta: %v", err)
			}
			for i, content := range block.B_content {
				name := strings.Trim(string(content.B_name[:]), "\x00")
				if name == ".." {
					block.B_content[i].B_inodo = destParentInodeIndex
					break
				}
			}
			err = block.Serialize(path, int64(sb.S_block_start)+int64(blockIndex)*int64(sb.S_block_size))
			if err != nil {
				return fmt.Errorf("error al actualizar bloque de carpeta: %v", err)
			}
		}
	}

	// Eliminar la referencia del directorio padre origen
	srcParentBlock.B_content[srcEntryIndex] = structures.FolderContent{}
	err = srcParentBlock.Serialize(path, int64(sb.S_block_start)+int64(srcParentInode.I_block[0])*int64(sb.S_block_size))
	if err != nil {
		return fmt.Errorf("error al actualizar bloque del origen: %v", err)
	}

	// Agregar la nueva entrada al directorio destino
	newEntry := structures.FolderContent{
		B_inodo: srcInodeIndex,
	}
	copy(newEntry.B_name[:], []byte(destName))
	for i := range destParentBlock.B_content {
		if destParentBlock.B_content[i].B_inodo == -1 {
			destParentBlock.B_content[i] = newEntry
			break
		}
		if i == len(destParentBlock.B_content)-1 {
			return errors.New("no hay espacio en el bloque del directorio destino para agregar la nueva entrada")
		}
	}

	// Serializar el bloque del directorio destino
	err = destParentBlock.Serialize(path, int64(sb.S_block_start)+int64(destParentInode.I_block[0])*int64(sb.S_block_size))
	if err != nil {
		return fmt.Errorf("error al actualizar bloque del destino: %v", err)
	}

	// Actualizar el superbloque
	err = sb.Serialize(path, int64(partition.Part_start))
	if err != nil {
		return fmt.Errorf("error al actualizar superbloque: %v", err)
	}

	return nil
}

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
	re := regexp.MustCompile(`(?i)-path="[^"]+"|(?i)-path=[^\s]+`)
	matches := re.FindAllString(args, -1)

	// Validar parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	allowedParams := map[string]bool{
		"-path": true,
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
		}
	}

	if cmd.path == "" {
		return "", fmt.Errorf("faltan parámetros requeridos: -path")
	}

	err := commandRemove(cmd)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("REMOVE: Elemento eliminado exitosamente\n-> Ruta: %s", cmd.path), nil
}

// commandRemove elimina el archivo o carpeta especificado
func commandRemove(remove *REMOVE) error {
	// Verificar sesión activa
	if stores.ActiveSession == nil {
		return errors.New("debe iniciar sesión primero para eliminar un elemento")
	}

	// Obtener superbloque y partición montada
	sb, partition, path, err := stores.GetMountedPartitionSuperblock(stores.ActiveSession.PartitionID)
	if err != nil {
		return fmt.Errorf("error al obtener partición: %v", err)
	}

	// Normalizar la ruta
	remove.path = filepath.Clean(remove.path)
	if !strings.HasPrefix(remove.path, "/") {
		return errors.New("la ruta debe ser absoluta (comenzar con /)")
	}

	// Separar directorio padre y nombre del elemento
	dir, elementName := filepath.Split(remove.path)
	if elementName == "" {
		return errors.New("el nombre del elemento no puede estar vacío")
	}

	// Buscar el directorio padre
	parentInode, parentBlock, _, err := findParentDir(sb, partition, path, dir)
	if err != nil {
		return fmt.Errorf("error al buscar directorio padre: %v", err)
	}

	// Verificar permisos de escritura en el directorio padre
	if stores.ActiveSession.Username != "root" {
		hasPermission := checkWritePermission(parentInode)
		if !hasPermission {
			return errors.New("permiso denegado: no tiene permisos de escritura en la carpeta padre")
		}
	}

	// Buscar el elemento en el directorio padre
	var targetInodeIndex int32 = -1
	var targetEntryIndex int
	for i, content := range parentBlock.B_content {
		name := strings.Trim(string(content.B_name[:]), "\x00")
		if name == elementName {
			targetInodeIndex = content.B_inodo
			targetEntryIndex = i
			break
		}
	}

	if targetInodeIndex == -1 {
		return fmt.Errorf("el elemento %s no existe en la ruta especificada", elementName)
	}

	// Verificar permisos de escritura en el elemento y su contenido (si es carpeta)
	canDelete, err := checkElementPermissions(sb, path, targetInodeIndex)
	if err != nil {
		return fmt.Errorf("error al verificar permisos: %v", err)
	}
	if !canDelete {
		return errors.New("no se puede eliminar: falta permiso de escritura en el elemento o su contenido")
	}

	// Eliminar el elemento (archivo o carpeta)
	err = deleteElement(sb, path, targetInodeIndex)
	if err != nil {
		return fmt.Errorf("error al eliminar elemento: %v", err)
	}

	// Eliminar la entrada del directorio padre
	parentBlock.B_content[targetEntryIndex] = structures.FolderContent{}
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

// findParentDir busca el directorio padre de la ruta especificada
func findParentDir(sb *structures.SuperBlock, partition *structures.Partition, path string, dir string) (*structures.Inode, *structures.FolderBlock, int32, error) {
	if dir == "/" {
		inode := &structures.Inode{}
		err := inode.Deserialize(path, int64(sb.S_inode_start))
		if err != nil {
			return nil, nil, 0, err
		}
		block := &structures.FolderBlock{}
		err = block.Deserialize(path, int64(sb.S_block_start)+int64(inode.I_block[0])*int64(sb.S_block_size))
		if err != nil {
			return nil, nil, 0, err
		}
		return inode, block, 0, nil
	}

	parts := strings.Split(strings.Trim(dir, "/"), "/")
	currentInodeIndex := int32(0)
	currentInode := &structures.Inode{}
	err := currentInode.Deserialize(path, int64(sb.S_inode_start)+int64(currentInodeIndex)*int64(sb.S_inode_size))
	if err != nil {
		return nil, nil, 0, err
	}

	for _, part := range parts {
		if part == "" {
			continue
		}
		block := &structures.FolderBlock{}
		err := block.Deserialize(path, int64(sb.S_block_start)+int64(currentInode.I_block[0])*int64(sb.S_block_size))
		if err != nil {
			return nil, nil, 0, err
		}
		found := false
		for _, content := range block.B_content {
			name := strings.Trim(string(content.B_name[:]), "\x00")
			if name == part {
				currentInodeIndex = content.B_inodo
				currentInode = &structures.Inode{}
				err = currentInode.Deserialize(path, int64(sb.S_inode_start)+int64(currentInodeIndex)*int64(sb.S_inode_size))
				if err != nil {
					return nil, nil, 0, err
				}
				found = true
				break
			}
		}
		if !found {
			return nil, nil, 0, fmt.Errorf("directorio padre %s no encontrado", dir)
		}
	}

	block := &structures.FolderBlock{}
	err = block.Deserialize(path, int64(sb.S_block_start)+int64(currentInode.I_block[0])*int64(sb.S_block_size))
	if err != nil {
		return nil, nil, 0, err
	}
	return currentInode, block, currentInodeIndex, nil
}

// checkElementPermissions verifica recursivamente los permisos de escritura
func checkElementPermissions(sb *structures.SuperBlock, path string, inodeIndex int32) (bool, error) {
	inode := &structures.Inode{}
	err := inode.Deserialize(path, int64(sb.S_inode_start)+int64(inodeIndex)*int64(sb.S_inode_size))
	if err != nil {
		return false, err
	}

	// Verificar permisos de escritura en el elemento
	if stores.ActiveSession.Username != "root" {
		hasPermission := checkWritePermission(inode)
		if !hasPermission {
			return false, nil
		}
	}

	// Si es una carpeta, verificar permisos en su contenido
	if inode.I_type[0] == '1' { // '1' para carpetas
		for _, blockIndex := range inode.I_block {
			if blockIndex == -1 {
				continue
			}
			block := &structures.FolderBlock{}
			err := block.Deserialize(path, int64(sb.S_block_start)+int64(blockIndex)*int64(sb.S_block_size))
			if err != nil {
				return false, err
			}
			for _, content := range block.B_content {
				name := strings.Trim(string(content.B_name[:]), "\x00")
				if name == "." || name == ".." || content.B_inodo == -1 {
					continue
				}
				canDelete, err := checkElementPermissions(sb, path, content.B_inodo)
				if err != nil {
					return false, err
				}
				if !canDelete {
					return false, nil
				}
			}
		}
	}

	return true, nil
}

// deleteElement elimina un elemento (archivo o carpeta) y su contenido
func deleteElement(sb *structures.SuperBlock, path string, inodeIndex int32) error {
	inode := &structures.Inode{}
	err := inode.Deserialize(path, int64(sb.S_inode_start)+int64(inodeIndex)*int64(sb.S_inode_size))
	if err != nil {
		return err
	}

	// Si es una carpeta, eliminar su contenido recursivamente
	if inode.I_type[0] == '1' { // '1' para carpetas
		for _, blockIndex := range inode.I_block {
			if blockIndex == -1 {
				continue
			}
			block := &structures.FolderBlock{}
			err := block.Deserialize(path, int64(sb.S_block_start)+int64(blockIndex)*int64(sb.S_block_size))
			if err != nil {
				return err
			}
			for j, content := range block.B_content {
				name := strings.Trim(string(content.B_name[:]), "\x00")
				if name == "." || name == ".." || content.B_inodo == -1 {
					continue
				}
				err = deleteElement(sb, path, content.B_inodo)
				if err != nil {
					return err
				}
				block.B_content[j] = structures.FolderContent{}
			}
			err = block.Serialize(path, int64(sb.S_block_start)+int64(blockIndex)*int64(sb.S_block_size))
			if err != nil {
				return err
			}
		}
	}

	// Liberar los bloques del inodo
	for i, blockIndex := range inode.I_block {
		if blockIndex != -1 {
			err := freeBlock(sb, path, blockIndex)
			if err != nil {
				return err
			}
			inode.I_block[i] = -1
		}
	}
	// Serializar el inodo actualizado
	err = inode.Serialize(path, int64(sb.S_inode_start)+int64(inodeIndex)*int64(sb.S_inode_size))
	if err != nil {
		return err
	}

	// Liberar el inodo
	err = freeInode(sb, path, inodeIndex)
	if err != nil {
		return err
	}

	return nil
}

// freeInode libera un inodo
func freeInode(sb *structures.SuperBlock, path string, inodeIndex int32) error {
	// Incrementar el conteo de inodos libres
	sb.S_free_inodes_count++

	// Actualizar el bitmap de inodos
	bitmapOffset := int64(sb.S_bm_inode_start) + int64(inodeIndex/8)
	bitPosition := inodeIndex % 8

	file, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Leer el byte del bitmap
	_, err = file.Seek(bitmapOffset, 0)
	if err != nil {
		return err
	}
	byteData := make([]byte, 1)
	_, err = file.Read(byteData)
	if err != nil {
		return err
	}

	// Limpiar el bit correspondiente (marcar como libre: 0)
	byteData[0] &^= (1 << bitPosition)

	// Escribir el byte actualizado
	_, err = file.Seek(bitmapOffset, 0)
	if err != nil {
		return err
	}
	_, err = file.Write(byteData)
	if err != nil {
		return err
	}

	return nil
}

// freeBlock libera un bloque
func freeBlock(sb *structures.SuperBlock, path string, blockIndex int32) error {
	// Incrementar el conteo de bloques libres
	sb.S_free_blocks_count++

	// Actualizar el bitmap de bloques
	bitmapOffset := int64(sb.S_bm_block_start) + int64(blockIndex/8)
	bitPosition := blockIndex % 8

	file, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Leer el byte del bitmap
	_, err = file.Seek(bitmapOffset, 0)
	if err != nil {
		return err
	}
	byteData := make([]byte, 1)
	_, err = file.Read(byteData)
	if err != nil {
		return err
	}

	// Limpiar el bit correspondiente (marcar como libre: 0)
	byteData[0] &^= (1 << bitPosition)

	// Escribir el byte actualizado
	_, err = file.Seek(bitmapOffset, 0)
	if err != nil {
		return err
	}
	_, err = file.Write(byteData)
	if err != nil {
		return err
	}

	return nil
}

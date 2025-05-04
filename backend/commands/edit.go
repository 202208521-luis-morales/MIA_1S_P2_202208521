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
	re := regexp.MustCompile(`(?i)-path="[^"]+"|(?i)-path=[^\s]+|(?i)-contenido="[^"]+"|(?i)-contenido=[^\s]+`)
	matches := re.FindAllString(args, -1)

	// Validar parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	allowedParams := map[string]bool{
		"-path":      true,
		"-contenido": true,
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
		case "-contenido":
			if value == "" {
				return "", errors.New("la ruta del contenido no puede estar vacía")
			}
			cmd.contenido = value
		}
	}

	if cmd.path == "" || cmd.contenido == "" {
		return "", fmt.Errorf("faltan parámetros requeridos: -path y -contenido son obligatorios")
	}

	err := commandEdit(cmd)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("EDIT: Archivo editado exitosamente\n-> Ruta: %s", cmd.path), nil
}

// commandEdit edita el contenido de un archivo
func commandEdit(edit *EDIT) error {
	// Verificar sesión activa
	if stores.ActiveSession == nil {
		return errors.New("debe iniciar sesión primero para editar un archivo")
	}

	// Obtener superbloque y partición montada
	sb, partition, path, err := stores.GetMountedPartitionSuperblock(stores.ActiveSession.PartitionID)
	if err != nil {
		return fmt.Errorf("error al obtener partición: %v", err)
	}

	// Normalizar la ruta
	edit.path = filepath.Clean(edit.path)
	if !strings.HasPrefix(edit.path, "/") {
		return errors.New("la ruta debe ser absoluta (comenzar con /)")
	}

	// Separar directorio padre y nombre del archivo
	dir, fileName := filepath.Split(edit.path)
	if fileName == "" {
		return errors.New("el nombre del archivo no puede estar vacío")
	}

	// Buscar el directorio padre
	_, parentBlock, _, err := findParentDir(sb, partition, path, dir)
	if err != nil {
		return fmt.Errorf("error al buscar directorio padre: %v", err)
	}

	// Buscar el archivo en el directorio padre
	var targetInodeIndex int32 = -1
	for _, content := range parentBlock.B_content {
		name := strings.Trim(string(content.B_name[:]), "\x00")
		if name == fileName {
			targetInodeIndex = content.B_inodo
			break
		}
	}

	if targetInodeIndex == -1 {
		return fmt.Errorf("el archivo %s no existe en la ruta especificada", fileName)
	}

	// Leer el inodo del archivo
	inode := &structures.Inode{}
	err = inode.Deserialize(path, int64(sb.S_inode_start)+int64(targetInodeIndex)*int64(sb.S_inode_size))
	if err != nil {
		return fmt.Errorf("error al leer inodo del archivo: %v", err)
	}

	// Verificar que sea un archivo (no una carpeta)
	if inode.I_type[0] != '0' {
		return errors.New("la ruta especificada no corresponde a un archivo")
	}

	// Verificar permisos de lectura
	if stores.ActiveSession.Username != "root" {
		hasReadPermission := checkReadPermission(inode)
		if !hasReadPermission {
			return errors.New("permiso denegado: no tiene permisos de lectura en el archivo")
		}
	}

	// Verificar permisos de escritura
	if stores.ActiveSession.Username != "root" {
		hasWritePermission := checkWritePermission(inode)
		if !hasWritePermission {
			return errors.New("permiso denegado: no tiene permisos de escritura en el archivo")
		}
	}

	// Leer el nuevo contenido desde el archivo del sistema operativo
	newContent, err := os.ReadFile(edit.contenido)
	if err != nil {
		return fmt.Errorf("error al leer el archivo de contenido: %v", err)
	}

	// Liberar los bloques actuales del archivo
	for i, blockIndex := range inode.I_block {
		if blockIndex != -1 {
			err := freeBlock(sb, path, blockIndex)
			if err != nil {
				return fmt.Errorf("error al liberar bloque: %v", err)
			}
			inode.I_block[i] = -1
		}
	}

	// Asignar nuevos bloques para el nuevo contenido
	contentLen := len(newContent)
	blocksNeeded := (contentLen + 63) / 64 // Cada FileBlock almacena 64 bytes
	if blocksNeeded > 15 {
		return fmt.Errorf("el contenido es demasiado grande: se necesitan %d bloques, pero el inodo solo soporta 15", blocksNeeded)
	}

	// Actualizar el tamaño del archivo
	inode.I_size = int32(contentLen)

	// Escribir el nuevo contenido en los bloques
	for i := 0; i < blocksNeeded; i++ {
		// Asignar un nuevo bloque
		blockIndex, err := allocateBlock(sb, path)
		if err != nil {
			return fmt.Errorf("error al asignar bloque: %v", err)
		}
		inode.I_block[i] = blockIndex

		// Preparar el contenido del bloque
		start := i * 64
		end := start + 64
		if end > contentLen {
			end = contentLen
		}
		blockContent := make([]byte, 64)
		copy(blockContent, newContent[start:end])

		// Escribir el bloque
		fileBlock := &structures.FileBlock{}
		copy(fileBlock.B_content[:], blockContent)
		err = fileBlock.Serialize(path, int64(sb.S_block_start)+int64(blockIndex)*int64(sb.S_block_size))
		if err != nil {
			return fmt.Errorf("error al escribir bloque: %v", err)
		}
	}

	// Serializar el inodo actualizado
	err = inode.Serialize(path, int64(sb.S_inode_start)+int64(targetInodeIndex)*int64(sb.S_inode_size))
	if err != nil {
		return fmt.Errorf("error al actualizar inodo: %v", err)
	}

	// Actualizar el superbloque
	err = sb.Serialize(path, int64(partition.Part_start))
	if err != nil {
		return fmt.Errorf("error al actualizar superbloque: %v", err)
	}

	return nil
}

// allocateBlock asigna un nuevo bloque
func allocateBlock(sb *structures.SuperBlock, path string) (int32, error) {
	if sb.S_free_blocks_count == 0 {
		return -1, errors.New("no hay bloques libres disponibles")
	}

	// Buscar un bloque libre en el bitmap
	file, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return -1, err
	}
	defer file.Close()

	bitmapSize := int(sb.S_blocks_count / 8)
	if sb.S_blocks_count%8 != 0 {
		bitmapSize++
	}

	bitmapData := make([]byte, bitmapSize)
	_, err = file.Seek(int64(sb.S_bm_block_start), 0)
	if err != nil {
		return -1, err
	}
	_, err = file.Read(bitmapData)
	if err != nil {
		return -1, err
	}

	// Buscar un bit libre (0)
	var blockIndex int32 = -1
	for byteIdx := 0; byteIdx < bitmapSize; byteIdx++ {
		for bitIdx := 0; bitIdx < 8; bitIdx++ {
			if int32(byteIdx*8+bitIdx) >= sb.S_blocks_count {
				break
			}
			if (bitmapData[byteIdx] & (1 << bitIdx)) == 0 {
				blockIndex = int32(byteIdx*8 + bitIdx)
				bitmapData[byteIdx] |= (1 << bitIdx) // Marcar como ocupado
				break
			}
		}
		if blockIndex != -1 {
			break
		}
	}

	if blockIndex == -1 {
		return -1, errors.New("no se encontró un bloque libre")
	}

	// Escribir el bitmap actualizado
	_, err = file.Seek(int64(sb.S_bm_block_start), 0)
	if err != nil {
		return -1, err
	}
	_, err = file.Write(bitmapData)
	if err != nil {
		return -1, err
	}

	// Decrementar el conteo de bloques libres
	sb.S_free_blocks_count--

	return blockIndex, nil
}

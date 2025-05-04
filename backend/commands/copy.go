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

// COPY estructura que representa el comando copy con sus parámetros
type COPY struct {
	path    string // Ruta del archivo/carpeta a copiar
	destino string // Ruta del destino
}

// ParseCopy parsea el comando copy y devuelve el resultado de la copia
func ParseCopy(tokens []string) (string, error) {
	cmd := &COPY{}
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

	err := commandCopy(cmd)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("COPY: Elemento copiado exitosamente\n-> Origen: %s\n-> Destino: %s", cmd.path, cmd.destino), nil
}

// commandCopy realiza la copia del archivo o carpeta
func commandCopy(cmd *COPY) error {
	// Verificar sesión activa
	if stores.ActiveSession == nil {
		return errors.New("debe iniciar sesión primero para copiar un elemento")
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

	// Separar directorio padre y nombre del elemento a copiar
	srcDir, srcName := filepath.Split(cmd.path)
	if srcName == "" {
		return errors.New("el nombre del elemento a copiar no puede estar vacío")
	}

	// Verificar que el elemento fuente exista
	_, srcParentBlock, _, err := findParentDir(sb, partition, path, srcDir)
	if err != nil {
		return fmt.Errorf("error al buscar directorio padre del origen: %v", err)
	}

	var srcInodeIndex int32 = -1
	for _, content := range srcParentBlock.B_content {
		name := strings.Trim(string(content.B_name[:]), "\x00")
		if name == srcName {
			srcInodeIndex = content.B_inodo
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

	// Verificar permisos de lectura en el elemento fuente
	if stores.ActiveSession.Username != "root" {
		hasReadPermission := checkReadPermission(srcInode)
		if !hasReadPermission {
			return fmt.Errorf("permiso denegado: no tiene permisos de lectura en %s", cmd.path)
		}
	}

	// Verificar la carpeta destino
	destDir, _ := filepath.Split(cmd.destino)
	destParentInode, destParentBlock, _, err := findParentDir(sb, partition, path, destDir)
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

	// Copiar el elemento
	newInodeIndex, err := copyElement(sb, path, srcInode, srcInodeIndex)
	if err != nil {
		return fmt.Errorf("error al copiar elemento: %v", err)
	}

	// Agregar la nueva entrada al directorio destino
	newEntry := structures.FolderContent{
		B_inodo: newInodeIndex,
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

// copyElement copia un elemento (archivo o carpeta) y su contenido
func copyElement(sb *structures.SuperBlock, path string, srcInode *structures.Inode, srcInodeIndex int32) (int32, error) {
	// Asignar un nuevo inodo para la copia
	newInodeIndex, err := allocateInode(sb, path)
	if err != nil {
		return -1, fmt.Errorf("error al asignar inodo: %v", err)
	}

	// Crear una copia del inodo
	newInode := *srcInode
	for i := range newInode.I_block {
		newInode.I_block[i] = -1 // Limpiar los apuntadores a bloques
	}

	// Copiar los bloques según el tipo de elemento
	if srcInode.I_type[0] == '0' { // Archivo
		// Leer y copiar los bloques de contenido
		for i, blockIndex := range srcInode.I_block {
			if blockIndex == -1 {
				continue
			}
			srcBlock := &structures.FileBlock{}
			err := srcBlock.Deserialize(path, int64(sb.S_block_start)+int64(blockIndex)*int64(sb.S_block_size))
			if err != nil {
				return -1, fmt.Errorf("error al leer bloque de archivo: %v", err)
			}

			// Asignar un nuevo bloque
			newBlockIndex, err := allocateBlock(sb, path)
			if err != nil {
				return -1, fmt.Errorf("error al asignar bloque: %v", err)
			}
			newInode.I_block[i] = newBlockIndex

			// Copiar el contenido del bloque
			newBlock := *srcBlock
			err = newBlock.Serialize(path, int64(sb.S_block_start)+int64(newBlockIndex)*int64(sb.S_block_size))
			if err != nil {
				return -1, fmt.Errorf("error al escribir bloque de archivo: %v", err)
			}
		}
	} else if srcInode.I_type[0] == '1' { // Carpeta
		// Copiar los bloques de carpeta y su contenido recursivamente
		for i, blockIndex := range srcInode.I_block {
			if blockIndex == -1 {
				continue
			}
			srcBlock := &structures.FolderBlock{}
			err := srcBlock.Deserialize(path, int64(sb.S_block_start)+int64(blockIndex)*int64(sb.S_block_size))
			if err != nil {
				return -1, fmt.Errorf("error al leer bloque de carpeta: %v", err)
			}

			// Asignar un nuevo bloque
			newBlockIndex, err := allocateBlock(sb, path)
			if err != nil {
				return -1, fmt.Errorf("error al asignar bloque: %v", err)
			}
			newInode.I_block[i] = newBlockIndex

			// Copiar el bloque, ajustando los inodos de los contenidos
			newBlock := &structures.FolderBlock{}
			for j, content := range srcBlock.B_content {
				name := strings.Trim(string(content.B_name[:]), "\x00")
				if name == "." || name == ".." {
					newBlock.B_content[j] = content
					continue
				}
				if content.B_inodo == -1 {
					newBlock.B_content[j] = content
					continue
				}

				// Leer el inodo del elemento hijo
				childInode := &structures.Inode{}
				err := childInode.Deserialize(path, int64(sb.S_inode_start)+int64(content.B_inodo)*int64(sb.S_inode_size))
				if err != nil {
					return -1, fmt.Errorf("error al leer inodo hijo: %v", err)
				}

				// Verificar permisos de lectura para el elemento hijo
				if stores.ActiveSession.Username != "root" {
					hasReadPermission := checkReadPermission(childInode)
					if !hasReadPermission {
						fmt.Printf("No se copió %s debido a falta de permisos de lectura\n", name)
						continue
					}
				}

				// Copiar el elemento hijo recursivamente
				newChildInodeIndex, err := copyElement(sb, path, childInode, content.B_inodo)
				if err != nil {
					return -1, fmt.Errorf("error al copiar elemento hijo %s: %v", name, err)
				}

				// Actualizar la entrada con el nuevo inodo
				newBlock.B_content[j] = structures.FolderContent{
					B_name:  content.B_name,
					B_inodo: newChildInodeIndex,
				}
			}

			// Serializar el nuevo bloque
			err = newBlock.Serialize(path, int64(sb.S_block_start)+int64(newBlockIndex)*int64(sb.S_block_size))
			if err != nil {
				return -1, fmt.Errorf("error al escribir bloque de carpeta: %v", err)
			}
		}
	}

	// Serializar el nuevo inodo
	err = newInode.Serialize(path, int64(sb.S_inode_start)+int64(newInodeIndex)*int64(sb.S_inode_size))
	if err != nil {
		return -1, fmt.Errorf("error al escribir inodo: %v", err)
	}

	return newInodeIndex, nil
}

// allocateInode asigna un nuevo inodo
func allocateInode(sb *structures.SuperBlock, path string) (int32, error) {
	if sb.S_free_inodes_count == 0 {
		return -1, errors.New("no hay inodos libres disponibles")
	}

	// Buscar un inodo libre en el bitmap
	file, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return -1, err
	}
	defer file.Close()

	bitmapSize := int(sb.S_inodes_count / 8)
	if sb.S_inodes_count%8 != 0 {
		bitmapSize++
	}

	bitmapData := make([]byte, bitmapSize)
	_, err = file.Seek(int64(sb.S_bm_inode_start), 0)
	if err != nil {
		return -1, err
	}
	_, err = file.Read(bitmapData)
	if err != nil {
		return -1, err
	}

	// Buscar un bit libre (0)
	var inodeIndex int32 = -1
	for byteIdx := 0; byteIdx < bitmapSize; byteIdx++ {
		for bitIdx := 0; bitIdx < 8; bitIdx++ {
			if int32(byteIdx*8+bitIdx) >= sb.S_inodes_count {
				break
			}
			if (bitmapData[byteIdx] & (1 << bitIdx)) == 0 {
				inodeIndex = int32(byteIdx*8 + bitIdx)
				bitmapData[byteIdx] |= (1 << bitIdx) // Marcar como ocupado
				break
			}
		}
		if inodeIndex != -1 {
			break
		}
	}

	if inodeIndex == -1 {
		return -1, errors.New("no se encontró un inodo libre")
	}

	// Escribir el bitmap actualizado
	_, err = file.Seek(int64(sb.S_bm_inode_start), 0)
	if err != nil {
		return -1, err
	}
	_, err = file.Write(bitmapData)
	if err != nil {
		return -1, err
	}

	// Decrementar el conteo de inodos libres
	sb.S_free_inodes_count--

	return inodeIndex, nil
}

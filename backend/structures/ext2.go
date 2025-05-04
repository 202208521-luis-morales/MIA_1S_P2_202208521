package structures

import (
	utils "backend/utils"
	"fmt"
	"strings"
	"time"
)

// Crear users.txt en nuestro sistema de archivos
func (sb *SuperBlock) CreateUsersFile(path string) error {
	// ----------- Creamos / -----------
	// Creamos el inodo raíz
	rootInode := &Inode{
		I_uid:   1,
		I_gid:   1,
		I_size:  0,
		I_atime: float32(time.Now().Unix()),
		I_ctime: float32(time.Now().Unix()),
		I_mtime: float32(time.Now().Unix()),
		I_block: [15]int32{sb.S_blocks_count, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1},
		I_type:  [1]byte{'0'},
		I_perm:  [3]byte{'7', '7', '7'},
	}

	// Serializar el inodo raíz (inodo 0)
	err := rootInode.Serialize(path, int64(sb.S_inode_start)) // S_inode_start + 0 * S_inode_size
	if err != nil {
		return err
	}

	// Actualizar el bitmap de inodos para el inodo raíz (inodo 0)
	err = sb.UpdateBitmapInode(path, sb.S_inodes_count) // Usar S_inodes_count como índice del inodo
	if err != nil {
		return err
	}

	// Actualizar el superbloque
	sb.S_inodes_count++
	sb.S_free_inodes_count--
	sb.S_first_ino = sb.S_inodes_count // S_first_ino ahora refleja el próximo inodo disponible

	// Creamos el bloque del Inodo Raíz
	rootBlock := &FolderBlock{
		B_content: [4]FolderContent{
			{B_name: [12]byte{'.'}, B_inodo: 0},
			{B_name: [12]byte{'.', '.'}, B_inodo: 0},
			{B_name: [12]byte{'-'}, B_inodo: -1},
			{B_name: [12]byte{'-'}, B_inodo: -1},
		},
	}

	// Actualizar el bitmap de bloques
	err = sb.UpdateBitmapBlock(path)
	if err != nil {
		return err
	}

	// Serializar el bloque de carpeta raíz
	err = rootBlock.Serialize(path, int64(sb.S_first_blo))
	if err != nil {
		return err
	}

	// Actualizar el superbloque
	sb.S_blocks_count++
	sb.S_free_blocks_count--
	sb.S_first_blo += sb.S_block_size

	// ----------- Creamos /users.txt -----------
	usersText := "1,G,root\n1,U,root,root,123\n"

	// Deserializar el inodo raíz
	err = rootInode.Deserialize(path, int64(sb.S_inode_start+0)) // 0 porque es el inodo raíz
	if err != nil {
		return err
	}

	// Actualizamos el inodo raíz
	rootInode.I_atime = float32(time.Now().Unix())

	// Serializar el inodo raíz
	err = rootInode.Serialize(path, int64(sb.S_inode_start+0)) // 0 porque es el inodo raíz
	if err != nil {
		return err
	}

	// Deserializar el bloque de carpeta raíz
	err = rootBlock.Deserialize(path, int64(sb.S_block_start+0)) // 0 porque es el bloque de carpeta raíz
	if err != nil {
		return err
	}

	// Actualizamos el bloque de carpeta raíz
	rootBlock.B_content[2] = FolderContent{B_name: [12]byte{'u', 's', 'e', 'r', 's', '.', 't', 'x', 't'}, B_inodo: sb.S_inodes_count}

	// Serializar el bloque de carpeta raíz
	err = rootBlock.Serialize(path, int64(sb.S_block_start+0)) // 0 porque es el bloque de carpeta raíz
	if err != nil {
		return err
	}

	// Creamos el inodo users.txt
	usersInode := &Inode{
		I_uid:   1,
		I_gid:   1,
		I_size:  int32(len(usersText)),
		I_atime: float32(time.Now().Unix()),
		I_ctime: float32(time.Now().Unix()),
		I_mtime: float32(time.Now().Unix()),
		I_block: [15]int32{sb.S_blocks_count, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1},
		I_type:  [1]byte{'1'},
		I_perm:  [3]byte{'7', '7', '7'},
	}

	// Actualizar el bitmap de inodos para users.txt (inodo 1)
	err = sb.UpdateBitmapInode(path, sb.S_inodes_count) // Usar S_inodes_count como índice del inodo
	if err != nil {
		return err
	}

	// Serializar el inodo users.txt (inodo 1)
	err = usersInode.Serialize(path, int64(sb.S_inode_start+sb.S_inode_size)) // S_inode_start + 1 * S_inode_size
	if err != nil {
		return err
	}

	// Actualizamos el superbloque
	sb.S_inodes_count++
	sb.S_free_inodes_count--
	sb.S_first_ino = sb.S_inodes_count // S_first_ino ahora refleja el próximo inodo disponible

	// Creamos el bloque de users.txt
	usersBlock := &FileBlock{
		B_content: [64]byte{},
	}
	// Copiamos el texto de usuarios en el bloque
	copy(usersBlock.B_content[:], usersText)

	// Serializar el bloque de users.txt
	err = usersBlock.Serialize(path, int64(sb.S_first_blo))
	if err != nil {
		return err
	}

	// Actualizar el bitmap de bloques
	err = sb.UpdateBitmapBlock(path)
	if err != nil {
		return err
	}

	// Actualizamos el superbloque
	sb.S_blocks_count++
	sb.S_free_blocks_count--
	sb.S_first_blo += sb.S_block_size

	return nil
}

// createFolderInInode crea una carpeta en un inodo específico
func (sb *SuperBlock) createFolderInInode(path string, inodeIndex int32, parentsDir []string, destDir string) error {
	inode := &Inode{}
	err := inode.Deserialize(path, int64(sb.S_inode_start+(inodeIndex*sb.S_inode_size)))
	if err != nil {
		return err
	}
	if inode.I_type[0] == '1' {
		return nil
	}

	foundSpace := false
	for blockIdx, blockIndex := range inode.I_block {
		if blockIndex == -1 {
			if !foundSpace {
				if sb.S_free_blocks_count <= 0 {
					return fmt.Errorf("no hay bloques libres para ampliar el inodo %d", inodeIndex)
				}
				inode.I_block[blockIdx] = sb.S_first_blo
				sb.S_first_blo += sb.S_block_size
				sb.S_free_blocks_count--
				sb.S_blocks_count++
				err = sb.UpdateBitmapBlockAt(path, inode.I_block[blockIdx], '1')
				if err != nil {
					return fmt.Errorf("error al actualizar bitmap de bloques: %v", err)
				}

				newBlock := &FolderBlock{
					B_content: [4]FolderContent{
						{B_name: [12]byte{'-'}, B_inodo: -1},
						{B_name: [12]byte{'-'}, B_inodo: -1},
						{B_name: [12]byte{'-'}, B_inodo: -1},
						{B_name: [12]byte{'-'}, B_inodo: -1},
					},
				}
				err = newBlock.Serialize(path, int64(inode.I_block[blockIdx]))
				if err != nil {
					return fmt.Errorf("error al crear nuevo bloque para inodo %d: %v", inodeIndex, err)
				}
				blockIndex = inode.I_block[blockIdx]
			} else {
				break
			}
		}

		block := &FolderBlock{}
		err := block.Deserialize(path, int64(sb.S_block_start+(blockIndex*sb.S_block_size)))
		if err != nil {
			return err
		}

		for indexContent := 2; indexContent < len(block.B_content); indexContent++ {
			content := block.B_content[indexContent]

			if len(parentsDir) != 0 {
				if content.B_inodo == -1 {
					break
				}
				parentDir, err := utils.First(parentsDir)
				if err != nil {
					return err
				}
				contentName := strings.Trim(string(content.B_name[:]), "\x00 ")
				parentDirName := strings.Trim(parentDir, "\x00 ")
				if strings.EqualFold(contentName, parentDirName) {
					err := sb.createFolderInInode(path, content.B_inodo, utils.RemoveElement(parentsDir, 0), destDir)
					if err != nil {
						return err
					}
					return nil
				}
			} else {
				if content.B_inodo != -1 {
					continue
				}

				foundSpace = true
				copy(content.B_name[:], destDir)
				content.B_inodo = sb.S_inodes_count
				block.B_content[indexContent] = content
				err = block.Serialize(path, int64(sb.S_block_start+(blockIndex*sb.S_block_size)))
				if err != nil {
					return err
				}

				folderInode := &Inode{
					I_uid:   1,
					I_gid:   1,
					I_size:  0,
					I_atime: float32(time.Now().Unix()),
					I_ctime: float32(time.Now().Unix()),
					I_mtime: float32(time.Now().Unix()),
					I_block: [15]int32{sb.S_blocks_count, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1},
					I_type:  [1]byte{'0'},
					I_perm:  [3]byte{'6', '6', '4'},
				}

				err = folderInode.Serialize(path, int64(sb.S_inode_start+(sb.S_inodes_count*sb.S_inode_size)))
				if err != nil {
					return err
				}

				err = sb.UpdateBitmapInode(path, sb.S_inodes_count)
				if err != nil {
					return err
				}

				sb.S_inodes_count++
				sb.S_free_inodes_count--
				sb.S_first_ino = sb.S_inodes_count

				folderBlock := &FolderBlock{
					B_content: [4]FolderContent{
						{B_name: [12]byte{'.'}, B_inodo: content.B_inodo},
						{B_name: [12]byte{'.', '.'}, B_inodo: inodeIndex},
						{B_name: [12]byte{'-'}, B_inodo: -1},
						{B_name: [12]byte{'-'}, B_inodo: -1},
					},
				}

				err = folderBlock.Serialize(path, int64(sb.S_first_blo))
				if err != nil {
					return err
				}

				err = sb.UpdateBitmapBlock(path)
				if err != nil {
					return err
				}

				sb.S_blocks_count++
				sb.S_free_blocks_count--
				sb.S_first_blo += sb.S_block_size

				return nil
			}
		}
	}

	if !foundSpace {
		return fmt.Errorf("no hay espacio en el inodo %d para crear la carpeta %s", inodeIndex, destDir)
	}
	return nil
}

// ReadUsersFile lee el contenido de users.txt desde la partición EXT2, soportando múltiples bloques
func (sb *SuperBlock) ReadUsersFile(partition *Partition, path string) (string, error) {
	// Cargar el inodo raíz (índice 0)
	rootInode := &Inode{}
	err := rootInode.Deserialize(path, int64(sb.S_inode_start))
	if err != nil {
		return "", fmt.Errorf("error al cargar inodo raíz: %v", err)
	}

	// Cargar el bloque de carpeta raíz
	rootBlock := &FolderBlock{}
	err = rootBlock.Deserialize(path, int64(sb.S_block_start))
	if err != nil {
		return "", fmt.Errorf("error al cargar bloque raíz: %v", err)
	}

	// Buscar users.txt en el bloque raíz
	var usersInodeIndex int32 = -1
	for _, content := range rootBlock.B_content {
		name := strings.Trim(string(content.B_name[:]), "\x00")
		if name == "users.txt" {
			usersInodeIndex = content.B_inodo
			break
		}
	}
	if usersInodeIndex == -1 {
		return "", fmt.Errorf("no se encontró users.txt en la raíz")
	}

	// Cargar el inodo de users.txt
	usersInode := &Inode{}
	err = usersInode.Deserialize(path, int64(sb.S_inode_start+usersInodeIndex*sb.S_inode_size))
	if err != nil {
		return "", fmt.Errorf("error al cargar inodo de users.txt: %v", err)
	}

	// Leer el contenido desde todos los bloques
	var content strings.Builder
	blockSize := int(sb.S_block_size) // Tamaño de cada bloque (64 bytes)
	bytesRemaining := int(usersInode.I_size)

	for i, blockIndex := range usersInode.I_block {
		if blockIndex == -1 || bytesRemaining <= 0 {
			break
		}
		usersBlock := &FileBlock{}
		err = usersBlock.Deserialize(path, int64(blockIndex))
		if err != nil {
			return "", fmt.Errorf("error al cargar bloque %d de users.txt: %v", i, err)
		}
		// Leer solo los bytes restantes o el tamaño del bloque, lo que sea menor
		bytesToRead := blockSize
		if bytesRemaining < blockSize {
			bytesToRead = bytesRemaining
		}
		content.Write(usersBlock.B_content[:bytesToRead])
		bytesRemaining -= bytesToRead
	}

	return strings.Trim(content.String(), "\x00"), nil
}

// UpdateUsersFile actualiza el contenido de users.txt en la partición EXT2, soportando múltiples bloques
func (sb *SuperBlock) UpdateUsersFile(partition *Partition, path, newContent string) error {
	// Cargar el inodo raíz (índice 0)
	rootInode := &Inode{}
	err := rootInode.Deserialize(path, int64(sb.S_inode_start))
	if err != nil {
		return fmt.Errorf("error al cargar inodo raíz: %v", err)
	}

	// Cargar el bloque de carpeta raíz
	rootBlock := &FolderBlock{}
	err = rootBlock.Deserialize(path, int64(sb.S_block_start))
	if err != nil {
		return fmt.Errorf("error al cargar bloque raíz: %v", err)
	}

	// Buscar el inodo de users.txt
	var usersInodeIndex int32 = -1
	for _, content := range rootBlock.B_content {
		name := strings.Trim(string(content.B_name[:]), "\x00")
		if name == "users.txt" {
			usersInodeIndex = content.B_inodo
			break
		}
	}
	if usersInodeIndex == -1 {
		return fmt.Errorf("no se encontró users.txt en la raíz")
	}

	// Cargar el inodo de users.txt
	usersInode := &Inode{}
	err = usersInode.Deserialize(path, int64(sb.S_inode_start+usersInodeIndex*sb.S_inode_size))
	if err != nil {
		return fmt.Errorf("error al cargar inodo de users.txt: %v", err)
	}

	// Actualizar el tamaño y tiempo del inodo
	usersInode.I_size = int32(len(newContent))
	usersInode.I_mtime = float32(time.Now().Unix()) // Actualizar tiempo de modificación

	// Calcular cuántos bloques se necesitan
	blockSize := int(sb.S_block_size) // 64 bytes por bloque
	numBlocksNeeded := (len(newContent) + blockSize - 1) / blockSize
	if numBlocksNeeded > 15 { // I_block tiene 15 entradas
		return fmt.Errorf("el contenido de users.txt excede el límite de bloques soportados (15)")
	}

	// Reutilizar bloques existentes y asignar nuevos si es necesario
	contentBytes := []byte(newContent)
	for i := 0; i < numBlocksNeeded; i++ {
		start := i * blockSize
		end := start + blockSize
		if end > len(contentBytes) {
			end = len(contentBytes)
		}

		usersBlock := &FileBlock{}
		if usersInode.I_block[i] != -1 {
			// Reutilizar bloque existente
			err = usersBlock.Deserialize(path, int64(usersInode.I_block[i]))
			if err != nil {
				return fmt.Errorf("error al cargar bloque %d de users.txt: %v", i, err)
			}
		} else {
			// Asignar un nuevo bloque
			if sb.S_free_blocks_count <= 0 {
				return fmt.Errorf("no hay bloques libres para ampliar users.txt")
			}
			usersInode.I_block[i] = sb.S_first_blo
			sb.S_first_blo += sb.S_block_size // Avanzar al siguiente bloque disponible
			sb.S_free_blocks_count--
			sb.S_blocks_count++
			err = sb.UpdateBitmapBlockAt(path, usersInode.I_block[i], '1') // Marcar bloque como usado (cambiamos 'X' por '1')
			if err != nil {
				return fmt.Errorf("error al actualizar bitmap de bloques: %v", err)
			}
		}

		// Copiar el segmento del contenido al bloque
		copy(usersBlock.B_content[:], contentBytes[start:end])
		err = usersBlock.Serialize(path, int64(usersInode.I_block[i]))
		if err != nil {
			return fmt.Errorf("error al actualizar bloque %d de users.txt: %v", i, err)
		}
	}

	// Liberar bloques sobrantes si el contenido se redujo
	for i := numBlocksNeeded; i < 15 && usersInode.I_block[i] != -1; i++ {
		err = sb.UpdateBitmapBlockAt(path, usersInode.I_block[i], '0')
		if err != nil {
			return fmt.Errorf("error al liberar bloque %d de users.txt: %v", i, err)
		}
		// Limpiar el contenido del bloque
		usersBlock := &FileBlock{}
		usersBlock.B_content = [64]byte{} // Inicializar a ceros
		err = usersBlock.Serialize(path, int64(usersInode.I_block[i]))
		if err != nil {
			return fmt.Errorf("error al limpiar bloque %d de users.txt: %v", i, err)
		}
		usersInode.I_block[i] = -1
		sb.S_free_blocks_count++
		sb.S_blocks_count--
	}

	// Serializar el inodo actualizado
	err = usersInode.Serialize(path, int64(sb.S_inode_start+usersInodeIndex*sb.S_inode_size))
	if err != nil {
		return fmt.Errorf("error al actualizar inodo de users.txt: %v", err)
	}

	// Actualizar el superbloque
	err = sb.Serialize(path, int64(partition.Part_start))
	if err != nil {
		return fmt.Errorf("error al actualizar superbloque: %v", err)
	}

	return nil
}

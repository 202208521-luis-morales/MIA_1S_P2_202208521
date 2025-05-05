package structures

import (
	"backend/utils"
	"fmt"
	"strings"
	"time"
)

// Crear users.txt en nuestro sistema de archivos
func (sb *SuperBlock) CreateUsersFileExt3(path string, journauling_start int64) error {
	fmt.Println("DEBUG: Iniciando CreateUsersFileExt3 - Path:", path, "Journal Start:", journauling_start)

	// ----------- Creamos / -----------
	// Crear Journal inicial (vacío por ahora)
	journal := &Journal{
		J_count: sb.S_inodes_count,
		J_content: Information{
			I_operation: [10]byte{'m', 'k', 'd', 'i', 'r'},
			I_path:      [32]byte{'/'},
			I_content:   [64]byte{},
			I_date:      float32(time.Now().Unix()),
		},
	}
	fmt.Println("DEBUG: Serializing initial journal - Position:", journauling_start)
	err := journal.Serialize(path, journauling_start)
	if err != nil {
		fmt.Println("DEBUG: Error serializing initial journal:", err)
		return err
	}
	fmt.Println("DEBUG: Initial journal serialized successfully")

	// Crear inodo raíz
	fmt.Println("DEBUG: Creando root inode...")
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
	fmt.Println("DEBUG: Root inode created - I_block[0]:", rootInode.I_block[0], "I_type:", string(rootInode.I_type[:]), "I_perm:", string(rootInode.I_perm[:]))

	// Serializar el inodo raíz
	fmt.Println("DEBUG: Serializing root inode - Position:", sb.S_first_ino)
	err = rootInode.Serialize(path, int64(sb.S_first_ino))
	if err != nil {
		fmt.Println("DEBUG: Error serializing root inode:", err)
		return err
	}
	fmt.Println("DEBUG: Root inode serialized successfully")

	// Actualizar el bitmap de inodos
	fmt.Println("DEBUG: Updating inode bitmap for root - Index:", sb.S_inodes_count)
	err = sb.UpdateBitmapInode(path, sb.S_inodes_count)
	if err != nil {
		fmt.Println("DEBUG: Error updating inode bitmap for root:", err)
		return err
	}
	fmt.Println("DEBUG: Inode bitmap updated - S_inodes_count:", sb.S_inodes_count)

	// Actualizar el superbloque
	sb.S_inodes_count++
	sb.S_free_inodes_count--
	sb.S_first_ino += sb.S_inode_size
	fmt.Println("DEBUG: Superblock updated - S_inodes_count:", sb.S_inodes_count, "S_free_inodes_count:", sb.S_free_inodes_count, "S_first_ino:", sb.S_first_ino)

	// Crear el bloque del inodo raíz
	fmt.Println("DEBUG: Creating root block...")
	rootBlock := &FolderBlock{
		B_content: [4]FolderContent{
			{B_name: [12]byte{'.'}, B_inodo: 0},
			{B_name: [12]byte{'.', '.'}, B_inodo: 0},
			{B_name: [12]byte{'-'}, B_inodo: -1},
			{B_name: [12]byte{'-'}, B_inodo: -1},
		},
	}
	fmt.Println("DEBUG: Root block created - Entry 0:", string(rootBlock.B_content[0].B_name[:]), "Inodo:", rootBlock.B_content[0].B_inodo)

	// Actualizar I_block[0] del inodo raíz con la posición del bloque
	rootInode.I_block[0] = sb.S_first_blo
	fmt.Println("DEBUG: Updated root inode I_block[0] to:", rootInode.I_block[0])

	// Serializar el bloque de carpeta raíz
	fmt.Println("DEBUG: Serializing root block - Position:", sb.S_first_blo)
	err = rootBlock.Serialize(path, int64(sb.S_first_blo))
	if err != nil {
		fmt.Println("DEBUG: Error serializing root block:", err)
		return err
	}
	fmt.Println("DEBUG: Root block serialized successfully")

	// Actualizar el bitmap de bloques
	fmt.Println("DEBUG: Updating block bitmap for root...")
	err = sb.UpdateBitmapBlock(path)
	if err != nil {
		fmt.Println("DEBUG: Error updating block bitmap for root:", err)
		return err
	}
	fmt.Println("DEBUG: Block bitmap updated")

	// Actualizar el superbloque
	sb.S_blocks_count++
	sb.S_free_blocks_count--
	sb.S_first_blo += sb.S_block_size
	fmt.Println("DEBUG: Superblock updated - S_blocks_count:", sb.S_blocks_count, "S_free_blocks_count:", sb.S_free_blocks_count, "S_first_blo:", sb.S_first_blo)

	// Re-serializar el inodo raíz con el bloque actualizado
	fmt.Println("DEBUG: Re-serializing root inode - Position:", sb.S_first_ino-sb.S_inode_size)
	err = rootInode.Serialize(path, int64(sb.S_first_ino-sb.S_inode_size))
	if err != nil {
		fmt.Println("DEBUG: Error re-serializing root inode:", err)
		return err
	}
	fmt.Println("DEBUG: Root inode re-serialized successfully")

	// ----------- Creamos /users.txt -----------
	usersText := "1,G,root\n1,U,root,root,123\n"
	fmt.Println("DEBUG: Creating users.txt with content:", usersText)

	// Deserializar el inodo raíz
	fmt.Println("DEBUG: Deserializing root inode - Position:", sb.S_first_ino-sb.S_inode_size)
	err = rootInode.Deserialize(path, int64(sb.S_first_ino-sb.S_inode_size))
	if err != nil {
		fmt.Println("DEBUG: Error deserializing root inode:", err)
		return err
	}
	fmt.Println("DEBUG: Root inode deserialized - I_block[0]:", rootInode.I_block[0])

	// Actualizar el inodo raíz (solo atime)
	rootInode.I_atime = float32(time.Now().Unix())
	fmt.Println("DEBUG: Updated root inode I_atime:", rootInode.I_atime)

	// Serializar el inodo raíz
	fmt.Println("DEBUG: Serializing root inode - Position:", sb.S_first_ino-sb.S_inode_size)
	err = rootInode.Serialize(path, int64(sb.S_first_ino-sb.S_inode_size))
	if err != nil {
		fmt.Println("DEBUG: Error serializing root inode:", err)
		return err
	}
	fmt.Println("DEBUG: Root inode serialized successfully")

	// Deserializar el bloque de carpeta raíz
	fmt.Println("DEBUG: Deserializing root block - Position:", rootInode.I_block[0])
	err = rootBlock.Deserialize(path, int64(rootInode.I_block[0]))
	if err != nil {
		fmt.Println("DEBUG: Error deserializing root block:", err)
		return err
	}
	fmt.Println("DEBUG: Root block deserialized - Entry 2 Inodo:", rootBlock.B_content[2].B_inodo)

	// Actualizar el bloque de carpeta raíz con users.txt
	copy(rootBlock.B_content[2].B_name[:], "users.txt")
	rootBlock.B_content[2].B_inodo = sb.S_inodes_count
	fmt.Println("DEBUG: Updated root block - Entry 2: users.txt, Inodo:", rootBlock.B_content[2].B_inodo)

	// Serializar el bloque de carpeta raíz
	fmt.Println("DEBUG: Serializing root block - Position:", rootInode.I_block[0])
	err = rootBlock.Serialize(path, int64(rootInode.I_block[0]))
	if err != nil {
		fmt.Println("DEBUG: Error serializing root block:", err)
		return err
	}
	fmt.Println("DEBUG: Root block serialized successfully")

	// Crear el inodo users.txt
	fmt.Println("DEBUG: Creating users.txt inode...")
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
	fmt.Println("DEBUG: Users.txt inode created - I_size:", usersInode.I_size, "I_block[0]:", usersInode.I_block[0])

	// Actualizar el bitmap de inodos
	fmt.Println("DEBUG: Updating inode bitmap for users.txt - Index:", sb.S_inodes_count)
	err = sb.UpdateBitmapInode(path, sb.S_inodes_count)
	if err != nil {
		fmt.Println("DEBUG: Error updating inode bitmap for users.txt:", err)
		return err
	}
	fmt.Println("DEBUG: Inode bitmap updated")

	// Serializar el inodo users.txt
	fmt.Println("DEBUG: Serializing users.txt inode - Position:", sb.S_first_ino)
	err = usersInode.Serialize(path, int64(sb.S_first_ino))
	if err != nil {
		fmt.Println("DEBUG: Error serializing users.txt inode:", err)
		return err
	}
	fmt.Println("DEBUG: Users.txt inode serialized successfully")

	// Actualizar el superbloque
	sb.S_inodes_count++
	sb.S_free_inodes_count--
	sb.S_first_ino += sb.S_inode_size
	fmt.Println("DEBUG: Superblock updated - S_inodes_count:", sb.S_inodes_count, "S_free_inodes_count:", sb.S_free_inodes_count, "S_first_ino:", sb.S_first_ino)

	// Crear Journal para users.txt
	journalFile := &Journal{
		J_count: sb.S_inodes_count,
		J_content: Information{
			I_operation: [10]byte{'m', 'k', 'f', 'i', 'l', 'e'},
			I_path:      [32]byte{'/', 'u', 's', 'e', 'r', 's', '.', 't', 'x', 't'},
			I_content:   [64]byte{},
			I_date:      float32(time.Now().Unix()),
		},
	}
	copy(journalFile.J_content.I_content[:], usersText)
	fmt.Println("DEBUG: Serializing journal for users.txt - Position:", journauling_start)
	err = journalFile.Serialize(path, journauling_start)
	if err != nil {
		fmt.Println("DEBUG: Error serializing journal for users.txt:", err)
		return err
	}
	fmt.Println("DEBUG: Journal for users.txt serialized successfully")

	// Crear el bloque de users.txt
	fmt.Println("DEBUG: Creating users.txt block...")
	usersBlock := &FileBlock{
		B_content: [64]byte{},
	}
	copy(usersBlock.B_content[:], usersText)
	fmt.Println("DEBUG: Users.txt block content:", string(usersBlock.B_content[:]))

	// Serializar el bloque de users.txt
	fmt.Println("DEBUG: Serializing users.txt block - Position:", sb.S_first_blo)
	err = usersBlock.Serialize(path, int64(sb.S_first_blo))
	if err != nil {
		fmt.Println("DEBUG: Error serializing users.txt block:", err)
		return err
	}
	fmt.Println("DEBUG: Users.txt block serialized successfully")

	// Actualizar el bitmap de bloques
	fmt.Println("DEBUG: Updating block bitmap for users.txt...")
	err = sb.UpdateBitmapBlock(path)
	if err != nil {
		fmt.Println("DEBUG: Error updating block bitmap for users.txt:", err)
		return err
	}
	fmt.Println("DEBUG: Block bitmap updated")

	// Actualizar el inodo users.txt con el bloque
	usersInode.I_block[0] = sb.S_first_blo
	fmt.Println("DEBUG: Updated users.txt inode I_block[0] to:", usersInode.I_block[0])

	// Re-serializar el inodo users.txt
	fmt.Println("DEBUG: Re-serializing users.txt inode - Position:", sb.S_first_ino-sb.S_inode_size)
	err = usersInode.Serialize(path, int64(sb.S_first_ino-sb.S_inode_size))
	if err != nil {
		fmt.Println("DEBUG: Error re-serializing users.txt inode:", err)
		return err
	}
	fmt.Println("DEBUG: Users.txt inode re-serialized successfully")

	// Actualizar el superbloque
	sb.S_blocks_count++
	sb.S_free_blocks_count--
	sb.S_first_blo += sb.S_block_size
	fmt.Println("DEBUG: Superblock updated - S_blocks_count:", sb.S_blocks_count, "S_free_blocks_count:", sb.S_free_blocks_count, "S_first_blo:", sb.S_first_blo)

	return nil
}

// createFolderInInode crea una carpeta en un inodo específico
func (sb *SuperBlock) createFolderInInodeExt3(path string, inodeIndex int32, parentsDir []string, destDir string) error {
	// Crear un nuevo inodo
	inode := &Inode{}
	// Deserializar el inodo
	err := inode.Deserialize(path, int64(sb.S_inode_start+(inodeIndex*sb.S_inode_size)))
	if err != nil {
		return err
	}
	// Verificar si el inodo es de tipo carpeta
	if inode.I_type[0] == '1' {
		return nil
	}

	// Iterar sobre cada bloque del inodo (apuntadores)
	for _, blockIndex := range inode.I_block {
		// Si el bloque no existe, salir
		if blockIndex == -1 {
			break
		}

		// Crear un nuevo bloque de carpeta
		block := &FolderBlock{}

		// Deserializar el bloque
		err := block.Deserialize(path, int64(sb.S_block_start+(blockIndex*sb.S_block_size))) // 64 porque es el tamaño de un bloque
		if err != nil {
			return err
		}

		// Iterar sobre cada contenido del bloque, desde el index 2 porque los primeros dos son . y ..
		for indexContent := 2; indexContent < len(block.B_content); indexContent++ {
			// Obtener el contenido del bloque
			content := block.B_content[indexContent]

			// Sí las carpetas padre no están vacías debereamos buscar la carpeta padre más cercana
			if len(parentsDir) != 0 {
				//fmt.Println("---------ESTOY  VISITANDO--------")

				// Si el contenido está vacío, salir
				if content.B_inodo == -1 {
					break
				}

				// Obtenemos la carpeta padre más cercana
				parentDir, err := utils.First(parentsDir)
				if err != nil {
					return err
				}

				// Convertir B_name a string y eliminar los caracteres nulos
				contentName := strings.Trim(string(content.B_name[:]), "\x00 ")
				// Convertir parentDir a string y eliminar los caracteres nulos
				parentDirName := strings.Trim(parentDir, "\x00 ")
				// Si el nombre del contenido coincide con el nombre de la carpeta padre
				if strings.EqualFold(contentName, parentDirName) {
					//fmt.Println("---------LA ENCONTRÉ-------")
					// Si son las mismas, entonces entramos al inodo que apunta el bloque
					err := sb.createFolderInInodeExt3(path, content.B_inodo, utils.RemoveElement(parentsDir, 0), destDir)
					if err != nil {
						return err
					}
					return nil
				}
			} else {
				//fmt.Println("---------ESTOY  CREANDO--------")

				// Si el apuntador al inodo está ocupado, continuar con el siguiente
				if content.B_inodo != -1 {
					continue
				}

				// Actualizar el contenido del bloque
				copy(content.B_name[:], destDir)
				content.B_inodo = sb.S_inodes_count

				// Actualizar el bloque
				block.B_content[indexContent] = content

				// Serializar el bloque
				err = block.Serialize(path, int64(sb.S_block_start+(blockIndex*sb.S_block_size)))
				if err != nil {
					return err
				}

				// HACER EL JOURNALING

				// Crear el inodo de la carpeta
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

				// Serializar el inodo de la carpeta
				err = folderInode.Serialize(path, int64(sb.S_first_ino))
				if err != nil {
					return err
				}

				// Actualizar el bitmap de inodos
				err = sb.UpdateBitmapInode(path, sb.S_inodes_count)
				if err != nil {
					return err
				}

				// Actualizar el superbloque
				sb.S_inodes_count++
				sb.S_free_inodes_count--
				sb.S_first_ino += sb.S_inode_size

				// TODO: Ponen su Journal

				// Crear el bloque de la carpeta
				folderBlock := &FolderBlock{
					B_content: [4]FolderContent{
						{B_name: [12]byte{'.'}, B_inodo: content.B_inodo},
						{B_name: [12]byte{'.', '.'}, B_inodo: inodeIndex},
						{B_name: [12]byte{'-'}, B_inodo: -1},
						{B_name: [12]byte{'-'}, B_inodo: -1},
					},
				}

				// Serializar el bloque de la carpeta
				err = folderBlock.Serialize(path, int64(sb.S_first_blo))
				if err != nil {
					return err
				}

				// Actualizar el bitmap de bloques
				err = sb.UpdateBitmapBlock(path)
				if err != nil {
					return err
				}

				// Actualizar el superbloque
				sb.S_blocks_count++
				sb.S_free_blocks_count--
				sb.S_first_blo += sb.S_block_size

				return nil
			}
		}

	}
	return nil
}

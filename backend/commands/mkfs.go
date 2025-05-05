package commands

import (
	stores "backend/stores"
	structures "backend/structures"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"
	"time"
	"unsafe"
)

// MKFS estructura que representa el comando mkfs con sus parámetros
type MKFS struct {
	id  string // ID de la partición a formatear
	typ string // Tipo de formateo (full)
	fs  string // Sistema de archivos (2fs o 3fs)
}

// ParseMkfs parsea el comando mkfs y formatea la partición
func ParseMkfs(tokens []string) (string, error) {
	cmd := &MKFS{}
	args := strings.Join(tokens, " ")
	fmt.Println("DEBUG: Parsing tokens:", tokens)
	fmt.Println("DEBUG: Joined args:", args)
	// Expresión regular para capturar los parámetros válidos, insensible a mayúsculas/minúsculas
	re := regexp.MustCompile(`(?i)-id=[^\s]+|(?i)-type=[^\s]+|(?i)-fs=[23]fs`)
	matches := re.FindAllString(args, -1)
	fmt.Println("DEBUG: Matches found:", matches)

	// Validar que no haya parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	fmt.Println("DEBUG: All params found:", allParams)
	allowedParams := map[string]bool{
		"-id":   true,
		"-type": true,
		"-fs":   true,
	}

	for _, param := range allParams {
		paramName := strings.ToLower(strings.SplitN(param, "=", 2)[0])
		fmt.Println("DEBUG: Checking param:", paramName)
		if !allowedParams[paramName] {
			return "", fmt.Errorf("parámetro no permitido: %s", paramName)
		}
	}

	for _, match := range matches {
		fmt.Println("DEBUG: Processing match:", match)
		kv := strings.SplitN(match, "=", 2)
		if len(kv) != 2 {
			return "", fmt.Errorf("formato de parámetro inválido: %s", match)
		}
		key, value := strings.ToLower(kv[0]), kv[1]
		fmt.Println("DEBUG: Key:", key, "Value:", value)

		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			value = strings.Trim(value, "\"")
			fmt.Println("DEBUG: Trimmed quotes from value:", value)
		}

		switch key {
		case "-id":
			if value == "" {
				return "", errors.New("el ID no puede estar vacío")
			}
			cmd.id = value
			fmt.Println("DEBUG: Set ID:", cmd.id)
		case "-type":
			if value != "full" {
				return "", errors.New("el tipo debe ser full")
			}
			cmd.typ = value
			fmt.Println("DEBUG: Set type:", cmd.typ)
		case "-fs":
			if value != "2fs" && value != "3fs" {
				return "", errors.New("el sistema de archivos debe ser 2fs o 3fs")
			}
			cmd.fs = value
			fmt.Println("DEBUG: Set fs:", cmd.fs)
		}
	}

	if cmd.id == "" {
		fmt.Println("DEBUG: ID is empty, returning error")
		return "", errors.New("faltan parámetros requeridos: -id")
	}

	// Valores por defecto
	if cmd.typ == "" {
		cmd.typ = "full"
		fmt.Println("DEBUG: Set default type to full")
	}
	if cmd.fs == "" {
		cmd.fs = "2fs"
		fmt.Println("DEBUG: Set default fs to 2fs")
	}

	fmt.Println("DEBUG: Final cmd:", cmd)
	err := commandMkfs(cmd)
	if err != nil {
		fmt.Println("DEBUG: CommandMkfs returned error:", err)
		return "", err
	}
	return fmt.Sprintf("MKFS: Sistema de archivos creado exitosamente\n"+
		"-> ID: %s\n"+
		"-> Tipo: %s\n"+
		"-> Sistema de archivos: %s",
		cmd.id,
		cmd.typ,
		map[string]string{"2fs": "EXT2", "3fs": "EXT3"}[cmd.fs]), nil
}

// commandMkfs ejecuta la lógica para formatear la partición
func commandMkfs(cmd *MKFS) error {
	fmt.Println("\nDEBUG: Inicia commandMkfs - Sistema de archivos:", cmd.fs)

	// Obtener el superbloque y la partición
	sb, partition, path, err := stores.GetMountedPartitionSuperblock(cmd.id)
	if err != nil {
		fmt.Println("DEBUG: Error getting partition:", err)
		return fmt.Errorf("error al obtener partición: %v", err)
	}
	fmt.Println("DEBUG: Partition obtained - Path:", path, "Partition Size:", partition.Part_size)

	// Calcular n ajustado para EXT2 o EXT3
	n := calculateN(partition, cmd.fs)
	fmt.Println("DEBUG: Calculated n:", n)

	// Calcular posiciones de inicio
	journalStart, bmInodeStart, bmBlockStart, inodeStart, blockStart := calculateStartPositions(partition, cmd.fs, n)
	fmt.Println("DEBUG: Posiciones calculadas - Journal Start:", journalStart, "Bitmap Inode Start:", bmInodeStart, "Bitmap Block Start:", bmBlockStart, "Inode Start:", inodeStart, "Block Start:", blockStart)

	// Determinar el tipo de sistema de archivos
	var fsType int32
	if cmd.fs == "2fs" {
		fsType = 2
	} else {
		fsType = 3
	}
	fmt.Println("DEBUG: Filesystem type set to:", fsType)

	// Inicializar el superbloque
	sb.S_filesystem_type = fsType
	sb.S_inodes_count = 0
	sb.S_blocks_count = 0
	sb.S_free_inodes_count = n
	sb.S_free_blocks_count = n * 3
	sb.S_mtime = float32(time.Now().Unix())
	sb.S_umtime = float32(time.Now().Unix())
	sb.S_mnt_count = 1
	sb.S_magic = 0xEF53
	sb.S_inode_size = int32(unsafe.Sizeof(structures.Inode{}))
	sb.S_block_size = int32(unsafe.Sizeof(structures.FileBlock{}))
	sb.S_bm_inode_start = bmInodeStart
	sb.S_bm_block_start = bmBlockStart
	sb.S_inode_start = inodeStart
	sb.S_block_start = blockStart
	sb.S_first_ino = inodeStart
	sb.S_first_blo = blockStart
	fmt.Println("DEBUG: SuperBlock inicializado - S_filesystem_type:", sb.S_filesystem_type, "S_inodes_count:", sb.S_inodes_count, "S_blocks_count:", sb.S_blocks_count, "S_free_inodes_count:", sb.S_free_inodes_count, "S_free_blocks_count:", sb.S_free_blocks_count, "S_bm_inode_start:", sb.S_bm_inode_start, "S_bm_block_start:", sb.S_bm_block_start, "S_inode_start:", sb.S_inode_start, "S_block_start:", sb.S_block_start, "S_first_ino:", sb.S_first_ino, "S_first_blo:", sb.S_first_blo)

	// Crear los bitmaps
	fmt.Println("DEBUG: Creating bitmaps...")
	err = sb.CreateBitMaps(path)
	if err != nil {
		fmt.Println("DEBUG: Error creating bitmaps:", err)
		return fmt.Errorf("error al crear bitmaps: %v", err)
	}
	fmt.Println("DEBUG: Bitmaps created successfully")

	// Crear inodo raíz y archivo users.txt según el sistema de archivos
	if sb.S_filesystem_type == 3 {
		// EXT3
		fmt.Println("DEBUG: Creating filesystem for EXT3...")
		err = sb.CreateUsersFileExt3(path, int64(journalStart))
		if err != nil {
			fmt.Println("DEBUG: Error creating users.txt for EXT3:", err)
			return fmt.Errorf("error al crear users.txt para EXT3: %v", err)
		}
		fmt.Println("DEBUG: EXT3 filesystem created successfully")
	} else {
		// EXT2
		fmt.Println("DEBUG: Creating filesystem for EXT2...")
		err = createRootAndUsersExt2(sb, partition, path)
		if err != nil {
			fmt.Println("DEBUG: Error creating root and users.txt for EXT2:", err)
			return fmt.Errorf("error al crear raíz y users.txt para EXT2: %v", err)
		}
		fmt.Println("DEBUG: EXT2 filesystem created successfully")
	}

	// Actualizar superbloque
	fmt.Println("DEBUG: Serializing superblock...")
	err = sb.Serialize(path, int64(partition.Part_start))
	if err != nil {
		fmt.Println("DEBUG: Error serializing superblock:", err)
		return fmt.Errorf("error al actualizar superbloque: %v", err)
	}
	fmt.Println("DEBUG: Superblock serialized successfully")

	// Sincronizar archivo
	fmt.Println("DEBUG: Opening file for sync:", path)
	file, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		fmt.Println("DEBUG: Error opening file for sync:", err)
		return fmt.Errorf("error al abrir el archivo para sincronizar: %v", err)
	}
	defer file.Close()
	fmt.Println("DEBUG: Syncing file...")
	err = file.Sync()
	if err != nil {
		fmt.Println("DEBUG: Error syncing file:", err)
		return fmt.Errorf("error al sincronizar el archivo: %v", err)
	}
	fmt.Println("DEBUG: File synced successfully")

	return nil
}

// createRootAndUsersExt2 crea el inodo raíz y users.txt para EXT2
func createRootAndUsersExt2(sb *structures.SuperBlock, partition *structures.Partition, path string) error {
	fmt.Println("DEBUG: Creating root inode for EXT2...")
	// Crear inodo raíz
	rootInode := &structures.Inode{
		I_uid:   1, // root user
		I_gid:   1, // root group
		I_size:  0,
		I_atime: float32(time.Now().Unix()),
		I_ctime: float32(time.Now().Unix()),
		I_mtime: float32(time.Now().Unix()),
		I_type:  [1]byte{'0'}, // Carpeta
		I_perm:  [3]byte{'6', '6', '4'},
	}
	for i := 0; i < 15; i++ {
		rootInode.I_block[i] = -1
	}
	rootInode.I_block[0] = sb.S_block_start
	fmt.Println("DEBUG: Root inode created - I_uid:", rootInode.I_uid, "I_gid:", rootInode.I_gid, "I_size:", rootInode.I_size, "I_block[0]:", rootInode.I_block[0])

	// Crear bloque raíz
	fmt.Println("DEBUG: Creating root block for EXT2...")
	rootBlock := &structures.FolderBlock{}
	copy(rootBlock.B_content[0].B_name[:], ".")
	rootBlock.B_content[0].B_inodo = 0
	copy(rootBlock.B_content[1].B_name[:], "..")
	rootBlock.B_content[1].B_inodo = 0
	copy(rootBlock.B_content[2].B_name[:], "users.txt")
	rootBlock.B_content[2].B_inodo = 1
	rootBlock.B_content[3].B_inodo = -1
	fmt.Println("DEBUG: Root block created - Entry 0:", string(rootBlock.B_content[0].B_name[:]), "Inodo:", rootBlock.B_content[0].B_inodo)
	fmt.Println("DEBUG: Root block created - Entry 1:", string(rootBlock.B_content[1].B_name[:]), "Inodo:", rootBlock.B_content[1].B_inodo)
	fmt.Println("DEBUG: Root block created - Entry 2:", string(rootBlock.B_content[2].B_name[:]), "Inodo:", rootBlock.B_content[2].B_inodo)

	// Actualizar contadores y bitmap para el inodo raíz
	sb.S_inodes_count++
	sb.S_free_inodes_count--
	fmt.Println("DEBUG: Updating inode bitmap for root - S_inodes_count:", sb.S_inodes_count, "S_free_inodes_count:", sb.S_free_inodes_count)
	err := sb.UpdateBitmapInode(path, sb.S_first_ino)
	if err != nil {
		fmt.Println("DEBUG: Error updating inode bitmap for root:", err)
		return fmt.Errorf("error al actualizar bitmap para inodo raíz: %v", err)
	}
	sb.S_first_ino += sb.S_inode_size
	fmt.Println("DEBUG: Updated S_first_ino:", sb.S_first_ino)

	fmt.Println("DEBUG: Serializing root inode - Position:", sb.S_inode_start)
	// Serializar inodo y bloque raíz
	err = rootInode.Serialize(path, int64(sb.S_inode_start))
	if err != nil {
		fmt.Println("DEBUG: Error serializing root inode:", err)
		return fmt.Errorf("error al serializar inodo raíz: %v", err)
	}
	fmt.Println("DEBUG: Root inode serialized successfully")

	fmt.Println("DEBUG: Serializing root block - Position:", rootInode.I_block[0])
	err = rootBlock.Serialize(path, int64(rootInode.I_block[0]))
	if err != nil {
		fmt.Println("DEBUG: Error serializing root block:", err)
		return fmt.Errorf("error al serializar bloque raíz: %v", err)
	}
	fmt.Println("DEBUG: Root block serialized successfully")

	// Actualizar bitmap de bloques para el bloque raíz
	sb.S_blocks_count++
	sb.S_free_blocks_count--
	fmt.Println("DEBUG: Updating block bitmap for root - S_blocks_count:", sb.S_blocks_count, "S_free_blocks_count:", sb.S_free_blocks_count)
	sb.S_first_blo += sb.S_block_size
	err = sb.UpdateBitmapBlockAt(path, rootInode.I_block[0], '1')
	if err != nil {
		fmt.Println("DEBUG: Error updating block bitmap for root:", err)
		return fmt.Errorf("error al actualizar bitmap de bloques para raíz: %v", err)
	}
	fmt.Println("DEBUG: Updated S_first_blo:", sb.S_first_blo)

	// Crear inodo para users.txt
	fmt.Println("DEBUG: Creating users.txt inode for EXT2...")
	usersInode := &structures.Inode{
		I_uid:   1,
		I_gid:   1,
		I_size:  0,
		I_atime: float32(time.Now().Unix()),
		I_ctime: float32(time.Now().Unix()),
		I_mtime: float32(time.Now().Unix()),
		I_type:  [1]byte{'1'}, // Archivo
		I_perm:  [3]byte{'6', '6', '4'},
	}
	for i := 0; i < 15; i++ {
		usersInode.I_block[i] = -1
	}
	usersInode.I_block[0] = sb.S_first_blo
	fmt.Println("DEBUG: Users.txt inode created - I_uid:", usersInode.I_uid, "I_gid:", usersInode.I_gid, "I_size:", usersInode.I_size, "I_block[0]:", usersInode.I_block[0])

	// Actualizar contadores y bitmap para users.txt
	sb.S_inodes_count++
	sb.S_free_inodes_count--
	fmt.Println("DEBUG: Updating inode bitmap for users.txt - S_inodes_count:", sb.S_inodes_count, "S_free_inodes_count:", sb.S_free_inodes_count)
	err = sb.UpdateBitmapInode(path, sb.S_first_ino)
	if err != nil {
		fmt.Println("DEBUG: Error updating inode bitmap for users.txt:", err)
		return fmt.Errorf("error al actualizar bitmap para users.txt: %v", err)
	}
	sb.S_first_ino += sb.S_inode_size
	fmt.Println("DEBUG: Updated S_first_ino:", sb.S_first_ino)

	fmt.Println("DEBUG: Serializing users.txt inode - Position:", sb.S_inode_start+sb.S_inode_size)
	// Serializar inodo users.txt (antes de escribir el contenido)
	err = usersInode.Serialize(path, int64(sb.S_inode_start+sb.S_inode_size))
	if err != nil {
		fmt.Println("DEBUG: Error serializing users.txt inode:", err)
		return fmt.Errorf("error al serializar inodo users.txt: %v", err)
	}
	fmt.Println("DEBUG: Users.txt inode serialized successfully")

	// Escribir contenido inicial en users.txt
	content := []byte("1,G,root\n1,U,root,root,123\n")
	usersInode.I_size = int32(len(content))
	usersBlock := &structures.FileBlock{}
	copy(usersBlock.B_content[:], content)
	fmt.Println("DEBUG: Writing users.txt content:", string(content))
	fmt.Println("DEBUG: Users.txt size set to:", usersInode.I_size)
	fmt.Println("DEBUG: Users.txt block position:", usersInode.I_block[0])

	// Serializar el bloque de users.txt
	fmt.Println("DEBUG: Serializing users.txt block - Position:", usersInode.I_block[0])
	err = usersBlock.Serialize(path, int64(usersInode.I_block[0]))
	if err != nil {
		fmt.Println("DEBUG: Error serializing users.txt block:", err)
		return fmt.Errorf("error al serializar bloque de users.txt: %v", err)
	}
	fmt.Println("DEBUG: Users.txt block serialized successfully")

	// Volver a serializar el inodo de users.txt para reflejar el tamaño actualizado
	fmt.Println("DEBUG: Re-serializing users.txt inode - Position:", sb.S_inode_start+sb.S_inode_size)
	err = usersInode.Serialize(path, int64(sb.S_inode_start+sb.S_inode_size))
	if err != nil {
		fmt.Println("DEBUG: Error re-serializing users.txt inode:", err)
		return fmt.Errorf("error al serializar inodo users.txt después de escribir contenido: %v", err)
	}
	fmt.Println("DEBUG: Users.txt inode re-serialized successfully")

	// Actualizar bitmap de bloques para users.txt
	sb.S_blocks_count++
	sb.S_free_blocks_count--
	fmt.Println("DEBUG: Updating block bitmap for users.txt - S_blocks_count:", sb.S_blocks_count, "S_free_blocks_count:", sb.S_free_blocks_count)
	sb.S_first_blo += sb.S_block_size
	fmt.Println("DEBUG: Updated S_first_blo:", sb.S_first_blo)
	err = sb.UpdateBitmapBlockAt(path, usersInode.I_block[0], '1')
	if err != nil {
		fmt.Println("DEBUG: Error updating block bitmap for users.txt:", err)
		return fmt.Errorf("error al actualizar bitmap de bloques para users.txt: %v", err)
	}
	fmt.Println("DEBUG: Block bitmap for users.txt updated successfully")

	return nil
}

func calculateN(partition *structures.Partition, fs string) int32 {
	fmt.Println("DEBUG: Calculating n for partition size:", partition.Part_size, "fs:", fs)
	numerator := int(partition.Part_size) - binary.Size(structures.SuperBlock{})
	fmt.Println("DEBUG: Numerator:", numerator)
	baseDenominator := 4 + binary.Size(structures.Inode{}) + 3*binary.Size(structures.FileBlock{})
	fmt.Println("DEBUG: Base denominator:", baseDenominator)
	temp := 0
	if fs == "3fs" {
		temp = binary.Size(structures.Journal{})
		fmt.Println("DEBUG: Added journal size for EXT3:", temp)
	}
	denominator := baseDenominator + temp
	fmt.Println("DEBUG: Final denominator:", denominator)
	n := math.Floor(float64(numerator) / float64(denominator))
	fmt.Println("DEBUG: Calculated n:", int32(n))
	return int32(n)
}

func calculateStartPositions(partition *structures.Partition, fs string, n int32) (int32, int32, int32, int32, int32) {
	fmt.Println("DEBUG: Calculating start positions for fs:", fs, "n:", n)
	superblockSize := int32(binary.Size(structures.SuperBlock{}))
	journalSize := int32(binary.Size(structures.Journal{}))
	inodeSize := int32(binary.Size(structures.Inode{}))
	fmt.Println("DEBUG: Sizes - Superblock:", superblockSize, "Journal:", journalSize, "Inode:", inodeSize)

	// EXT2
	journalStart := int32(0)
	bmInodeStart := partition.Part_start + superblockSize
	bmBlockStart := bmInodeStart + n
	inodeStart := bmBlockStart + (3 * n)
	blockStart := inodeStart + (inodeSize * n)
	fmt.Println("DEBUG: EXT2 positions - Journal Start:", journalStart, "Bitmap Inode Start:", bmInodeStart, "Bitmap Block Start:", bmBlockStart, "Inode Start:", inodeStart, "Block Start:", blockStart)

	// Ajustar para EXT3
	if fs == "3fs" {
		journalStart = partition.Part_start + superblockSize
		bmInodeStart = journalStart + (journalSize * n)
		bmBlockStart = bmInodeStart + n
		inodeStart = bmBlockStart + (3 * n)
		blockStart = inodeStart + (inodeSize * n)
		fmt.Println("DEBUG: EXT3 positions - Journal Start:", journalStart, "Bitmap Inode Start:", bmInodeStart, "Bitmap Block Start:", bmBlockStart, "Inode Start:", inodeStart, "Block Start:", blockStart)
	}

	return journalStart, bmInodeStart, bmBlockStart, inodeStart, blockStart
}

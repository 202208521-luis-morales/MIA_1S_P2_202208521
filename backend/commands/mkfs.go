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
	id string // ID de la partición a formatear
}

// ParseMkfs parsea el comando mkfs y formatea la partición
func ParseMkfs(tokens []string) (string, error) {
	cmd := &MKFS{}
	args := strings.Join(tokens, " ")
	// Expresión regular para capturar los parámetros válidos, insensible a mayúsculas/minúsculas
	re := regexp.MustCompile(`(?i)-id=[^\s]+`)
	matches := re.FindAllString(args, -1)

	// Validar que no haya parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	allowedParams := map[string]bool{
		"-id":   true,
		"-type": true,
	}

	for _, param := range allParams {
		// Extraer solo el nombre del parámetro (sin el valor) y convertir a minúsculas
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
		// Convertir el nombre del parámetro a minúsculas para la comparación
		key, value := strings.ToLower(kv[0]), kv[1]
		if key == "-id" {
			if value == "" {
				return "", errors.New("el ID no puede estar vacío")
			}
			cmd.id = value
		}
	}

	if cmd.id == "" {
		return "", errors.New("faltan parámetros requeridos: -id")
	}

	err := commandMkfs(cmd.id) // Pasar cmd.id en lugar de cmd
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("MKFS: Sistema de archivos creado exitosamente\n-> ID: %s", cmd.id), nil
}

// commandMkfs ejecuta la lógica para formatear la partición
func commandMkfs(id string) error {
	fmt.Println("\nInicia commandMkfs")
	// Obtener el superbloque y la partición
	sb, partition, path, err := stores.GetMountedPartitionSuperblock(id)
	if err != nil {
		return fmt.Errorf("error al obtener partición: %v", err)
	}

	n := calculateN(partition)
	// Inicializar el superbloque
	sb.S_inodes_count = 0
	sb.S_free_inodes_count = n // Ajusta según el cálculo real
	sb.S_blocks_count = 0
	sb.S_free_blocks_count = n * 3 // Ajusta según el cálculo real
	sb.S_inode_size = int32(unsafe.Sizeof(structures.Inode{}))
	sb.S_block_size = int32(unsafe.Sizeof(structures.FileBlock{}))
	sb.S_bm_inode_start = int32(partition.Part_start) + int32(unsafe.Sizeof(*sb))
	sb.S_bm_block_start = sb.S_bm_inode_start + sb.S_free_inodes_count
	sb.S_inode_start = sb.S_bm_block_start + sb.S_free_blocks_count
	sb.S_block_start = sb.S_inode_start + (sb.S_free_inodes_count * sb.S_inode_size)
	sb.S_first_ino = 0
	sb.S_first_blo = sb.S_block_start
	sb.S_mtime = float32(time.Now().Unix())
	sb.S_umtime = float32(time.Now().Unix())

	fmt.Println("DEBUG: SuperBlock inicializado - S_bm_inode_start:", sb.S_bm_inode_start, "S_bm_block_start:", sb.S_bm_block_start, "S_inode_start:", sb.S_inode_start, "S_block_start:", sb.S_block_start)

	// Crear los bitmaps
	err = sb.CreateBitMaps(path)
	if err != nil {
		return fmt.Errorf("error al crear bitmaps: %v", err)
	}

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
	fmt.Println("DEBUG: Inodo raíz creado - I_block[0]:", rootInode.I_block[0])

	// Crear bloque raíz
	rootBlock := &structures.FolderBlock{}
	copy(rootBlock.B_content[0].B_name[:], ".")
	rootBlock.B_content[0].B_inodo = 0
	copy(rootBlock.B_content[1].B_name[:], "..")
	rootBlock.B_content[1].B_inodo = 0
	copy(rootBlock.B_content[2].B_name[:], "users.txt")
	rootBlock.B_content[2].B_inodo = 1
	rootBlock.B_content[3].B_inodo = -1
	fmt.Println("DEBUG: Bloque raíz creado - Entrada 0:", string(rootBlock.B_content[0].B_name[:]), "Inodo:", rootBlock.B_content[0].B_inodo)
	fmt.Println("DEBUG: Bloque raíz creado - Entrada 1:", string(rootBlock.B_content[1].B_name[:]), "Inodo:", rootBlock.B_content[1].B_inodo)
	fmt.Println("DEBUG: Bloque raíz creado - Entrada 2:", string(rootBlock.B_content[2].B_name[:]), "Inodo:", rootBlock.B_content[2].B_inodo)

	// Actualizar contadores y bitmap para el inodo raíz
	sb.S_inodes_count++
	sb.S_free_inodes_count--
	err = sb.UpdateBitmapInode(path, sb.S_first_ino) // Marcar inodo 0 como ocupado
	if err != nil {
		return fmt.Errorf("error al actualizar bitmap para inodo raíz: %v", err)
	}
	sb.S_first_ino++

	fmt.Println("DEBUG: POSICION INODO RAIZ: sb.S_inode_start = ", sb.S_inode_start)
	// Serializar inodo y bloque raíz
	err = rootInode.Serialize(path, int64(sb.S_inode_start))
	if err != nil {
		return fmt.Errorf("error al serializar inodo raíz: %v", err)
	}
	err = rootBlock.Serialize(path, int64(rootInode.I_block[0]))
	if err != nil {
		return fmt.Errorf("error al serializar bloque raíz: %v", err)
	}

	// Actualizar bitmap de bloques para el bloque raíz
	sb.S_blocks_count++
	sb.S_free_blocks_count--
	sb.S_first_blo += sb.S_block_size
	err = sb.UpdateBitmapBlockAt(path, rootInode.I_block[0], '1')
	if err != nil {
		return fmt.Errorf("error al actualizar bitmap de bloques para raíz: %v", err)
	}

	// Crear inodo para users.txt
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

	// Actualizar contadores y bitmap para users.txt
	sb.S_inodes_count++
	sb.S_free_inodes_count--
	err = sb.UpdateBitmapInode(path, sb.S_first_ino) // Marcar inodo 1 como ocupado
	if err != nil {
		return fmt.Errorf("error al actualizar bitmap para users.txt: %v", err)
	}
	sb.S_first_ino++

	fmt.Println("DEBUG: sb.S_inode_start+sb.S_inode_size = ", sb.S_inode_start+sb.S_inode_size)
	// Serializar inodo users.txt (antes de escribir el contenido)
	err = usersInode.Serialize(path, int64(sb.S_inode_start+sb.S_inode_size))
	if err != nil {
		return fmt.Errorf("error al serializar inodo users.txt: %v", err)
	}

	// Escribir contenido inicial en users.txt
	content := []byte("1,G,root\n1,U,root,root,123\n")
	usersInode.I_size = int32(len(content))
	usersBlock := &structures.FileBlock{}
	copy(usersBlock.B_content[:], content)
	fmt.Println("DEBUG: Contenido de users.txt a escribir:", string(content))
	fmt.Println("DEBUG: Tamaño de users.txt (I_size):", usersInode.I_size)
	fmt.Println("DEBUG: Bloque de users.txt - I_block[0]:", usersInode.I_block[0])

	// Serializar el bloque de users.txt
	err = usersBlock.Serialize(path, int64(usersInode.I_block[0]))
	if err != nil {
		return fmt.Errorf("error al serializar bloque de users.txt: %v", err)
	}

	// Volver a serializar el inodo de users.txt para reflejar el tamaño actualizado
	err = usersInode.Serialize(path, int64(sb.S_inode_start+sb.S_inode_size))
	if err != nil {
		return fmt.Errorf("error al serializar inodo users.txt después de escribir contenido: %v", err)
	}

	// Actualizar bitmap de bloques para users.txt
	sb.S_blocks_count++
	sb.S_free_blocks_count--
	sb.S_first_blo += sb.S_block_size
	fmt.Println("sb.S_first_blo: ", sb.S_first_blo)
	err = sb.UpdateBitmapBlockAt(path, usersInode.I_block[0], '1')
	if err != nil {
		return fmt.Errorf("error al actualizar bitmap de bloques para users.txt: %v", err)
	}

	// Actualizar superbloque
	err = sb.Serialize(path, int64(partition.Part_start))
	if err != nil {
		return fmt.Errorf("error al actualizar superbloque: %v", err)
	}

	// Abrir el archivo para sincronizar
	file, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("error al abrir el archivo para sincronizar: %v", err)
	}
	defer file.Close()
	err = file.Sync()
	if err != nil {
		return fmt.Errorf("error al sincronizar el archivo: %v", err)
	}

	return nil
}

func calculateN(partition *structures.Partition) int32 {
	/*
		numerador = (partition_montada.size - sizeof(Structs::Superblock)
		denominador base = (4 + sizeof(Structs::Inodes) + 3 * sizeof(Structs::Fileblock))
		n = floor(numerador / denominador)
	*/

	numerator := int(partition.Part_size) - binary.Size(structures.SuperBlock{})
	denominator := 4 + binary.Size(structures.Inode{}) + 3*binary.Size(structures.FileBlock{}) // No importa que bloque poner, ya que todos tienen el mismo tamaño
	n := math.Floor(float64(numerator) / float64(denominator))

	return int32(n)
}

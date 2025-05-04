package structures

import (
	"encoding/binary"
	"fmt"
	"os"
)

// CreateBitMaps crea los Bitmaps de inodos y bloques en el archivo especificado
func (sb *SuperBlock) CreateBitMaps(path string) error {
	fmt.Println("\nDEBUG: Iniciando CreateBitMaps - Path:", path)
	fmt.Println("DEBUG: S_bm_inode_start:", sb.S_bm_inode_start, "S_bm_block_start:", sb.S_bm_block_start)
	fmt.Println("DEBUG: S_inodes_count:", sb.S_inodes_count, "S_free_inodes_count:", sb.S_free_inodes_count)
	fmt.Println("DEBUG: S_blocks_count:", sb.S_blocks_count, "S_free_blocks_count:", sb.S_free_blocks_count)

	// Escribir Bitmaps
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644) // Cambiar a os.O_RDWR
	if err != nil {
		return fmt.Errorf("error al abrir el archivo: %v", err)
	}
	defer file.Close()

	// Bitmap de inodos
	fmt.Println("DEBUG: Inicializando bitmap de inodos")
	_, err = file.Seek(int64(sb.S_bm_inode_start), 0)
	if err != nil {
		return fmt.Errorf("error al posicionar el puntero en S_bm_inode_start: %v", err)
	}
	fmt.Println("DEBUG: Puntero posicionado en S_bm_inode_start:", sb.S_bm_inode_start)

	// Calcular el número total de inodos
	totalInodes := sb.S_inodes_count + sb.S_free_inodes_count
	fmt.Println("DEBUG: Total de inodos a inicializar:", totalInodes)

	// Crear un buffer de '0' para todos los inodos
	buffer := make([]byte, totalInodes)
	for i := range buffer {
		buffer[i] = '0'
	}
	fmt.Println("DEBUG: Buffer creado con", len(buffer), "bytes, todos '0'")

	// Escribir el buffer en el archivo
	err = binary.Write(file, binary.LittleEndian, buffer)
	if err != nil {
		return fmt.Errorf("error al escribir el bitmap de inodos: %v", err)
	}
	fmt.Println("DEBUG: Bitmap de inodos escrito -", len(buffer), "bytes")

	// Sincronizar los cambios en el disco
	err = file.Sync()
	if err != nil {
		return fmt.Errorf("error al sincronizar el bitmap de inodos: %v", err)
	}

	// Verificar que se escribió correctamente
	_, err = file.Seek(int64(sb.S_bm_inode_start), 0)
	if err != nil {
		return fmt.Errorf("error al reposicionar el puntero para verificar el bitmap de inodos: %v", err)
	}
	verifyBuffer := make([]byte, totalInodes)
	n, err := file.Read(verifyBuffer)
	if err != nil {
		return fmt.Errorf("error al verificar el bitmap de inodos: %v", err)
	}
	fmt.Println("DEBUG: Verificación - Bytes leídos:", n)
	for i := 0; i < 20 && i < len(verifyBuffer); i++ {
		fmt.Printf("DEBUG: Byte %d: %c (ASCII: %d)\n", i, verifyBuffer[i], verifyBuffer[i])
	}

	// Bitmap de bloques
	fmt.Println("DEBUG: Inicializando bitmap de bloques")
	_, err = file.Seek(int64(sb.S_bm_block_start), 0)
	if err != nil {
		return fmt.Errorf("error al posicionar el puntero en S_bm_block_start: %v", err)
	}
	fmt.Println("DEBUG: Puntero posicionado en S_bm_block_start:", sb.S_bm_block_start)

	// Calcular el número total de bloques
	totalBlocks := sb.S_blocks_count + sb.S_free_blocks_count
	fmt.Println("DEBUG: Total de bloques a inicializar:", totalBlocks)

	// Crear un buffer de '0' para todos los bloques
	buffer = make([]byte, totalBlocks)
	for i := range buffer {
		buffer[i] = '0'
	}
	fmt.Println("DEBUG: Buffer creado con", len(buffer), "bytes, todos '0'")

	// Escribir el buffer en el archivo
	err = binary.Write(file, binary.LittleEndian, buffer)
	if err != nil {
		return fmt.Errorf("error al escribir el bitmap de bloques: %v", err)
	}
	fmt.Println("DEBUG: Bitmap de bloques escrito -", len(buffer), "bytes")

	// Sincronizar los cambios en el disco
	err = file.Sync()
	if err != nil {
		return fmt.Errorf("error al sincronizar el bitmap de bloques: %v", err)
	}

	// Verificar que se escribió correctamente
	_, err = file.Seek(int64(sb.S_bm_block_start), 0)
	if err != nil {
		return fmt.Errorf("error al reposicionar el puntero para verificar el bitmap de bloques: %v", err)
	}
	verifyBuffer = make([]byte, totalBlocks)
	n, err = file.Read(verifyBuffer)
	if err != nil {
		return fmt.Errorf("error al verificar el bitmap de bloques: %v", err)
	}
	fmt.Println("DEBUG: Verificación - Bytes leídos:", n)
	for i := 0; i < 20 && i < len(verifyBuffer); i++ {
		fmt.Printf("DEBUG: Byte %d: %c (ASCII: %d)\n", i, verifyBuffer[i], verifyBuffer[i])
	}

	return nil
}

// UpdateBitmapInode actualiza el siguiente inodo disponible en el bitmap
func (sb *SuperBlock) UpdateBitmapInode(path string, inodeIndex int32) error {
	fmt.Println("DEBUG: Iniciando UpdateBitmapInode - Path:", path)
	fmt.Println("DEBUG: S_bm_inode_start:", sb.S_bm_inode_start, "Inode Index:", inodeIndex)

	// Calcular la posición del inodo
	position := int64(sb.S_bm_inode_start) + int64(inodeIndex)
	fmt.Println("DEBUG: Calculando posición - Posición absoluta:", position)

	// Abrir el archivo
	file, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("error al abrir el archivo: %v", err)
	}
	defer file.Close()

	// Posicionar el puntero
	_, err = file.Seek(position, 0)
	if err != nil {
		return fmt.Errorf("error al posicionar el puntero: %v", err)
	}
	fmt.Println("DEBUG: Puntero posicionado en:", position)

	// Leer el valor actual
	buffer := make([]byte, 1)
	_, err = file.Read(buffer)
	if err != nil {
		return fmt.Errorf("error al leer el bitmap: %v", err)
	}
	fmt.Println("DEBUG: Valor actual en posición", position, ":", string(buffer[0]), "(ASCII:", buffer[0], ")")

	// Escribir '1' para marcar el inodo como ocupado
	_, err = file.Seek(position, 0)
	if err != nil {
		return fmt.Errorf("error al reposicionar el puntero: %v", err)
	}
	_, err = file.Write([]byte{'1'})
	if err != nil {
		return fmt.Errorf("error al escribir en el bitmap: %v", err)
	}
	fmt.Println("DEBUG: Escrito '1' en posición:", position)

	// Verificar que se escribió correctamente
	_, err = file.Seek(position, 0)
	if err != nil {
		return fmt.Errorf("error al reposicionar el puntero para verificar: %v", err)
	}
	_, err = file.Read(buffer)
	if err != nil {
		return fmt.Errorf("error al verificar el bitmap: %v", err)
	}
	fmt.Println("DEBUG: Verificación - Valor escrito en posición", position, ":", string(buffer[0]), "(ASCII:", buffer[0], ")")

	// Sincronizar los cambios
	err = file.Sync()
	if err != nil {
		return fmt.Errorf("error al sincronizar el archivo: %v", err)
	}

	return nil
}

// UpdateBitmapBlock actualiza el siguiente bloque disponible en el bitmap
func (sb *SuperBlock) UpdateBitmapBlock(path string) error {
	// Abrir el archivo
	file, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Calcular la posición relativa del último bloque asignado
	// S_first_blo ya se incrementó, así que usamos S_first_blo - S_block_size
	blockIndex := (sb.S_first_blo - sb.S_block_size - sb.S_block_start) / sb.S_block_size
	position := int64(sb.S_bm_block_start) + int64(blockIndex)
	_, err = file.Seek(position, 0)
	if err != nil {
		return err
	}

	// Escribir el bit '1' en el archivo
	_, err = file.Write([]byte{'1'}) // Usar '1' en lugar de 'X'
	if err != nil {
		return err
	}

	fmt.Println("DEBUG: Bitmap de bloques actualizado - Posición:", position, "Valor: 1")
	return nil
}

// UpdateBitmapBlockAt actualiza un bit específico en el bitmap de bloques
func (sb *SuperBlock) UpdateBitmapBlockAt(path string, blockIndex int32, value byte) error {
	// Abrir el archivo
	file, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Calcular la posición relativa en el bitmap (blockIndex es el offset desde S_block_start)
	bitmapPos := int64(sb.S_bm_block_start) + int64((blockIndex-sb.S_block_start)/sb.S_block_size)
	if bitmapPos < int64(sb.S_bm_block_start) || bitmapPos >= int64(sb.S_bm_block_start+sb.S_free_blocks_count+sb.S_blocks_count) {
		return fmt.Errorf("índice de bloque %d fuera del rango del bitmap", blockIndex)
	}

	// Mover el puntero del archivo a la posición específica
	_, err = file.Seek(bitmapPos, 0)
	if err != nil {
		return err
	}

	// Escribir el bit (0 para libre, 1 para usado)
	if value != '0' && value != '1' { // Cambiar 'X' por '1'
		return fmt.Errorf("valor inválido para bitmap: %c, debe ser '0' o '1'", value)
	}
	_, err = file.Write([]byte{value})
	if err != nil {
		return err
	}

	fmt.Println("DEBUG: Bitmap de bloques actualizado - Posición:", bitmapPos, "Valor:", string(value))
	return nil
}

// SetBitmapInode actualiza el bitmap de inodos en la posición especificada con el valor dado ('1' para ocupar, '0' para liberar)
func (sb *SuperBlock) SetBitmapInode(path string, inodeIndex int32, value byte) error {
	fmt.Println("DEBUG: Iniciando SetBitmapInode - Path:", path, "Inode Index:", inodeIndex, "Value:", string(value))

	// Validar el valor
	if value != '0' && value != '1' {
		return fmt.Errorf("valor inválido para el bitmap de inodos: %v (debe ser '0' o '1')", value)
	}

	// Calcular la posición del inodo
	position := int64(sb.S_bm_inode_start) + int64(inodeIndex)
	fmt.Println("DEBUG: Calculando posición - Posición absoluta:", position)

	// Abrir el archivo
	file, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("error al abrir el archivo: %v", err)
	}
	defer file.Close()

	// Posicionar el puntero
	_, err = file.Seek(position, 0)
	if err != nil {
		return fmt.Errorf("error al posicionar el puntero: %v", err)
	}
	fmt.Println("DEBUG: Puntero posicionado en:", position)

	// Leer el valor actual (para depuración)
	buffer := make([]byte, 1)
	_, err = file.Read(buffer)
	if err != nil {
		return fmt.Errorf("error al leer el bitmap: %v", err)
	}
	fmt.Println("DEBUG: Valor actual en posición", position, ":", string(buffer[0]), "(ASCII:", buffer[0], ")")

	// Escribir el valor especificado
	_, err = file.Seek(position, 0)
	if err != nil {
		return fmt.Errorf("error al reposicionar el puntero: %v", err)
	}
	_, err = file.Write([]byte{value})
	if err != nil {
		return fmt.Errorf("error al escribir en el bitmap: %v", err)
	}
	fmt.Println("DEBUG: Escrito", string(value), "en posición:", position)

	// Verificar que se escribió correctamente
	_, err = file.Seek(position, 0)
	if err != nil {
		return fmt.Errorf("error al reposicionar el puntero para verificar: %v", err)
	}
	_, err = file.Read(buffer)
	if err != nil {
		return fmt.Errorf("error al verificar el bitmap: %v", err)
	}
	fmt.Println("DEBUG: Verificación - Valor escrito en posición", position, ":", string(buffer[0]), "(ASCII:", buffer[0], ")")

	// Sincronizar los cambios
	err = file.Sync()
	if err != nil {
		return fmt.Errorf("error al sincronizar el archivo: %v", err)
	}

	return nil
}

// SetBitmapBlock actualiza el bitmap de bloques en la posición especificada con el valor dado ('1' para ocupar, '0' para liberar)
func (sb *SuperBlock) SetBitmapBlock(path string, blockIndex int32, value byte) error {
	fmt.Println("DEBUG: Iniciando SetBitmapBlock - Path:", path, "Block Index:", blockIndex, "Value:", string(value))

	// Validar el valor
	if value != '0' && value != '1' {
		return fmt.Errorf("valor inválido para el bitmap de bloques: %v (debe ser '0' o '1')", value)
	}

	// Calcular la posición del bloque
	position := int64(sb.S_bm_block_start) + int64(blockIndex-sb.S_block_start)/int64(sb.S_block_size)
	fmt.Println("DEBUG: Calculando posición - Posición absoluta:", position)

	// Abrir el archivo
	file, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("error al abrir el archivo: %v", err)
	}
	defer file.Close()

	// Posicionar el puntero
	_, err = file.Seek(position, 0)
	if err != nil {
		return fmt.Errorf("error al posicionar el puntero: %v", err)
	}
	fmt.Println("DEBUG: Puntero posicionado en:", position)

	// Leer el valor actual (para depuración)
	buffer := make([]byte, 1)
	_, err = file.Read(buffer)
	if err != nil {
		return fmt.Errorf("error al leer el bitmap: %v", err)
	}
	fmt.Println("DEBUG: Valor actual en posición", position, ":", string(buffer[0]), "(ASCII:", buffer[0], ")")

	// Escribir el valor especificado
	_, err = file.Seek(position, 0)
	if err != nil {
		return fmt.Errorf("error al reposicionar el puntero: %v", err)
	}
	_, err = file.Write([]byte{value})
	if err != nil {
		return fmt.Errorf("error al escribir en el bitmap: %v", err)
	}
	fmt.Println("DEBUG: Escrito", string(value), "en posición:", position)

	// Verificar que se escribió correctamente
	_, err = file.Seek(position, 0)
	if err != nil {
		return fmt.Errorf("error al reposicionar el puntero para verificar: %v", err)
	}
	_, err = file.Read(buffer)
	if err != nil {
		return fmt.Errorf("error al verificar el bitmap: %v", err)
	}
	fmt.Println("DEBUG: Verificación - Valor escrito en posición", position, ":", string(buffer[0]), "(ASCII:", buffer[0], ")")

	// Sincronizar los cambios
	err = file.Sync()
	if err != nil {
		return fmt.Errorf("error al sincronizar el archivo: %v", err)
	}

	return nil
}

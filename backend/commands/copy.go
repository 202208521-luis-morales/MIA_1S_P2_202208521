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
	destino string // Ruta del destino (debe ser una carpeta)
}

// ParseCopy parsea el comando copy y devuelve el resultado de la copia
func ParseCopy(tokens []string) (string, error) {
	cmd := &COPY{}
	args := strings.Join(tokens, " ")
	fmt.Println("DEBUG: Iniciando ParseCopy - Tokens:", tokens)
	fmt.Println("DEBUG: Args combinados:", args)

	re := regexp.MustCompile(`(?i)-path="[^"]+"|(?i)-path=[^\s]+|(?i)-destino="[^"]+"|(?i)-destino=[^\s]+`)
	matches := re.FindAllString(args, -1)
	fmt.Println("DEBUG: Coincidencias encontradas:", matches)

	// Validar parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	fmt.Println("DEBUG: Todos los parámetros encontrados:", allParams)
	allowedParams := map[string]bool{
		"-path":    true,
		"-destino": true,
	}

	for _, param := range allParams {
		paramName := strings.ToLower(strings.SplitN(param, "=", 2)[0])
		fmt.Println("DEBUG: Verificando parámetro:", paramName)
		if !allowedParams[paramName] {
			fmt.Println("DEBUG: Error - Parámetro no permitido:", paramName)
			return "", fmt.Errorf("parámetro no permitido: %s", paramName)
		}
	}

	for _, match := range matches {
		fmt.Println("DEBUG: Procesando coincidencia:", match)
		kv := strings.SplitN(match, "=", 2)
		if len(kv) != 2 {
			fmt.Println("DEBUG: Error - Formato de parámetro inválido:", match)
			return "", fmt.Errorf("formato de parámetro inválido: %s", match)
		}
		key := strings.ToLower(kv[0])
		value := strings.Trim(kv[1], "\"")
		fmt.Println("DEBUG: Clave:", key, "Valor:", value)

		switch key {
		case "-path":
			if value == "" {
				fmt.Println("DEBUG: Error - La ruta no puede estar vacía")
				return "", errors.New("la ruta no puede estar vacía")
			}
			cmd.path = value
			fmt.Println("DEBUG: Ruta establecida:", cmd.path)
		case "-destino":
			if value == "" {
				fmt.Println("DEBUG: Error - La ruta de destino no puede estar vacía")
				return "", errors.New("la ruta de destino no puede estar vacía")
			}
			cmd.destino = value
			fmt.Println("DEBUG: Ruta de destino establecida:", cmd.destino)
		}
	}

	if cmd.path == "" || cmd.destino == "" {
		fmt.Println("DEBUG: Error - Falta alguno de los parámetros requeridos: -path o -destino")
		return "", fmt.Errorf("faltan parámetros requeridos: -path y -destino son obligatorios")
	}

	fmt.Println("DEBUG: Ejecutando commandCopy con cmd:", cmd)
	err := commandCopy(cmd)
	if err != nil {
		fmt.Println("DEBUG: Error en commandCopy:", err)
		return "", err
	}
	fmt.Println("DEBUG: commandCopy ejecutado exitosamente")
	return fmt.Sprintf("COPY: Elemento copiado exitosamente\n-> Origen: %s\n-> Destino: %s", cmd.path, cmd.destino), nil
}

// commandCopy realiza la copia del archivo o carpeta
func commandCopy(cmd *COPY) error {
	fmt.Println("\nDEBUG: Iniciando commandCopy - Path:", cmd.path, "Destino:", cmd.destino)

	// Verificar sesión activa
	fmt.Println("DEBUG: Verificando sesión activa...")
	if stores.ActiveSession == nil {
		fmt.Println("DEBUG: Error - No hay sesión activa")
		return errors.New("debe iniciar sesión primero para copiar un elemento")
	}
	fmt.Println("DEBUG: Sesión activa encontrada - PartitionID:", stores.ActiveSession.PartitionID)

	// Obtener superbloque y partición montada
	fmt.Println("DEBUG: Obteniendo superbloque y partición...")
	sb, partition, path, err := stores.GetMountedPartitionSuperblock(stores.ActiveSession.PartitionID)
	if err != nil {
		fmt.Println("DEBUG: Error al obtener partición:", err)
		return fmt.Errorf("error al obtener partición: %v", err)
	}
	fmt.Println("DEBUG: Superbloque y partición obtenidos - Path:", path, "Part_start:", partition.Part_start)
	fmt.Println("DEBUG: SuperBlock - S_inode_start:", sb.S_inode_start, "S_block_start:", sb.S_block_start)

	// Normalizar las rutas
	fmt.Println("DEBUG: Normalizando rutas - Path:", cmd.path, "Destino:", cmd.destino)
	cmd.path = filepath.Clean(cmd.path)
	cmd.destino = filepath.Clean(cmd.destino)
	fmt.Println("DEBUG: Rutas normalizadas - Path:", cmd.path, "Destino:", cmd.destino)
	if !strings.HasPrefix(cmd.path, "/") || !strings.HasPrefix(cmd.destino, "/") {
		fmt.Println("DEBUG: Error - Las rutas no son absolutas")
		return errors.New("las rutas deben ser absolutas (comenzar con /)")
	}

	// Separar directorio padre y nombre del elemento a copiar
	fmt.Println("DEBUG: Separando directorio padre y nombre del origen...")
	srcDir, srcName := filepath.Split(cmd.path)
	fmt.Println("DEBUG: Directorio padre origen:", srcDir, "Nombre del elemento:", srcName)
	if srcName == "" {
		fmt.Println("DEBUG: Error - Nombre del elemento a copiar vacío")
		return errors.New("el nombre del elemento a copiar no puede estar vacío")
	}

	// Verificar que el elemento fuente exista
	fmt.Println("DEBUG: Buscando directorio padre del origen:", srcDir)
	_, srcParentBlock, _, err := findParentDir(sb, partition, path, srcDir)
	if err != nil {
		fmt.Println("DEBUG: Error al buscar directorio padre del origen:", err)
		return fmt.Errorf("error al buscar directorio padre del origen: %v", err)
	}
	fmt.Println("DEBUG: Directorio padre del origen encontrado - Entrada 0:", string(srcParentBlock.B_content[0].B_name[:]))

	var srcInodeIndex int32 = -1
	for i, content := range srcParentBlock.B_content {
		name := strings.Trim(string(content.B_name[:]), "\x00")
		fmt.Printf("DEBUG: Entrada %d - Nombre: %q, Inodo: %d\n", i, name, content.B_inodo)
		if name == srcName {
			srcInodeIndex = content.B_inodo
			fmt.Println("DEBUG: Elemento origen encontrado - Inodo:", srcInodeIndex)
			break
		}
	}

	if srcInodeIndex == -1 {
		fmt.Println("DEBUG: Error - Elemento no encontrado:", srcName)
		return fmt.Errorf("el elemento %s no existe en la ruta especificada", cmd.path)
	}

	// Leer el inodo del elemento fuente
	fmt.Println("DEBUG: Leyendo inodo del origen - Inodo:", srcInodeIndex)
	srcInode := &structures.Inode{}
	inodePosition := int64(sb.S_inode_start) + int64(srcInodeIndex)*int64(sb.S_inode_size)
	fmt.Println("DEBUG: Deserializando inodo - Posición:", inodePosition)
	err = srcInode.Deserialize(path, inodePosition)
	if err != nil {
		fmt.Println("DEBUG: Error al leer inodo del origen:", err)
		return fmt.Errorf("error al leer inodo del origen: %v", err)
	}
	fmt.Println("DEBUG: Inodo deserializado - I_type:", string(srcInode.I_type[:]), "I_size:", srcInode.I_size)

	// Verificar permisos de lectura en el elemento fuente
	fmt.Println("DEBUG: Verificando permisos de lectura en el origen...")
	if stores.ActiveSession.Username != "root" {
		hasReadPermission := checkReadPermission(srcInode)
		fmt.Println("DEBUG: Permiso de lectura:", hasReadPermission)
		if !hasReadPermission {
			fmt.Println("DEBUG: Error - Permiso de lectura denegado en el origen")
			return fmt.Errorf("permiso denegado: no tiene permisos de lectura en %s", cmd.path)
		}
	} else {
		fmt.Println("DEBUG: Usuario es root, permisos de lectura no verificados")
	}

	// Verificar que el destino sea una carpeta existente
	fmt.Println("DEBUG: Verificando que el destino sea una carpeta existente:", cmd.destino)
	destInode, destBlock, _, err := findParentDir(sb, partition, path, cmd.destino)
	if err != nil {
		fmt.Println("DEBUG: Error - Carpeta destino no encontrada:", err)
		return fmt.Errorf("error: la carpeta destino %s no existe", cmd.destino)
	}
	fmt.Println("DEBUG: Directorio destino encontrado - Entrada 0:", string(destBlock.B_content[0].B_name[:]))

	// Verificar que el destino sea una carpeta (I_type == '0')
	if destInode.I_type[0] != '0' {
		fmt.Println("DEBUG: Error - El destino no es una carpeta")
		return fmt.Errorf("error: el destino %s debe ser una carpeta", cmd.destino)
	}

	// Verificar permisos de escritura en la carpeta destino
	fmt.Println("DEBUG: Verificando permisos de escritura en el destino...")
	if stores.ActiveSession.Username != "root" {
		hasWritePermission := checkWritePermission(destInode)
		fmt.Println("DEBUG: Permiso de escritura:", hasWritePermission)
		if !hasWritePermission {
			fmt.Println("DEBUG: Error - Permiso de escritura denegado en el destino")
			return fmt.Errorf("permiso denegado: no tiene permisos de escritura en %s", cmd.destino)
		}
	} else {
		fmt.Println("DEBUG: Usuario es root, permisos de escritura no verificados")
	}

	// Verificar si ya existe un elemento con el mismo nombre en el destino
	fmt.Println("DEBUG: Verificando si el nombre ya existe en el destino:", srcName)
	for i, content := range destBlock.B_content {
		name := strings.Trim(string(content.B_name[:]), "\x00")
		fmt.Printf("DEBUG: Entrada %d - Nombre: %q, Inodo: %d\n", i, name, content.B_inodo)
		if name == srcName {
			fmt.Println("DEBUG: Error - Nombre ya existe en el destino")
			return fmt.Errorf("ya existe un elemento con el nombre %s en %s", srcName, cmd.destino)
		}
	}

	// Copiar el elemento
	fmt.Println("DEBUG: Copiando elemento - Inodo origen:", srcInodeIndex)
	newInodeIndex, err := copyElement(sb, path, srcInode, srcInodeIndex, srcName)
	if err != nil {
		fmt.Println("DEBUG: Error al copiar elemento:", err)
		return fmt.Errorf("error al copiar elemento: %v", err)
	}
	fmt.Println("DEBUG: Nuevo inodo creado - Índice:", newInodeIndex)

	// Agregar la nueva entrada al directorio destino
	fmt.Println("DEBUG: Agregando nueva entrada al directorio destino...")
	newEntry := structures.FolderContent{
		B_inodo: newInodeIndex,
	}
	copy(newEntry.B_name[:], []byte(srcName))
	for i := range destBlock.B_content {
		if destBlock.B_content[i].B_inodo == -1 {
			destBlock.B_content[i] = newEntry
			fmt.Println("DEBUG: Entrada agregada - Índice:", i)
			break
		}
		if i == len(destBlock.B_content)-1 {
			fmt.Println("DEBUG: Error - No hay espacio en el bloque del directorio destino")
			return errors.New("no hay espacio en el bloque del directorio destino para agregar la nueva entrada")
		}
	}

	// Serializar el bloque del directorio destino
	fmt.Println("DEBUG: Serializando bloque del directorio destino - Posición:", destInode.I_block[0])
	err = destBlock.Serialize(path, int64(destInode.I_block[0])) // Usar posición absoluta
	if err != nil {
		fmt.Println("DEBUG: Error al actualizar bloque del destino:", err)
		return fmt.Errorf("error al actualizar bloque del destino: %v", err)
	}
	fmt.Println("DEBUG: Bloque del directorio destino actualizado exitosamente")

	// Actualizar el superbloque
	fmt.Println("DEBUG: Serializando superbloque - Posición:", partition.Part_start)
	err = sb.Serialize(path, int64(partition.Part_start))
	if err != nil {
		fmt.Println("DEBUG: Error al actualizar superbloque:", err)
		return fmt.Errorf("error al actualizar superbloque: %v", err)
	}
	fmt.Println("DEBUG: Superbloque actualizado exitosamente")

	return nil
}

// copyElement copia un elemento (archivo o carpeta) y su contenido
func copyElement(sb *structures.SuperBlock, path string, srcInode *structures.Inode, srcInodeIndex int32, elementName string) (int32, error) {
	fmt.Println("\nDEBUG: Iniciando copyElement - Inodo origen:", srcInodeIndex, "Nombre:", elementName)

	// Asignar un nuevo inodo para la copia
	fmt.Println("DEBUG: Asignando nuevo inodo...")
	newInodeIndex, err := allocateInode(sb, path)
	if err != nil {
		fmt.Println("DEBUG: Error al asignar inodo:", err)
		return -1, fmt.Errorf("error al asignar inodo: %v", err)
	}
	fmt.Println("DEBUG: Nuevo inodo asignado - Índice:", newInodeIndex)

	// Crear una copia del inodo
	fmt.Println("DEBUG: Creando copia del inodo origen...")
	newInode := *srcInode
	for i := range newInode.I_block {
		newInode.I_block[i] = -1 // Limpiar los apuntadores a bloques
		fmt.Println("DEBUG: I_block[", i, "] limpiado a -1")
	}

	// Copiar los bloques según el tipo de elemento
	if srcInode.I_type[0] == '1' { // Archivo
		fmt.Println("DEBUG: El elemento es un archivo, copiando bloques...")
		// Leer y copiar los bloques de contenido
		for i, blockIndex := range srcInode.I_block {
			if blockIndex == -1 {
				fmt.Println("DEBUG: Bloque", i, "no existe, saltando")
				continue
			}
			fmt.Println("DEBUG: Procesando bloque - Índice:", blockIndex)
			srcBlock := &structures.FileBlock{}
			err := srcBlock.Deserialize(path, int64(blockIndex)) // Usar posición absoluta
			if err != nil {
				fmt.Println("DEBUG: Error al leer bloque de archivo:", err)
				return -1, fmt.Errorf("error al leer bloque de archivo: %v", err)
			}
			fmt.Println("DEBUG: Bloque de archivo leído - Contenido parcial:", string(srcBlock.B_content[:10]))

			// Asignar un nuevo bloque
			fmt.Println("DEBUG: Asignando nuevo bloque para copia...")
			newBlockIndex, err := allocateBlock(sb, path)
			if err != nil {
				fmt.Println("DEBUG: Error al asignar bloque:", err)
				return -1, fmt.Errorf("error al asignar bloque: %v", err)
			}
			newInode.I_block[i] = newBlockIndex
			fmt.Println("DEBUG: Nuevo bloque asignado - Índice:", newBlockIndex)

			// Copiar el contenido del bloque
			fmt.Println("DEBUG: Copiando contenido al nuevo bloque...")
			newBlock := *srcBlock
			err = newBlock.Serialize(path, int64(newBlockIndex)) // Usar posición absoluta
			if err != nil {
				fmt.Println("DEBUG: Error al escribir bloque de archivo:", err)
				return -1, fmt.Errorf("error al escribir bloque de archivo: %v", err)
			}
			fmt.Println("DEBUG: Bloque de archivo copiado exitosamente")
		}
	} else if srcInode.I_type[0] == '0' { // Carpeta
		fmt.Println("DEBUG: El elemento es una carpeta, copiando bloques y contenido recursivamente...")
		// Copiar los bloques de carpeta y su contenido recursivamente
		for i, blockIndex := range srcInode.I_block {
			if blockIndex == -1 {
				fmt.Println("DEBUG: Bloque", i, "no existe, saltando")
				continue
			}
			fmt.Println("DEBUG: Procesando bloque - Índice:", blockIndex)
			srcBlock := &structures.FolderBlock{}
			err := srcBlock.Deserialize(path, int64(blockIndex)) // Usar posición absoluta
			if err != nil {
				fmt.Println("DEBUG: Error al leer bloque de carpeta:", err)
				return -1, fmt.Errorf("error al leer bloque de carpeta: %v", err)
			}
			fmt.Println("DEBUG: Bloque de carpeta leído - Entrada 0:", string(srcBlock.B_content[0].B_name[:]))

			// Asignar un nuevo bloque
			fmt.Println("DEBUG: Asignando nuevo bloque para copia...")
			newBlockIndex, err := allocateBlock(sb, path)
			if err != nil {
				fmt.Println("DEBUG: Error al asignar bloque:", err)
				return -1, fmt.Errorf("error al asignar bloque: %v", err)
			}
			newInode.I_block[i] = newBlockIndex
			fmt.Println("DEBUG: Nuevo bloque asignado - Índice:", newBlockIndex)

			// Copiar el bloque, ajustando los inodos de los contenidos
			fmt.Println("DEBUG: Copiando contenido del bloque de carpeta...")
			newBlock := &structures.FolderBlock{}
			for j, content := range srcBlock.B_content {
				name := strings.Trim(string(content.B_name[:]), "\x00")
				fmt.Printf("DEBUG: Entrada %d - Nombre: %q, Inodo: %d\n", j, name, content.B_inodo)
				if name == "." || name == ".." {
					newBlock.B_content[j] = content
					fmt.Println("DEBUG: Entrada . o .. copiada sin cambios")
					continue
				}
				if content.B_inodo == -1 {
					newBlock.B_content[j] = content
					fmt.Println("DEBUG: Entrada con inodo -1 copiada sin cambios")
					continue
				}

				// Leer el inodo del elemento hijo
				fmt.Println("DEBUG: Leyendo inodo hijo - Inodo:", content.B_inodo)
				childInode := &structures.Inode{}
				err := childInode.Deserialize(path, int64(sb.S_inode_start)+int64(content.B_inodo)*int64(sb.S_inode_size))
				if err != nil {
					fmt.Println("DEBUG: Error al leer inodo hijo:", err)
					return -1, fmt.Errorf("error al leer inodo hijo: %v", err)
				}
				fmt.Println("DEBUG: Inodo hijo deserializado - I_type:", string(childInode.I_type[:]))

				// Verificar permisos de lectura para el elemento hijo
				if stores.ActiveSession.Username != "root" {
					hasReadPermission := checkReadPermission(childInode)
					fmt.Println("DEBUG: Permiso de lectura del hijo:", hasReadPermission)
					if !hasReadPermission {
						fmt.Println("DEBUG: Advertencia - No se copió", name, "por falta de permisos")
						fmt.Printf("No se copió %s debido a falta de permisos de lectura\n", name)
						continue
					}
				}

				// Copiar el elemento hijo recursivamente
				fmt.Println("DEBUG: Copiando elemento hijo:", name)
				newChildInodeIndex, err := copyElement(sb, path, childInode, content.B_inodo, name)
				if err != nil {
					fmt.Println("DEBUG: Error al copiar elemento hijo:", err)
					return -1, fmt.Errorf("error al copiar elemento hijo %s: %v", name, err)
				}
				fmt.Println("DEBUG: Nuevo inodo hijo creado - Índice:", newChildInodeIndex)

				// Actualizar la entrada con el nuevo inodo
				newBlock.B_content[j] = structures.FolderContent{
					B_name:  content.B_name,
					B_inodo: newChildInodeIndex,
				}
				fmt.Println("DEBUG: Entrada actualizada con nuevo inodo:", newChildInodeIndex)
			}

			// Serializar el nuevo bloque
			fmt.Println("DEBUG: Serializando nuevo bloque - Posición:", newBlockIndex)
			err = newBlock.Serialize(path, int64(newBlockIndex)) // Usar posición absoluta
			if err != nil {
				fmt.Println("DEBUG: Error al escribir bloque de carpeta:", err)
				return -1, fmt.Errorf("error al escribir bloque de carpeta: %v", err)
			}
			fmt.Println("DEBUG: Bloque de carpeta copiado exitosamente")
		}
	}

	// Serializar el nuevo inodo
	fmt.Println("DEBUG: Serializando nuevo inodo - Posición:", int64(sb.S_inode_start)+int64(newInodeIndex)*int64(sb.S_inode_size))
	err = newInode.Serialize(path, int64(sb.S_inode_start)+int64(newInodeIndex)*int64(sb.S_inode_size))
	if err != nil {
		fmt.Println("DEBUG: Error al escribir inodo:", err)
		return -1, fmt.Errorf("error al escribir inodo: %v", err)
	}
	fmt.Println("DEBUG: Nuevo inodo serializado exitosamente")

	return newInodeIndex, nil
}

// allocateInode asigna un nuevo inodo
func allocateInode(sb *structures.SuperBlock, path string) (int32, error) {
	fmt.Println("\nDEBUG: Iniciando allocateInode")

	if sb.S_free_inodes_count == 0 {
		fmt.Println("DEBUG: Error - No hay inodos libres disponibles")
		return -1, errors.New("no hay inodos libres disponibles")
	}

	// Buscar un inodo libre en el bitmap
	fmt.Println("DEBUG: Abriendo archivo para leer bitmap - Path:", path)
	file, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		fmt.Println("DEBUG: Error al abrir archivo:", err)
		return -1, err
	}
	defer file.Close()
	fmt.Println("DEBUG: Archivo abierto exitosamente")

	bitmapSize := int(sb.S_inodes_count / 8)
	if sb.S_inodes_count%8 != 0 {
		bitmapSize++
	}
	fmt.Println("DEBUG: Tamaño del bitmap:", bitmapSize)

	bitmapData := make([]byte, bitmapSize)
	fmt.Println("DEBUG: Posicionando puntero en S_bm_inode_start:", sb.S_bm_inode_start)
	_, err = file.Seek(int64(sb.S_bm_inode_start), 0)
	if err != nil {
		fmt.Println("DEBUG: Error al posicionar puntero:", err)
		return -1, err
	}
	fmt.Println("DEBUG: Leyendo bitmap...")
	_, err = file.Read(bitmapData)
	if err != nil {
		fmt.Println("DEBUG: Error al leer bitmap:", err)
		return -1, err
	}
	fmt.Println("DEBUG: Bitmap leído - Longitud:", len(bitmapData))

	// Buscar un bit libre (0)
	var inodeIndex int32 = -1
	for byteIdx := 0; byteIdx < bitmapSize; byteIdx++ {
		for bitIdx := 0; bitIdx < 8; bitIdx++ {
			if int32(byteIdx*8+bitIdx) >= sb.S_inodes_count {
				fmt.Println("DEBUG: Límite de inodos alcanzado")
				break
			}
			if (bitmapData[byteIdx] & (1 << bitIdx)) == 0 {
				inodeIndex = int32(byteIdx*8 + bitIdx)
				fmt.Println("DEBUG: Inodo libre encontrado - Índice:", inodeIndex)
				bitmapData[byteIdx] |= (1 << bitIdx) // Marcar como ocupado
				fmt.Println("DEBUG: Bit marcado como ocupado en posición:", byteIdx*8+bitIdx)
				break
			}
		}
		if inodeIndex != -1 {
			break
		}
	}

	if inodeIndex == -1 {
		fmt.Println("DEBUG: Error - No se encontró un inodo libre")
		return -1, errors.New("no se encontró un inodo libre")
	}

	// Escribir el bitmap actualizado
	fmt.Println("DEBUG: Posicionando puntero para escribir bitmap - S_bm_inode_start:", sb.S_bm_inode_start)
	_, err = file.Seek(int64(sb.S_bm_inode_start), 0)
	if err != nil {
		fmt.Println("DEBUG: Error al posicionar puntero:", err)
		return -1, err
	}
	fmt.Println("DEBUG: Escribiendo bitmap actualizado...")
	_, err = file.Write(bitmapData)
	if err != nil {
		fmt.Println("DEBUG: Error al escribir bitmap:", err)
		return -1, err
	}
	fmt.Println("DEBUG: Bitmap escrito exitosamente")

	// Decrementar el conteo de inodos libres
	fmt.Println("DEBUG: Decrementando S_free_inodes_count:", sb.S_free_inodes_count)
	sb.S_free_inodes_count--
	fmt.Println("DEBUG: Nuevo S_free_inodes_count:", sb.S_free_inodes_count)

	return inodeIndex, nil
}

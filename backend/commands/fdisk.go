package commands

import (
	structures "backend/structures"
	utils "backend/utils"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// FDISK estructura que representa el comando fdisk con sus parámetros
type FDISK struct {
	size   int    // Tamaño de la partición
	unit   string // Unidad de medida del tamaño (K o M)
	fit    string // Tipo de ajuste (BF, FF, WF)
	path   string // Ruta del archivo del disco
	typ    string // Tipo de partición (P, E, L)
	name   string // Nombre de la partición
	add    int    // Espacio a agregar/quitar (positivo o negativo)
	delete string // Tipo de eliminación (fast, full)
}

func ParseFdisk(tokens []string) (string, error) {
	cmd := &FDISK{}
	args := strings.Join(tokens, " ")
	// Expresión regular para capturar los parámetros válidos, insensible a mayúsculas/minúsculas
	re := regexp.MustCompile(`(?i)-size=\d+|(?i)-unit=[bBkKmM]|(?i)-fit=[bBfFwW]{2}|(?i)-path="[^"]+"|(?i)-path=[^\s]+|(?i)-type=[pPeElL]|(?i)-name="[^"]+"|(?i)-name=[^\s]+|(?i)-add=-?\d+|(?i)-delete=(fast|full)`)
	matches := re.FindAllString(args, -1)

	// Validar que no haya parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	allowedParams := map[string]bool{
		"-size":   true,
		"-unit":   true,
		"-fit":    true,
		"-path":   true,
		"-type":   true,
		"-name":   true,
		"-add":    true,
		"-delete": true,
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
		key, value := strings.ToLower(kv[0]), kv[1]

		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			value = strings.Trim(value, "\"")
		}

		switch key {
		case "-size":
			size, err := strconv.Atoi(value)
			if err != nil || size <= 0 {
				return "", errors.New("el tamaño debe ser un número entero positivo")
			}
			cmd.size = size
		case "-unit":
			value = strings.ToUpper(value)
			if value != "B" && value != "K" && value != "M" {
				return "", errors.New("la unidad debe ser B, K o M")
			}
			cmd.unit = value
		case "-fit":
			value = strings.ToUpper(value)
			if value != "BF" && value != "FF" && value != "WF" {
				return "", errors.New("el ajuste debe ser BF, FF o WF")
			}
			cmd.fit = value
		case "-path":
			if value == "" {
				return "", errors.New("el path no puede estar vacío")
			}
			cmd.path = value
		case "-type":
			value = strings.ToUpper(value)
			if value != "P" && value != "E" && value != "L" {
				return "", errors.New("el tipo debe ser P, E o L")
			}
			cmd.typ = value
		case "-name":
			if value == "" {
				return "", errors.New("el nombre no puede estar vacío")
			}
			cmd.name = value
		case "-add":
			addValue, err := strconv.Atoi(value)
			if err != nil {
				return "", errors.New("el valor de -add debe ser un número entero")
			}
			cmd.add = addValue
		case "-delete":
			value = strings.ToLower(value)
			if value != "fast" && value != "full" {
				return "", errors.New("el valor de -delete debe ser 'fast' o 'full'")
			}
			cmd.delete = value
		}
	}

	// Validaciones y valores por defecto
	if cmd.path == "" || cmd.name == "" {
		return "", errors.New("faltan parámetros requeridos: -path, -name")
	}
	if cmd.unit == "" {
		cmd.unit = "K"
	}
	if cmd.fit == "" {
		cmd.fit = "WF"
	}
	if cmd.typ == "" {
		cmd.typ = "P"
	}

	fmt.Println("DEBUG: Parseando FDISK")
	fmt.Printf("  size=%d, unit=%s, fit=%s, path=%s, type=%s, name=%s, add=%d, delete=%s\n", cmd.size, cmd.unit, cmd.fit, cmd.path, cmd.typ, cmd.name, cmd.add, cmd.delete)

	// Prioridad: -delete > -add > creación de partición
	if cmd.delete != "" {
		// Operación de eliminación
		err := deletePartition(cmd)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("FDISK: Partición eliminada exitosamente\n-> Path: %s\n-> Nombre: %s\n-> Modo: %s", cmd.path, cmd.name, cmd.delete), nil
	} else if cmd.add != 0 {
		// Operación de ajuste de tamaño
		err := adjustPartitionSize(cmd)
		if err != nil {
			return "", err
		}
		action := "agregados"
		if cmd.add < 0 {
			action = "quitados"
		}
		return fmt.Sprintf("FDISK: Tamaño de partición ajustado exitosamente\n-> Path: %s\n-> Nombre: %s\n-> Espacio %s: %d%s", cmd.path, cmd.name, action, cmd.add, cmd.unit), nil
	} else {
		// Operación de creación (requiere -size)
		if cmd.size == 0 {
			return "", errors.New("falta el parámetro requerido: -size")
		}
		err := commandFdisk(cmd)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("FDISK: Partición creada exitosamente\n-> Path: %s\n-> Nombre: %s\n-> Tamaño: %d%s\n-> Tipo: %s\n-> Fit: %s",
			cmd.path, cmd.name, cmd.size, cmd.unit, cmd.typ, cmd.fit), nil
	}
}

func commandFdisk(fdisk *FDISK) error {
	if _, err := os.Stat(fdisk.path); os.IsNotExist(err) {
		return errors.New("el disco especificado en -path no existe")
	} else if err != nil {
		return fmt.Errorf("error al verificar el disco: %v", err)
	}

	sizeBytes, err := utils.ConvertToBytes(fdisk.size, fdisk.unit)
	if err != nil {
		fmt.Println("DEBUG: Error convirtiendo tamaño:", err)
		return err
	}
	fmt.Println("DEBUG: Tamaño en bytes:", sizeBytes)

	var mbr structures.MBR
	err = mbr.Deserialize(fdisk.path)
	if err != nil {
		fmt.Println("DEBUG: Error deserializando MBR:", err)
		return err
	}
	fmt.Println("DEBUG: MBR antes de crear partición:")
	mbr.PrintMBR()
	mbr.PrintPartitions()

	if fdisk.typ == "P" {
		err = createPrimaryPartition(fdisk, sizeBytes)
	} else if fdisk.typ == "E" {
		err = createExtendedPartition(fdisk, sizeBytes)
	} else if fdisk.typ == "L" {
		err = createLogicalPartition(fdisk, sizeBytes)
	}
	if err != nil {
		return err
	}

	err = mbr.Deserialize(fdisk.path)
	if err != nil {
		fmt.Println("DEBUG: Error deserializando MBR después de crear partición:", err)
		return err
	}
	fmt.Println("DEBUG: MBR después de crear partición:")
	mbr.PrintMBR()
	mbr.PrintPartitions()

	return nil
}

func createPrimaryPartition(fdisk *FDISK, sizeBytes int) error {
	var mbr structures.MBR
	err := mbr.Deserialize(fdisk.path)
	if err != nil {
		return err
	}

	usedSlots := 0
	for _, p := range mbr.Mbr_partitions {
		if p.Part_start != -1 && (p.Part_type[0] == 'P' || p.Part_type[0] == 'E') {
			usedSlots++
		}
	}
	if usedSlots >= 4 {
		fmt.Println("DEBUG: Límite de 4 particiones alcanzado")
		return errors.New("se alcanzó el límite máximo de 4 particiones primarias y extendidas en el MBR")
	}

	availablePartition, startPartition, indexPartition := mbr.GetFirstAvailablePartition()
	if availablePartition == nil {
		fmt.Println("DEBUG: No hay particiones disponibles")
		return errors.New("no hay particiones disponibles en el MBR")
	}
	fmt.Println("DEBUG: Partición disponible encontrada - Start:", startPartition, "Index:", indexPartition)

	if int32(sizeBytes) > mbr.Mbr_size-int32(startPartition) {
		fmt.Println("DEBUG: Tamaño excede el espacio disponible")
		return errors.New("el tamaño de la partición excede el espacio disponible en el disco")
	}

	availablePartition.CreatePartition(startPartition, sizeBytes, fdisk.typ, fdisk.fit, fdisk.name)
	mbr.Mbr_partitions[indexPartition] = *availablePartition
	fmt.Println("DEBUG: Nueva partición primaria creada:")
	availablePartition.PrintPartition()

	err = mbr.Serialize(fdisk.path)
	if err != nil {
		return err
	}
	return nil
}

func createExtendedPartition(fdisk *FDISK, sizeBytes int) error {
	var mbr structures.MBR
	err := mbr.Deserialize(fdisk.path)
	if err != nil {
		return err
	}

	usedSlots := 0
	extendedCount := 0
	for _, p := range mbr.Mbr_partitions {
		if p.Part_start != -1 && (p.Part_type[0] == 'P' || p.Part_type[0] == 'E') {
			usedSlots++
			if p.Part_type[0] == 'E' {
				extendedCount++
			}
		}
	}
	if usedSlots >= 4 {
		fmt.Println("DEBUG: Límite de 4 particiones alcanzado")
		return errors.New("se alcanzó el límite máximo de 4 particiones primarias y extendidas en el MBR")
	}
	if extendedCount > 0 {
		fmt.Println("DEBUG: Ya existe una partición extendida")
		return errors.New("ya existe una partición extendida en el disco")
	}

	availablePartition, startPartition, indexPartition := mbr.GetFirstAvailablePartition()
	if availablePartition == nil {
		fmt.Println("DEBUG: No hay particiones disponibles")
		return errors.New("no hay particiones disponibles en el MBR")
	}
	fmt.Println("DEBUG: Partición disponible encontrada - Start:", startPartition, "Index:", indexPartition)

	if int32(sizeBytes) > mbr.Mbr_size-int32(startPartition) {
		fmt.Println("DEBUG: Tamaño excede el espacio disponible")
		return errors.New("el tamaño de la partición excede el espacio disponible en el disco")
	}

	availablePartition.CreatePartition(startPartition, sizeBytes, fdisk.typ, fdisk.fit, fdisk.name)
	mbr.Mbr_partitions[indexPartition] = *availablePartition
	fmt.Println("DEBUG: Nueva partición extendida creada:")
	availablePartition.PrintPartition()

	err = mbr.Serialize(fdisk.path)
	if err != nil {
		return err
	}
	return nil
}

func createLogicalPartition(fdisk *FDISK, sizeBytes int) error {
	fmt.Println("\nINICIO createLogicalPartition:")
	var mbr structures.MBR
	err := mbr.Deserialize(fdisk.path)
	if err != nil {
		return err
	}

	var extendedPartition *structures.Partition
	var extendedStart, extendedSize int32
	for i, p := range mbr.Mbr_partitions {
		if p.Part_type[0] == 'E' && p.Part_status[0] != 'N' {
			extendedPartition = &mbr.Mbr_partitions[i]
			extendedStart = p.Part_start
			extendedSize = p.Part_size
			break
		}
	}
	if extendedPartition == nil {
		fmt.Println("DEBUG: No se encontró partición extendida")
		return errors.New("no existe una partición extendida para crear una partición lógica")
	}
	fmt.Println("DEBUG: Partición extendida encontrada - Start:", extendedStart, "Size:", extendedSize)

	currentOffset := int64(extendedStart)
	var currentEBR structures.EBR
	err = currentEBR.Deserialize(fdisk.path, currentOffset)
	if err != nil || currentEBR.Part_start == 0 {
		currentOffset = int64(extendedStart)
		fmt.Println("DEBUG: No hay EBR previo, iniciando en:", currentOffset)
	} else {
		fmt.Println("DEBUG: Recorriendo EBRs existentes")
		for currentEBR.Part_next != -1 {
			currentOffset = int64(currentEBR.Part_next)
			err = currentEBR.Deserialize(fdisk.path, currentOffset)
			if err != nil {
				return fmt.Errorf("error leyendo EBR en offset %d: %v", currentOffset, err)
			}
			fmt.Println("DEBUG: EBR actual:")
			currentEBR.PrintEBR()
		}
		currentOffset = int64(currentEBR.Part_start + currentEBR.Part_s)
		fmt.Println("DEBUG: Último EBR encontrado, nuevo offset:", currentOffset)
	}

	if int32(currentOffset)+int32(sizeBytes) > extendedStart+extendedSize {
		fmt.Println("DEBUG: Tamaño excede el espacio en la extendida")
		return errors.New("el tamaño de la partición lógica excede el espacio disponible en la partición extendida")
	}

	newEBR := structures.EBR{
		Part_mount: [1]byte{'0'},
		Part_fit:   [1]byte{fdisk.fit[0]},
		Part_start: int32(currentOffset),
		Part_s:     int32(sizeBytes),
		Part_next:  -1,
		Part_name:  [16]byte{},
	}
	copy(newEBR.Part_name[:], fdisk.name)
	fmt.Println("DEBUG: Nuevo EBR creado:")
	newEBR.PrintEBR()

	if currentEBR.Part_start != 0 {
		currentEBR.Part_next = int32(currentOffset)
		err = currentEBR.Serialize(fdisk.path, int64(currentEBR.Part_start))
		if err != nil {
			return err
		}
		fmt.Println("DEBUG: EBR previo actualizado con Part_next:", currentEBR.Part_next)
	}

	err = newEBR.Serialize(fdisk.path, currentOffset)
	if err != nil {
		return err
	}
	fmt.Println("DEBUG: Nuevo EBR serializado en offset:", currentOffset)

	return nil
}

func deletePartition(fdisk *FDISK) error {
	// Verificar si el disco existe
	if _, err := os.Stat(fdisk.path); os.IsNotExist(err) {
		return errors.New("el disco especificado en -path no existe")
	} else if err != nil {
		return fmt.Errorf("error al verificar el disco: %v", err)
	}

	// Cargar el MBR
	var mbr structures.MBR
	err := mbr.Deserialize(fdisk.path)
	if err != nil {
		return fmt.Errorf("error deserializando MBR: %v", err)
	}

	// Buscar la partición por nombre (primaria o extendida)
	partition, index := mbr.GetPartitionByName(fdisk.name)
	if partition != nil {
		// Partición primaria o extendida encontrada
		fmt.Printf("¿Está seguro de eliminar la partición '%s' de tipo '%c'? (s/n): ", fdisk.name, partition.Part_type[0])
		var confirm string
		fmt.Scanln(&confirm)
		if strings.ToLower(confirm) != "s" {
			return errors.New("operación de eliminación cancelada")
		}

		// Si es extendida, eliminar las particiones lógicas
		if partition.Part_type[0] == 'E' {
			err := deleteLogicalPartitions(fdisk.path, int64(partition.Part_start))
			if err != nil {
				return fmt.Errorf("error eliminando particiones lógicas: %v", err)
			}
		}

		// Eliminar la partición
		if fdisk.delete == "full" {
			// Rellenar el espacio con \0
			file, err := os.OpenFile(fdisk.path, os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Errorf("error abriendo el disco: %v", err)
			}
			defer file.Close()

			_, err = file.Seek(int64(partition.Part_start), 0)
			if err != nil {
				return fmt.Errorf("error posicionando el puntero: %v", err)
			}

			zeros := make([]byte, partition.Part_size)
			_, err = file.Write(zeros)
			if err != nil {
				return fmt.Errorf("error rellenando con ceros: %v", err)
			}
		}

		// Marcar como vacía
		partition.Part_status = [1]byte{'N'}
		partition.Part_type = [1]byte{0}
		partition.Part_fit = [1]byte{0}
		partition.Part_start = -1
		partition.Part_size = 0
		partition.Part_name = [16]byte{}
		partition.Part_correlative = -1
		partition.Part_id = [4]byte{}
		mbr.Mbr_partitions[index] = *partition

		// Guardar el MBR
		err = mbr.Serialize(fdisk.path)
		if err != nil {
			return fmt.Errorf("error serializando MBR: %v", err)
		}
		return nil
	}

	// Si no es primaria/extendida, buscar en las particiones lógicas
	extendedPartition := false
	var extendedStart int64
	for _, p := range mbr.Mbr_partitions {
		if p.Part_type[0] == 'E' && p.Part_status[0] != 'N' {
			extendedPartition = true
			extendedStart = int64(p.Part_start)
			break
		}
	}
	if !extendedPartition {
		return fmt.Errorf("partición '%s' no encontrada", fdisk.name)
	}

	// Buscar la partición lógica
	currentOffset := extendedStart
	var prevEBR, currentEBR structures.EBR
	found := false
	for {
		err := currentEBR.Deserialize(fdisk.path, currentOffset)
		if err != nil {
			return fmt.Errorf("error leyendo EBR: %v", err)
		}
		if currentEBR.Part_s == 0 {
			break
		}

		ebrName := strings.TrimRight(string(currentEBR.Part_name[:]), "\x00")
		if ebrName == fdisk.name {
			found = true
			break
		}

		if currentEBR.Part_next == -1 {
			break
		}
		prevEBR = currentEBR
		currentOffset = int64(currentEBR.Part_next)
	}

	if !found {
		return fmt.Errorf("partición lógica '%s' no encontrada", fdisk.name)
	}

	// Confirmar eliminación
	fmt.Printf("¿Está seguro de eliminar la partición lógica '%s'? (s/n): ", fdisk.name)
	var confirm string
	fmt.Scanln(&confirm)
	if strings.ToLower(confirm) != "s" {
		return errors.New("operación de eliminación cancelada")
	}

	// Eliminar la partición lógica
	if fdisk.delete == "full" {
		file, err := os.OpenFile(fdisk.path, os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("error abriendo el disco: %v", err)
		}
		defer file.Close()

		_, err = file.Seek(currentOffset, 0)
		if err != nil {
			return fmt.Errorf("error posicionando el puntero: %v", err)
		}

		zeros := make([]byte, currentEBR.Part_s)
		_, err = file.Write(zeros)
		if err != nil {
			return fmt.Errorf("error rellenando con ceros: %v", err)
		}
	}

	// Actualizar la lista enlazada de EBRs
	if currentOffset == extendedStart {
		// Es el primer EBR
		if currentEBR.Part_next == -1 {
			// Es el único EBR, marcar como vacío
			currentEBR = structures.EBR{}
		} else {
			// Mover el siguiente EBR al inicio
			nextEBR := structures.EBR{}
			err := nextEBR.Deserialize(fdisk.path, int64(currentEBR.Part_next))
			if err != nil {
				return fmt.Errorf("error leyendo siguiente EBR: %v", err)
			}
			currentEBR = nextEBR
		}
	} else {
		// No es el primer EBR
		prevEBR.Part_next = currentEBR.Part_next
		err := prevEBR.Serialize(fdisk.path, int64(prevEBR.Part_start))
		if err != nil {
			return fmt.Errorf("error actualizando EBR previo: %v", err)
		}
	}

	// Guardar los cambios
	err = currentEBR.Serialize(fdisk.path, currentOffset)
	if err != nil {
		return fmt.Errorf("error serializando EBR: %v", err)
	}

	return nil
}

func deleteLogicalPartitions(path string, extendedStart int64) error {
	currentOffset := extendedStart
	for {
		var ebr structures.EBR
		err := ebr.Deserialize(path, currentOffset)
		if err != nil {
			return fmt.Errorf("error leyendo EBR: %v", err)
		}
		if ebr.Part_s == 0 {
			break
		}

		// Rellenar con ceros (modo full implícito para particiones lógicas)
		file, err := os.OpenFile(path, os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("error abriendo el disco: %v", err)
		}
		defer file.Close()

		_, err = file.Seek(currentOffset, 0)
		if err != nil {
			return fmt.Errorf("error posicionando el puntero: %v", err)
		}

		zeros := make([]byte, ebr.Part_s)
		_, err = file.Write(zeros)
		if err != nil {
			return fmt.Errorf("error rellenando con ceros: %v", err)
		}

		if ebr.Part_next == -1 {
			break
		}
		currentOffset = int64(ebr.Part_next)
	}

	// Marcar el primer EBR como vacío
	emptyEBR := structures.EBR{}
	err := emptyEBR.Serialize(path, extendedStart)
	if err != nil {
		return fmt.Errorf("error marcando EBR como vacío: %v", err)
	}

	return nil
}

func adjustPartitionSize(fdisk *FDISK) error {
	// Verificar si el disco existe
	if _, err := os.Stat(fdisk.path); os.IsNotExist(err) {
		return errors.New("el disco especificado en -path no existe")
	} else if err != nil {
		return fmt.Errorf("error al verificar el disco: %v", err)
	}

	// Convertir el valor de -add a bytes
	addBytes, err := utils.ConvertToBytes(fdisk.add, fdisk.unit)
	if err != nil {
		return fmt.Errorf("error convirtiendo el valor de -add a bytes: %v", err)
	}

	// Cargar el MBR
	var mbr structures.MBR
	err = mbr.Deserialize(fdisk.path)
	if err != nil {
		return fmt.Errorf("error deserializando MBR: %v", err)
	}

	// Buscar la partición por nombre (primaria o extendida)
	partition, index := mbr.GetPartitionByName(fdisk.name)
	if partition != nil {
		// Partición primaria o extendida encontrada
		newSize := int(partition.Part_size) + addBytes
		if newSize <= 0 {
			return errors.New("el nuevo tamaño de la partición no puede ser menor o igual a 0")
		}

		// Verificar espacio libre si se agrega espacio
		if addBytes > 0 {
			endOfPartition := int(partition.Part_start) + int(partition.Part_size)
			nextStart := int(mbr.Mbr_size)
			for i, p := range mbr.Mbr_partitions {
				if i == index || p.Part_start == -1 {
					continue
				}
				if int(p.Part_start) >= endOfPartition && int(p.Part_start) < nextStart {
					nextStart = int(p.Part_start)
				}
			}
			if endOfPartition+addBytes > nextStart {
				return errors.New("no hay suficiente espacio libre después de la partición para agregar el tamaño especificado")
			}

			// Si es extendida, verificar que las particiones lógicas quepan
			if partition.Part_type[0] == 'E' {
				currentOffset := int64(partition.Part_start)
				for {
					var ebr structures.EBR
					err := ebr.Deserialize(fdisk.path, currentOffset)
					if err != nil {
						return fmt.Errorf("error leyendo EBR: %v", err)
					}
					if ebr.Part_s == 0 {
						break
					}
					if int(currentOffset)+int(ebr.Part_s) > int(partition.Part_start)+newSize {
						return errors.New("el nuevo tamaño de la partición extendida no puede ser menor que el espacio ocupado por las particiones lógicas")
					}
					if ebr.Part_next == -1 {
						break
					}
					currentOffset = int64(ebr.Part_next)
				}
			}
		}

		// Actualizar el tamaño
		partition.Part_size = int32(newSize)
		mbr.Mbr_partitions[index] = *partition

		// Guardar el MBR
		err = mbr.Serialize(fdisk.path)
		if err != nil {
			return fmt.Errorf("error serializando MBR: %v", err)
		}
		return nil
	}

	// Si no es primaria/extendida, buscar en las particiones lógicas
	extendedPartition := false
	var extendedStart, extendedSize int64
	for _, p := range mbr.Mbr_partitions {
		if p.Part_type[0] == 'E' && p.Part_status[0] != 'N' {
			extendedPartition = true
			extendedStart = int64(p.Part_start)
			extendedSize = int64(p.Part_size)
			break
		}
	}
	if !extendedPartition {
		return fmt.Errorf("partición '%s' no encontrada", fdisk.name)
	}

	// Buscar y ajustar la partición lógica en un solo bucle
	currentOffset := extendedStart
	var currentEBR structures.EBR
	for {
		err := currentEBR.Deserialize(fdisk.path, currentOffset)
		if err != nil {
			return fmt.Errorf("error leyendo EBR: %v", err)
		}
		if currentEBR.Part_s == 0 {
			return fmt.Errorf("partición lógica '%s' no encontrada", fdisk.name)
		}

		ebrName := strings.TrimRight(string(currentEBR.Part_name[:]), "\x00")
		if ebrName == fdisk.name {
			break // Encontramos la partición, salimos del bucle para ajustarla
		}

		if currentEBR.Part_next == -1 {
			return fmt.Errorf("partición lógica '%s' no encontrada", fdisk.name)
		}
		currentOffset = int64(currentEBR.Part_next)
	}

	// Calcular el nuevo tamaño
	newSize := int(currentEBR.Part_s) + addBytes
	if newSize <= 0 {
		return errors.New("el nuevo tamaño de la partición lógica no puede ser menor o igual a 0")
	}

	// Verificar espacio libre si se agrega espacio
	if addBytes > 0 {
		endOfPartition := int(currentEBR.Part_start) + int(currentEBR.Part_s)
		nextStart := int(extendedStart + extendedSize)
		if currentEBR.Part_next != -1 {
			nextStart = int(currentEBR.Part_next)
		}
		if endOfPartition+addBytes > nextStart {
			return errors.New("no hay suficiente espacio libre después de la partición lógica para agregar el tamaño especificado")
		}
	}

	// Actualizar el tamaño y la posición del siguiente EBR si es necesario
	currentEBR.Part_s = int32(newSize)
	if currentEBR.Part_next != -1 {
		nextOffset := int(currentEBR.Part_start) + newSize
		currentEBR.Part_next = int32(nextOffset)

		// Actualizar la posición de los EBRs siguientes
		nextOffsetInt64 := int64(nextOffset)
		for {
			var nextEBR structures.EBR
			err := nextEBR.Deserialize(fdisk.path, nextOffsetInt64)
			if err != nil {
				return fmt.Errorf("error leyendo siguiente EBR: %v", err)
			}
			if nextEBR.Part_s == 0 {
				break
			}

			nextEBR.Part_start = int32(nextOffsetInt64)
			err = nextEBR.Serialize(fdisk.path, nextOffsetInt64)
			if err != nil {
				return fmt.Errorf("error serializando siguiente EBR: %v", err)
			}

			if nextEBR.Part_next == -1 {
				break
			}
			nextOffsetInt64 = int64(nextEBR.Part_next)
			nextEBR.Part_next = int32(nextOffsetInt64)
		}
	}

	// Guardar los cambios
	err = currentEBR.Serialize(fdisk.path, int64(currentEBR.Part_start))
	if err != nil {
		return fmt.Errorf("error serializando EBR: %v", err)
	}

	return nil
}

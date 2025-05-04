package commands

import (
	stores "backend/stores"
	structures "backend/structures"
	utils "backend/utils"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// MOUNT estructura que representa el comando mount con sus parámetros
type MOUNT struct {
	path string // Ruta del archivo del disco
	name string // Nombre de la partición
}

// ParseMount parsea el comando mount y devuelve el resultado del montaje
func ParseMount(tokens []string) (string, error) {
	cmd := &MOUNT{} // Crea una nueva instancia de MOUNT

	// Unir tokens en una sola cadena y luego dividir por espacios, respetando las comillas
	args := strings.Join(tokens, " ")
	// Expresión regular para capturar los parámetros válidos, insensible a mayúsculas/minúsculas
	re := regexp.MustCompile(`(?i)-path="[^"]+"|(?i)-path=[^\s]+|(?i)-name="[^"]+"|(?i)-name=[^\s]+`)
	matches := re.FindAllString(args, -1)

	// Validar que no haya parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	allowedParams := map[string]bool{
		"-path": true,
		"-name": true,
	}

	for _, param := range allParams {
		// Extraer solo el nombre del parámetro (sin el valor) y convertir a minúsculas
		paramName := strings.ToLower(strings.SplitN(param, "=", 2)[0])
		if !allowedParams[paramName] {
			return "", fmt.Errorf("parámetro no permitido: %s", paramName)
		}
	}

	// Itera sobre cada coincidencia encontrada
	for _, match := range matches {
		kv := strings.SplitN(match, "=", 2)
		if len(kv) != 2 {
			return "", fmt.Errorf("formato de parámetro inválido: %s", match)
		}
		// Convertir el nombre del parámetro a minúsculas para la comparación
		key, value := strings.ToLower(kv[0]), kv[1]

		// Quitar comillas del valor si están presentes
		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			value = strings.Trim(value, "\"")
		}

		switch key {
		case "-path":
			if value == "" {
				return "", errors.New("el path no puede estar vacío")
			}
			cmd.path = value
		case "-name":
			if value == "" {
				return "", errors.New("el nombre no puede estar vacío")
			}
			cmd.name = value
		}
	}

	// Verifica que los parámetros -path y -name hayan sido proporcionados
	if cmd.path == "" {
		return "", errors.New("faltan parámetros requeridos: -path")
	}
	if cmd.name == "" {
		return "", errors.New("faltan parámetros requeridos: -name")
	}

	// Montamos la partición y obtenemos el ID generado
	idPartition, err := commandMount(cmd)
	if err != nil {
		return "", err
	}

	// Devuelve un mensaje de éxito con los detalles del montaje
	return fmt.Sprintf("MOUNT: Partición montada exitosamente\n"+
		"-> Path: %s\n"+
		"-> Nombre: %s\n"+
		"-> ID: %s",
		cmd.path, cmd.name, idPartition), nil
}

// commandMount monta la partición y devuelve el ID generado
func commandMount(mount *MOUNT) (string, error) {
	// Crear una instancia de MBR
	var mbr structures.MBR

	// Deserializar la estructura MBR desde un archivo binario
	err := mbr.Deserialize(mount.path)
	if err != nil {
		fmt.Println("Error deserializando el MBR:", err)
		return "", err
	}

	// Buscar la partición en las particiones primarias/extendidas
	partition, indexPartition := mbr.GetPartitionByName(mount.name)
	if partition != nil {
		fmt.Println("\nPartición disponible (primaria/extendida):")
		partition.PrintPartition()

		// Verificar si ya está montada (usamos Part_status en lugar de Part_mount)
		if partition.Part_status[0] == '1' {
			return "", fmt.Errorf("la partición %s ya está montada", mount.name)
		}

		// Generar un ID único para la partición
		idPartition, partitionCorrelative, err := generatePartitionID(mount)
		if err != nil {
			fmt.Println("Error generando el id de partición:", err)
			return "", err
		}

		// Guardar la partición montada en la lista de montajes globales
		stores.MountedPartitions[idPartition] = mount.path

		// Modificamos la partición para indicar que está montada
		partition.MountPartition(partitionCorrelative, idPartition)

		fmt.Println("\nPartición montada (modificada):")
		partition.PrintPartition()

		// Guardar la partición modificada en el MBR
		mbr.Mbr_partitions[indexPartition] = *partition

		// Serializar la estructura MBR en el archivo binario
		err = mbr.Serialize(mount.path)
		if err != nil {
			fmt.Println("Error serializando el MBR:", err)
			return "", err
		}

		return idPartition, nil
	}

	// Si no se encontró en las particiones primarias/extendidas, buscar en las particiones lógicas
	for _, part := range mbr.Mbr_partitions {
		if part.Part_type[0] != 'E' || part.Part_status[0] == 'N' || part.Part_size == 0 {
			continue
		}
		currentOffset := int64(part.Part_start)
		var currentEBR structures.EBR
		for currentOffset != -1 {
			err := currentEBR.Deserialize(mount.path, currentOffset)
			if err != nil {
				return "", fmt.Errorf("error deserializando EBR en offset %d: %v", currentOffset, err)
			}
			ebrName := strings.Trim(string(currentEBR.Part_name[:]), "\x00")
			fmt.Println("DEBUG: Revisando partición lógica - Nombre:", ebrName, "Start:", currentEBR.Part_start, "Size:", currentEBR.Part_s)
			if ebrName == mount.name {
				// Verificar si ya está montada (usamos Part_mount para EBR)
				if currentEBR.Part_mount[0] == '1' {
					return "", fmt.Errorf("la partición lógica %s ya está montada", mount.name)
				}
				// Generar un ID único para la partición lógica
				idPartition, _, err := generatePartitionID(mount)
				if err != nil {
					fmt.Println("Error generando el id de partición:", err)
					return "", err
				}
				// Guardar la partición montada en la lista de montajes globales
				stores.MountedPartitions[idPartition] = mount.path
				// Modificamos el EBR para indicar que está montado
				currentEBR.Part_mount[0] = '1'
				err = currentEBR.Serialize(mount.path, currentOffset)
				if err != nil {
					return "", fmt.Errorf("error serializando EBR: %v", err)
				}
				fmt.Println("DEBUG: Partición lógica montada - Nombre:", ebrName, "ID:", idPartition)
				return idPartition, nil
			}
			currentOffset = int64(currentEBR.Part_next)
		}
	}

	fmt.Println("Error: la partición no existe")
	return "", errors.New("la partición no existe")
}

// generatePartitionID genera un ID único para la partición
func generatePartitionID(mount *MOUNT) (string, int, error) {
	// Asignar una letra a la partición y obtener el índice
	letter, partitionCorrelative, err := utils.GetLetterAndPartitionCorrelative(mount.path)
	if err != nil {
		fmt.Println("Error obteniendo la letra:", err)
		return "", 0, err
	}

	// Crear id de partición
	idPartition := fmt.Sprintf("%s%d%s", stores.Carnet, partitionCorrelative, letter)

	return idPartition, partitionCorrelative, nil
}

package commands

import (
	stores "backend/stores"
	structures "backend/structures"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// UNMOUNT estructura que representa el comando unmount con sus parámetros
type UNMOUNT struct {
	id string // ID de la partición a desmontar
}

// ParseUnmount parsea el comando unmount y devuelve el resultado del desmontaje
func ParseUnmount(tokens []string) (string, error) {
	cmd := &UNMOUNT{}
	args := strings.Join(tokens, " ")
	// Expresión regular para capturar el parámetro -id, insensible a mayúsculas/minúsculas
	re := regexp.MustCompile(`(?i)-id="[^"]+"|(?i)-id=[^\s]+`)
	matches := re.FindAllString(args, -1)

	// Validar que no haya parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	allowedParams := map[string]bool{
		"-id": true,
	}

	for _, param := range allParams {
		paramName := strings.ToLower(strings.SplitN(param, "=", 2)[0])
		if !allowedParams[paramName] {
			return "", fmt.Errorf("parámetro no permitido: %s", paramName)
		}
	}

	// Parsear el parámetro -id
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
		case "-id":
			if value == "" {
				return "", errors.New("el id no puede estar vacío")
			}
			cmd.id = value
		}
	}

	// Validar que el parámetro -id esté presente
	if cmd.id == "" {
		return "", errors.New("faltan parámetros requeridos: -id")
	}

	// Desmontar la partición
	err := commandUnmount(cmd)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("UNMOUNT: Partición desmontada exitosamente\n-> ID: %s", cmd.id), nil
}

// commandUnmount desmonta la partición primaria con el ID especificado
func commandUnmount(unmount *UNMOUNT) error {
	// Verificar si el ID existe en el mapa global de particiones montadas
	path, exists := stores.MountedPartitions[unmount.id]
	if !exists {
		return fmt.Errorf("el id '%s' no corresponde a ninguna partición montada", unmount.id)
	}

	// Cargar el MBR desde el disco
	var mbr structures.MBR
	err := mbr.Deserialize(path)
	if err != nil {
		return fmt.Errorf("error deserializando MBR: %v", err)
	}

	// Buscar la partición con el ID especificado (solo primarias)
	found := false
	var indexPartition int
	for i, p := range mbr.Mbr_partitions {
		if p.Part_start == -1 || p.Part_type[0] != 'P' {
			continue
		}
		partitionId := strings.TrimRight(string(p.Part_id[:]), "\x00")
		if partitionId == unmount.id {
			found = true
			indexPartition = i
			break
		}
	}

	if !found {
		return fmt.Errorf("partición con id '%s' no encontrada en el disco", unmount.id)
	}

	// Desmontar la partición primaria
	partition := &mbr.Mbr_partitions[indexPartition]
	partition.Part_status = [1]byte{'0'} // Usamos Part_status, como en mount
	partition.Part_correlative = 0
	partition.Part_id = [4]byte{}

	// Guardar el MBR actualizado
	err = mbr.Serialize(path)
	if err != nil {
		return fmt.Errorf("error serializando MBR: %v", err)
	}

	// Eliminar la partición del mapa global
	delete(stores.MountedPartitions, unmount.id)

	return nil
}

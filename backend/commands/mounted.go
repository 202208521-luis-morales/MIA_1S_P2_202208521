package commands

import (
	stores "backend/stores"
	"fmt"
	"strings"
)

// ParseMounted parsea el comando "mounted" y muestra las particiones montadas
func ParseMounted(tokens []string) (string, error) {
	// Verificar que no se hayan proporcionado parámetros adicionales
	if len(tokens) > 1 {
		return "", fmt.Errorf("el comando 'mounted' no acepta parámetros")
	}

	// Obtener las particiones montadas desde el mapa global
	if len(stores.MountedPartitions) == 0 {
		return "MOUNTED: No hay particiones montadas en el sistema", nil
	}

	// Construir la lista de particiones montadas
	var mountedList []string
	for id, path := range stores.MountedPartitions {
		// Formato: "ID (Path)"
		mountedList = append(mountedList, fmt.Sprintf("%s (%s)", id, path))
	}

	// Unir la lista en una sola cadena con saltos de línea
	output := fmt.Sprintf("MOUNTED: Particiones montadas en el sistema:\n%s",
		strings.Join(mountedList, "\n"))

	return output, nil
}

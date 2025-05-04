package commands

import (
	stores "backend/stores"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// RMDISK estructura que representa el comando rmdisk con su parámetro
type RMDISK struct {
	path string // Ruta del archivo del disco a eliminar
}

// ParseRmdisk parsea el comando rmdisk y elimina el archivo de disco especificado
func ParseRmdisk(tokens []string) (string, error) {
	cmd := &RMDISK{}
	args := strings.Join(tokens, " ")
	// Expresión regular insensible a mayúsculas/minúsculas
	re := regexp.MustCompile(`(?i)-path="[^"]+"|(?i)-path=[^\s]+`)
	matches := re.FindAllString(args, -1)

	// Validar que no haya parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	allowedParams := map[string]bool{
		"-path": true,
	}

	for _, param := range allParams {
		// Extraer solo el nombre del parámetro y convertir a minúsculas
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

		// Remove quotes from value if present
		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			value = strings.Trim(value, "\"")
		}

		switch key {
		case "-path":
			if value == "" {
				return "", errors.New("el path no puede estar vacío")
			}
			cmd.path = value
		}
	}

	if cmd.path == "" {
		return "", errors.New("faltan parámetros requeridos: -path")
	}

	err := commandRmdisk(cmd)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("RMDISK: Disco eliminado exitosamente\n-> Path: %s", cmd.path), nil
}

// commandRmdisk ejecuta la lógica para eliminar el archivo de disco
func commandRmdisk(rmdisk *RMDISK) error {
	// Verificar si el archivo existe
	if _, err := os.Stat(rmdisk.path); os.IsNotExist(err) {
		return errors.New("el disco especificado en -path no existe")
	} else if err != nil {
		return fmt.Errorf("error al verificar el disco: %v", err)
	}

	// Verificar si el disco está montado
	for id, mountedPath := range stores.MountedPartitions {
		if mountedPath == rmdisk.path {
			return fmt.Errorf("no se puede eliminar el disco porque está montado (ID: %s)", id)
		}
	}

	// Eliminar el archivo
	err := os.Remove(rmdisk.path)
	if err != nil {
		return fmt.Errorf("error al eliminar el disco: %v", err)
	}

	return nil
}

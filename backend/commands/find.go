package commands

import (
	stores "backend/stores"
	structures "backend/structures"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// FIND estructura que representa el comando find con sus parámetros
type FIND struct {
	path string // Ruta de la carpeta donde iniciar la búsqueda
	name string // Patrón de nombre para buscar
}

// ParseFind parsea el comando find y devuelve el resultado de la búsqueda
func ParseFind(tokens []string) (string, error) {
	cmd := &FIND{}
	args := strings.Join(tokens, " ")
	re := regexp.MustCompile(`(?i)-path="[^"]+"|(?i)-path=[^\s]+|(?i)-name="[^"]+"|(?i)-name=[^\s]+`)
	matches := re.FindAllString(args, -1)

	// Validar parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	allowedParams := map[string]bool{
		"-path": true,
		"-name": true,
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
		key := strings.ToLower(kv[0])
		value := strings.Trim(kv[1], "\"")

		switch key {
		case "-path":
			if value == "" {
				return "", errors.New("la ruta no puede estar vacía")
			}
			cmd.path = value
		case "-name":
			if value == "" {
				return "", errors.New("el nombre no puede estar vacío")
			}
			cmd.name = value
		}
	}

	if cmd.path == "" || cmd.name == "" {
		return "", fmt.Errorf("faltan parámetros requeridos: -path y -name son obligatorios")
	}

	result, err := commandFind(cmd)
	if err != nil {
		return "", err
	}
	return result, nil
}

// commandFind realiza la búsqueda de archivos o carpetas según el patrón
func commandFind(cmd *FIND) (string, error) {
	// Verificar sesión activa
	if stores.ActiveSession == nil {
		return "", errors.New("debe iniciar sesión primero para realizar una búsqueda")
	}

	// Obtener superbloque y partición montada
	sb, partition, path, err := stores.GetMountedPartitionSuperblock(stores.ActiveSession.PartitionID)
	if err != nil {
		return "", fmt.Errorf("error al obtener partición: %v", err)
	}

	// Normalizar la ruta
	cmd.path = filepath.Clean(cmd.path)
	if !strings.HasPrefix(cmd.path, "/") {
		return "", errors.New("la ruta debe ser absoluta (comenzar con /)")
	}

	// Buscar el directorio de inicio
	startInode, startBlock, _, err := findParentDir(sb, partition, path, cmd.path)
	if err != nil {
		return "", fmt.Errorf("error al buscar directorio de inicio: %v", err)
	}

	// Verificar que sea una carpeta
	if startInode.I_type[0] != '1' {
		return "", fmt.Errorf("la ruta %s no corresponde a una carpeta", cmd.path)
	}

	// Convertir el patrón a una expresión regular
	pattern := regexp.QuoteMeta(cmd.name)
	pattern = strings.ReplaceAll(pattern, `\?`, `.`)  // ? -> un carácter
	pattern = strings.ReplaceAll(pattern, `\*`, `.*`) // * -> cero o más caracteres
	re, err := regexp.Compile("^" + pattern + "$")
	if err != nil {
		return "", fmt.Errorf("patrón de búsqueda inválido: %v", err)
	}

	// Realizar la búsqueda recursiva
	results := []string{}
	err = findRecursive(sb, path, startInode, startBlock, cmd.path, re, &results, 0)
	if err != nil {
		return "", fmt.Errorf("error durante la búsqueda: %v", err)
	}

	// Formatear el resultado en formato de árbol
	if len(results) == 0 {
		return "FIND: No se encontraron coincidencias", nil
	}

	output := "FIND: Resultados de la búsqueda\n"
	for _, result := range results {
		output += result + "\n"
	}
	return output, nil
}

// findRecursive realiza una búsqueda recursiva en el sistema de archivos
func findRecursive(sb *structures.SuperBlock, path string, inode *structures.Inode, block *structures.FolderBlock, currentPath string, pattern *regexp.Regexp, results *[]string, depth int) error {
	// Iterar sobre las entradas del bloque de la carpeta
	for _, content := range block.B_content {
		name := strings.Trim(string(content.B_name[:]), "\x00")
		if name == "." || name == ".." || content.B_inodo == -1 {
			continue
		}

		// Leer el inodo del elemento
		childInode := &structures.Inode{}
		err := childInode.Deserialize(path, int64(sb.S_inode_start)+int64(content.B_inodo)*int64(sb.S_inode_size))
		if err != nil {
			return fmt.Errorf("error al leer inodo: %v", err)
		}

		// Verificar permisos de lectura
		if stores.ActiveSession.Username != "root" {
			hasReadPermission := checkReadPermission(childInode)
			if !hasReadPermission {
				continue // Omitir si no tiene permisos de lectura
			}
		}

		// Construir la ruta completa del elemento
		elementPath := filepath.Join(currentPath, name)

		// Verificar si el nombre coincide con el patrón
		if pattern.MatchString(name) {
			indent := strings.Repeat("| ", depth)
			*results = append(*results, indent+"|_ "+name)
		}

		// Si es una carpeta, buscar recursivamente
		if childInode.I_type[0] == '1' {
			for _, blockIndex := range childInode.I_block {
				if blockIndex == -1 {
					continue
				}
				childBlock := &structures.FolderBlock{}
				err := childBlock.Deserialize(path, int64(sb.S_block_start)+int64(blockIndex)*int64(sb.S_block_size))
				if err != nil {
					return fmt.Errorf("error al leer bloque de carpeta: %v", err)
				}
				err = findRecursive(sb, path, childInode, childBlock, elementPath, pattern, results, depth+1)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

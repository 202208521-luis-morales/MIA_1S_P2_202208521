package commands

import (
	stores "backend/stores"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// MKDIR estructura que representa el comando mkdir con sus parámetros
type MKDIR struct {
	path string // Ruta de la carpeta a crear
	p    bool   // Crear carpetas padres recursivamente
}

// ParseMkdir con debug
func ParseMkdir(tokens []string) (string, error) {
	fmt.Println("DEBUG: Iniciando ParseMkdir - Tokens:", tokens)
	cmd := &MKDIR{}
	args := strings.Join(tokens, " ")
	// Expresión regular para capturar los parámetros válidos, insensible a mayúsculas/minúsculas
	// -path debe tener un valor, -p no debe tenerlo
	re := regexp.MustCompile(`(?i)-path="[^"]+"|(?i)-path=[^\s]+|(?i)-p(?:\s|$)`)
	matches := re.FindAllString(args, -1)

	// Validar que no haya parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	allowedParams := map[string]bool{
		"-path": true,
		"-p":    true,
	}

	for _, param := range allParams {
		// Extraer solo el nombre del parámetro (sin el valor) y convertir a minúsculas
		kv := strings.SplitN(param, "=", 2)
		paramName := strings.ToLower(kv[0])
		if !allowedParams[strings.TrimSpace(paramName)] {
			return "", fmt.Errorf("parámetro no permitido: %s", paramName)
		}
		// Validar que -p no tenga un valor asignado
		if paramName == "-p" && len(kv) > 1 {
			return "", fmt.Errorf("el parámetro -p no debe recibir valor")
		}
	}

	for _, match := range matches {
		match = strings.TrimSpace(match)
		fmt.Println("DEBUG: Procesando match:", match)
		// Si el match es "-p", no tiene valor y no debe ser dividido por "="
		if strings.ToLower(match) == "-p" {
			cmd.p = true
			continue
		}

		// Para otros parámetros (como -path), esperamos un "="
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
		}
	}

	// Validar parámetros obligatorios
	if cmd.path == "" {
		return "", fmt.Errorf("faltan parámetros requeridos: -path")
	}
	fmt.Println("DEBUG: Comando MKDIR parseado - Path:", cmd.path, "P:", cmd.p)

	err := commandMkdir(cmd)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("MKDIR: Carpeta creada exitosamente\n-> Ruta: %s", cmd.path), nil
}

// commandMkdir con debug
func commandMkdir(mkdir *MKDIR) error {
	fmt.Println("\nDEBUG: Iniciando commandMkdir - Path:", mkdir.path, "P:", mkdir.p)

	if stores.ActiveSession == nil {
		return errors.New("debe iniciar sesión primero para crear una carpeta")
	}

	fmt.Println("DEBUG: Obteniendo superbloque y partición para ID:", stores.ActiveSession.PartitionID)
	sb, partition, path, err := stores.GetMountedPartitionSuperblock(stores.ActiveSession.PartitionID)
	if err != nil {
		return fmt.Errorf("error al obtener partición: %v", err)
	}
	fmt.Println("DEBUG: Partición encontrada - Path:", path, "Part_start:", partition.Part_start)
	fmt.Println("DEBUG: SuperBlock - Inodes Count:", sb.S_inodes_count, "Blocks Count:", sb.S_blocks_count, "Free Inodes:", sb.S_free_inodes_count, "Free Blocks:", sb.S_free_blocks_count)

	mkdir.path = filepath.Clean(mkdir.path)
	if !strings.HasPrefix(mkdir.path, "/") {
		return errors.New("la ruta debe ser absoluta (comenzar con /)")
	}
	fmt.Println("DEBUG: Ruta normalizada:", mkdir.path)

	dir, folderName := filepath.Split(mkdir.path)
	if folderName == "" {
		return errors.New("el nombre de la carpeta no puede estar vacío")
	}
	fmt.Println("DEBUG: Directorio padre:", dir, "Nombre de la carpeta:", folderName)

	parentInode, parentBlock, parentIndex, err := findOrCreateParentDir(sb, partition, path, dir, mkdir.p)
	if err != nil {
		return fmt.Errorf("error al buscar/crear directorio padre: %v", err)
	}
	fmt.Println("DEBUG: Directorio padre encontrado - Inodo:", parentIndex, "Bloque[0]:", parentInode.I_block[0])

	if stores.ActiveSession.Username != "root" {
		hasPermission := checkWritePermission(parentInode)
		fmt.Println("DEBUG: Verificando permisos de escritura - UserID:", stores.ActiveSession.UserID, "GroupID:", stores.ActiveSession.GroupID, "Permisos:", string(parentInode.I_perm[:]), "Resultado:", hasPermission)
		if !hasPermission {
			return errors.New("permiso denegado: no tiene permisos de escritura en la carpeta padre")
		}
	}

	for i, content := range parentBlock.B_content {
		name := strings.Trim(string(content.B_name[:]), "\x00")
		fmt.Printf("DEBUG: Revisando entrada %d en bloque padre - Nombre: %q, Inodo: %d\n", i, name, content.B_inodo)
		if name == folderName {
			return errors.New("la carpeta ya existe en la ruta especificada")
		}
	}

	fmt.Println("DEBUG: Creando nueva carpeta")
	_, err = createFolder(sb, path, folderName, parentIndex)
	if err != nil {
		return fmt.Errorf("error al crear carpeta: %v", err)
	}

	// Actualizar el superbloque
	err = sb.Serialize(path, int64(partition.Part_start))
	if err != nil {
		return fmt.Errorf("error al actualizar superbloque: %v", err)
	}
	fmt.Println("DEBUG: Superbloque actualizado")
	fmt.Println("DEBUG: Nueva carpeta creada exitosamente")

	return nil
}

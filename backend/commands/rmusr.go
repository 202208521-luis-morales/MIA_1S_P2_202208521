package commands

import (
	stores "backend/stores"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// RMUSR estructura que representa el comando rmusr con su parámetro
type RMUSR struct {
	user string // Nombre del usuario a eliminar
}

// ParseRmusr parsea el comando rmusr y elimina al usuario
func ParseRmusr(tokens []string) (string, error) {
	cmd := &RMUSR{}
	args := strings.Join(tokens, " ")
	// Expresión regular insensible a mayúsculas/minúsculas
	re := regexp.MustCompile(`(?i)-user=[^\s]+`)
	matches := re.FindAllString(args, -1)

	// Validar que no haya parámetros no permitidos
	reAllParams := regexp.MustCompile(`(?i)-[a-zA-ZñÑ]+(?:="[^"]+"|=[^\s]+|\s|$)`)
	allParams := reAllParams.FindAllString(args, -1)
	allowedParams := map[string]bool{
		"-user": true,
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
		case "-user":
			if value == "" {
				return "", errors.New("el nombre del usuario no puede estar vacío")
			}
			if len(value) > 10 {
				return "", errors.New("el nombre del usuario no puede exceder 10 caracteres")
			}
			cmd.user = value
		}
	}

	if cmd.user == "" {
		return "", errors.New("faltan parámetros requeridos: -user")
	}

	err := commandRmusr(cmd)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("RMUSR: Usuario eliminado exitosamente\n-> Usuario: %s", cmd.user), nil
}

// commandRmusr ejecuta la lógica para eliminar un usuario en users.txt con debug extremo
func commandRmusr(rmusr *RMUSR) error {
	fmt.Println("DEBUG: Iniciando commandRmusr - User:", rmusr.user)

	// Verificar sesión activa
	if stores.ActiveSession == nil {
		return errors.New("debe iniciar sesión primero para eliminar un usuario")
	}

	// Verificar que el usuario sea root
	if stores.ActiveSession.Username != "root" {
		return errors.New("solo el usuario root puede eliminar usuarios")
	}

	// Obtener la partición montada y su superbloque
	fmt.Println("DEBUG: Obteniendo superbloque y partición para ID:", stores.ActiveSession.PartitionID)
	sb, partition, path, err := stores.GetMountedPartitionSuperblock(stores.ActiveSession.PartitionID)
	if err != nil {
		return fmt.Errorf("error al obtener partición: %v", err)
	}
	fmt.Println("DEBUG: Partición encontrada - Path:", path, "Part_start:", partition.Part_start)

	// Leer el contenido actual de users.txt
	fmt.Println("DEBUG: Leyendo users.txt")
	usersText, err := sb.ReadUsersFile(partition, path)
	if err != nil {
		return fmt.Errorf("error al leer users.txt: %v", err)
	}
	fmt.Printf("DEBUG: Contenido actual de users.txt: %q\n", usersText)

	// Parsear líneas y buscar usuario
	lines := strings.Split(usersText, "\n")
	userFound := false
	var newUsersText strings.Builder

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if newUsersText.Len() > 0 {
				newUsersText.WriteString("\n")
			}
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) != 5 {
			fmt.Println("DEBUG: Línea ignorada por formato inválido:", line)
			newUsersText.WriteString(line + "\n")
			continue
		}
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}
		fmt.Println("DEBUG: Línea parseada:", fields)

		// Buscar y marcar el usuario como eliminado (ID = 0)
		if fields[1] == "U" && fields[2] == rmusr.user {
			userFound = true
			fmt.Println("DEBUG: Usuario encontrado:", fields)
			newLine := fmt.Sprintf("0,U,%s,%s,%s", fields[2], fields[3], fields[4])
			newUsersText.WriteString(newLine + "\n")
			fmt.Println("DEBUG: Nueva línea para usuario eliminado:", newLine)
		} else {
			newUsersText.WriteString(line + "\n")
		}
	}

	// Verificar si el usuario fue encontrado
	if !userFound {
		return errors.New("el usuario no existe en la partición")
	}

	// Actualizar users.txt
	newContent := strings.TrimRight(newUsersText.String(), "\n") // Evitar líneas vacías al final
	fmt.Println("DEBUG: Nuevo contenido de users.txt a escribir:", newContent)
	err = sb.UpdateUsersFile(partition, path, newContent)
	if err != nil {
		return fmt.Errorf("error al actualizar users.txt: %v", err)
	}
	fmt.Println("DEBUG: users.txt actualizado exitosamente")

	return nil
}
